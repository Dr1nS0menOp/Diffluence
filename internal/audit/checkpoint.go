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

// SignCheckpoint signs the head of the chain (ADR-013). A hash chain alone can be
// rewritten end to end by someone with database access; a signature cannot be
// forged without the key, which is not in the database.
func SignCheckpoint(ctx context.Context, pool *pgxpool.Pool, key ed25519.PrivateKey) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var seq int64
		var hash []byte
		err := tx.QueryRow(ctx, "SELECT seq, hash FROM audit_events ORDER BY seq DESC LIMIT 1").Scan(&seq, &hash)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		sig := ed25519.Sign(key, checkpointMessage(seq, hash))
		public, _ := key.Public().(ed25519.PublicKey)
		tag, err := tx.Exec(ctx, `INSERT INTO audit_checkpoints (seq, hash, signature, public_key) VALUES ($1, $2, $3, $4)
			ON CONFLICT (seq) DO NOTHING`, seq, hash, sig, []byte(public))
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		_, err = Record(ctx, tx, Event{ActorType: ActorSystem, ActorID: "worker", Action: "audit.checkpoint_signed",
			ObjectType: "audit_log", Result: ResultSuccess, Details: map[string]any{"seq": seq}})
		return err
	})
}

// VerifyCheckpoints checks every checkpoint against the key and the chain itself.
// It returns how many are valid, and an error for the first one that is not.
func VerifyCheckpoints(ctx context.Context, q db.Querier, public ed25519.PublicKey) (int, error) {
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
			return n, fmt.Errorf("audit: checkpoint at seq %d has a bad signature", seq)
		}
		if string(chain) != string(hash) {
			return n, fmt.Errorf("audit: the chain does not match the checkpoint at seq %d", seq)
		}
		n++
	}
	return n, rows.Err()
}
