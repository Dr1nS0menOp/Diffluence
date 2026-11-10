// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
)

// checkpointMessage is what gets signed: the position and hash of the chain head.
func checkpointMessage(seq int64, hash []byte) []byte {
	out := make([]byte, 8, 8+len(hash))
	binary.BigEndian.PutUint64(out, uint64(seq))
	return append(out, hash...)
}

// checkpointSignedAction is the event that records a new checkpoint.
const checkpointSignedAction = "audit.checkpoint_signed"

// Checkpoint is one signed head of the chain.
type Checkpoint struct {
	Seq       int64
	Hash      []byte
	Signature []byte
	PublicKey ed25519.PublicKey
}

// SignCheckpoint signs the head of the chain (ADR-013, ADR-039). A hash chain alone can
// be rewritten end to end by someone with database access; a signature cannot be forged
// without the key, which is not in the database. It reports false when there was nothing
// new to sign, so a quiet instance does not grow its log by a checkpoint every few minutes.
func SignCheckpoint(ctx context.Context, pool *pgxpool.Pool, key ed25519.PrivateKey) (Checkpoint, bool, error) {
	public, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return Checkpoint{}, false, errors.New("audit: unexpected key type")
	}
	var out Checkpoint
	signed := false
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var action string
		err := tx.QueryRow(ctx, "SELECT seq, hash, action FROM audit_events ORDER BY seq DESC LIMIT 1").Scan(&out.Seq, &out.Hash, &action)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		// Only the record of the last checkpoint came after it: nothing happened since.
		if action == checkpointSignedAction {
			return nil
		}
		out.Signature = ed25519.Sign(key, checkpointMessage(out.Seq, out.Hash))
		out.PublicKey = public
		tag, err := tx.Exec(ctx, `INSERT INTO audit_checkpoints (seq, hash, signature, public_key) VALUES ($1, $2, $3, $4)
			ON CONFLICT (seq) DO NOTHING`, out.Seq, out.Hash, out.Signature, []byte(public))
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		if _, err = Record(ctx, tx, Event{ActorType: ActorSystem, ActorID: "worker", Action: checkpointSignedAction,
			ObjectType: "audit_log", Result: ResultSuccess, Details: map[string]any{"seq": out.Seq}}); err != nil {
			return err
		}
		signed = true
		return nil
	})
	if err != nil || !signed {
		return Checkpoint{}, false, err
	}
	return out, true, nil
}

// BadCheckpointError says which signed checkpoint does not hold.
type BadCheckpointError struct {
	Seq    int64
	Reason string
}

func (e *BadCheckpointError) Error() string {
	return fmt.Sprintf("audit: %s at seq %d", e.Reason, e.Seq)
}

// Check verifies the chain and then the signed checkpoints. The chain alone can be
// rewritten end to end by someone with database access, and the signatures cannot, so
// everything that verifies goes through here (SEC-814).
func Check(ctx context.Context, q db.Querier, public ed25519.PublicKey) (VerifyResult, error) {
	res, err := Verify(ctx, q)
	if err != nil || !res.OK {
		return res, err
	}
	res.Checkpoints, err = VerifyCheckpoints(ctx, q, public)
	var bad *BadCheckpointError
	if errors.As(err, &bad) {
		res.OK, res.FirstBadSeq, res.Reason = false, bad.Seq, bad.Reason
		return res, nil
	}
	return res, err
}

// VerifyCheckpoints checks every checkpoint against the key and the chain itself.
// It returns how many are valid, and a BadCheckpointError for the first one that is not.
func VerifyCheckpoints(ctx context.Context, q db.Querier, public ed25519.PublicKey) (int, error) {
	start, err := latestAnchor(ctx, q)
	if err != nil {
		return 0, err
	}
	var prunedUpTo int64
	if start != nil {
		if !ed25519.Verify(public, anchorMessage(start.seq, start.prevHash), start.signature) {
			return 0, &BadCheckpointError{Seq: start.seq, Reason: "the anchor has a bad signature"}
		}
		prunedUpTo = start.seq
	}
	rows, err := q.Query(ctx, `SELECT c.seq, c.hash, c.signature, e.hash FROM audit_checkpoints c
		LEFT JOIN audit_events e ON e.seq = c.seq ORDER BY c.seq`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var seq int64
		var hash, sig, chain []byte
		if err := rows.Scan(&seq, &hash, &sig, &chain); err != nil {
			return n, err
		}
		if !ed25519.Verify(public, checkpointMessage(seq, hash), sig) {
			return n, &BadCheckpointError{Seq: seq, Reason: "the checkpoint has a bad signature"}
		}
		if seq > prunedUpTo && string(chain) != string(hash) {
			return n, &BadCheckpointError{Seq: seq, Reason: "the chain does not match the checkpoint"}
		}
		n++
	}
	return n, rows.Err()
}
