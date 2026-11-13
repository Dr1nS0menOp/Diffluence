// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
)

// The checkpoints are signed by a chain of keys (spec 070). The first is the installation's
// own, derived from master.key, and needs no row. Each later key is a row of
// audit_signing_keys: it signs the checkpoints from its from_seq on, and carries the
// installation's signature over that position and itself. So verifying needs only the
// installation's public key, and existing checkpoints keep the key they were made with.

// checkpointKeyChangedAction records the point where another key starts to sign.
const checkpointKeyChangedAction = "audit.checkpoint_key_changed"

// ErrKeyNotInForce means a later key signs the checkpoints now.
var ErrKeyNotInForce = errors.New("audit: this key no longer signs checkpoints")

// keyChangeLabel keeps an endorsement apart from every other message the key signs.
var keyChangeLabel = []byte("diffluence checkpoint key v1\x00")

// keyChangeMessage is what the installation's key signs to bring in a key at a position.
func keyChangeMessage(fromSeq int64, public []byte) []byte {
	out := append([]byte(nil), keyChangeLabel...)
	out = binary.BigEndian.AppendUint64(out, uint64(fromSeq)) // #nosec G115 -- from_seq is checked positive.
	return append(out, public...)
}

type era struct {
	from   int64
	public ed25519.PublicKey
}

// keyChain says which key signs the checkpoint at each position.
type keyChain struct {
	root ed25519.PublicKey
	eras []era // by from, oldest first
}

// missingTable is a database from before spec 070, which has no later keys.
func missingTable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}

// loadKeyChain reads the later keys and checks that the installation endorsed each one. A key
// without that endorsement is a BadCheckpointError at its position.
func loadKeyChain(ctx context.Context, q db.Querier, root ed25519.PublicKey) (keyChain, error) {
	chain := keyChain{root: root}
	rows, err := q.Query(ctx, "SELECT from_seq, public_key, endorsement FROM audit_signing_keys ORDER BY from_seq")
	if missingTable(err) {
		return chain, nil
	}
	if err != nil {
		return chain, err
	}
	defer rows.Close()
	for rows.Next() {
		var e era
		var public, endorsement []byte
		if err := rows.Scan(&e.from, &public, &endorsement); err != nil {
			return chain, err
		}
		if len(public) != ed25519.PublicKeySize || !ed25519.Verify(root, keyChangeMessage(e.from, public), endorsement) {
			return chain, &BadCheckpointError{Seq: e.from, Reason: "a checkpoint key is not endorsed by the installation's key"}
		}
		e.public = public
		chain.eras = append(chain.eras, e)
	}
	return chain, rows.Err()
}

// at is the key that signs the checkpoint at seq.
func (c keyChain) at(seq int64) ed25519.PublicKey {
	key := c.root
	for _, e := range c.eras {
		if e.from > seq {
			break
		}
		key = e.public
	}
	return key
}

// knows says whether public is the installation's key or an endorsed one.
func (c keyChain) knows(public []byte) bool {
	if bytes.Equal(public, c.root) {
		return true
	}
	for _, e := range c.eras {
		if bytes.Equal(public, e.public) {
			return true
		}
	}
	return false
}

// KeyInForce is the newest key after the installation's own, or nil when none was added.
func KeyInForce(ctx context.Context, q db.Querier) (ed25519.PublicKey, error) {
	var public []byte
	err := q.QueryRow(ctx, "SELECT public_key FROM audit_signing_keys ORDER BY from_seq DESC LIMIT 1").Scan(&public)
	if errors.Is(err, pgx.ErrNoRows) || missingTable(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return public, nil
}

// ChangeKey makes next the key that signs checkpoints from the next position of the chain
// on, endorsed by installation, and records the change. It holds the lock of the chain, so
// no event is sealed meanwhile. It returns the position, and false when next is in force
// already. Only the owner of the tables may add a key.
func ChangeKey(ctx context.Context, owner *pgxpool.Pool, installation, next ed25519.PrivateKey) (int64, bool, error) {
	public, ok := next.Public().(ed25519.PublicKey)
	if !ok {
		return 0, false, errors.New("audit: unexpected key type")
	}
	var from int64
	changed := false
	err := pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", chainLockKey); err != nil {
			return fmt.Errorf("audit: lock chain: %w", err)
		}
		inForce, err := KeyInForce(ctx, tx)
		if err != nil {
			return err
		}
		if inForce.Equal(public) {
			return nil
		}
		// Past every position a checkpoint or key already holds, so each keeps its key.
		if err := tx.QueryRow(ctx, `SELECT GREATEST(
				(SELECT COALESCE(max(seq), 0) FROM audit_events),
				(SELECT COALESCE(max(seq), 0) FROM audit_checkpoints),
				(SELECT COALESCE(max(from_seq), 0) FROM audit_signing_keys)) + 1`).Scan(&from); err != nil {
			return err
		}
		endorsement := ed25519.Sign(installation, keyChangeMessage(from, public))
		if _, err := tx.Exec(ctx, "INSERT INTO audit_signing_keys (from_seq, public_key, endorsement) VALUES ($1, $2, $3)",
			from, []byte(public), endorsement); err != nil {
			return fmt.Errorf("audit: add the checkpoint key: %w", err)
		}
		if _, err := Record(ctx, tx, Event{ActorType: ActorSystem, ActorID: "backup", Action: checkpointKeyChangedAction,
			ObjectType: "audit_log", Result: ResultSuccess,
			Details: map[string]any{"from_seq": from, "public_key": hex.EncodeToString(public)}}); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return from, changed, err
}

// WorkerCheckpoint is a round of the backup service (spec 070). It brings the worker's key in
// force when it is not yet, endorsed by the installation's key, and signs the head of the
// chain when the key just changed or the newest checkpoint is older than every. It seals what
// waits first, so the checkpoint covers everything committed before it.
func WorkerCheckpoint(ctx context.Context, owner *pgxpool.Pool, installation, worker ed25519.PrivateKey,
	every time.Duration) (Checkpoint, bool, error) {
	_, changed, err := ChangeKey(ctx, owner, installation, worker)
	if err != nil {
		return Checkpoint{}, false, err
	}
	if !changed {
		due, err := CheckpointDue(ctx, owner, every)
		if err != nil || !due {
			return Checkpoint{}, false, err
		}
	}
	if _, err := SealWaiting(ctx, owner); err != nil {
		return Checkpoint{}, false, err
	}
	return SignCheckpoint(ctx, owner, worker)
}

// CheckpointDue says whether the newest checkpoint is older than every, or there is none.
func CheckpointDue(ctx context.Context, q db.Querier, every time.Duration) (bool, error) {
	var newest *time.Time
	if err := q.QueryRow(ctx, "SELECT max(created_at) FROM audit_checkpoints").Scan(&newest); err != nil {
		return false, err
	}
	return newest == nil || time.Since(*newest) >= every, nil
}
