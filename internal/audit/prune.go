// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
)

// Pruned says what a prune did.
type Pruned struct {
	Removed   int64
	AnchorSeq int64
}

type anchor struct {
	seq       int64
	prevHash  []byte
	signature []byte
	publicKey []byte
}

func anchorMessage(seq int64, prevHash []byte) []byte {
	return append(binary.BigEndian.AppendUint64(nil, uint64(seq)), prevHash...) // #nosec G115 -- seq comes from a bigserial and is never negative.
}

// latestAnchor finds where the chain starts after pruning, if it was ever pruned.
func latestAnchor(ctx context.Context, q db.Querier) (*anchor, error) {
	var a anchor
	err := q.QueryRow(ctx, "SELECT seq, prev_hash, signature, public_key FROM audit_anchors ORDER BY seq DESC LIMIT 1").
		Scan(&a.seq, &a.prevHash, &a.signature, &a.publicKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (a *anchor) valid() bool {
	return len(a.publicKey) == ed25519.PublicKeySize &&
		ed25519.Verify(a.publicKey, anchorMessage(a.seq, a.prevHash), a.signature)
}

// Prune removes events older than the given moment, and leaves a signed anchor for
// verification to start from (spec 013, D11). Only the owner of the tables can do
// this: the application role cannot touch the triggers, and has no right to delete.
// A chain that does not verify is left alone, because pruning would hide the damage.
// The anchor is signed with the key of the checkpoints, which is not in the database.
func Prune(ctx context.Context, owner *pgxpool.Pool, before time.Time, key ed25519.PrivateKey) (Pruned, error) {
	check, err := Verify(ctx, owner)
	if err != nil {
		return Pruned{}, err
	}
	if !check.OK {
		return Pruned{}, fmt.Errorf("audit: the chain is damaged at %d: %s", check.FirstBadSeq, check.Reason)
	}
	public, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return Pruned{}, errors.New("audit: unexpected key type")
	}
	var out Pruned
	err = pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error {
		var lastHash []byte
		err := tx.QueryRow(ctx, "SELECT seq, hash FROM audit_events WHERE occurred_at < $1 ORDER BY seq DESC LIMIT 1", before).
			Scan(&out.AnchorSeq, &lastHash)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		// First, so the chain goes on after the rows below are gone.
		if _, err := Record(ctx, tx, Event{ActorType: ActorSystem, Action: "audit.pruned", Result: ResultSuccess,
			Details: map[string]any{"up_to_seq": out.AnchorSeq}}); err != nil {
			return err
		}
		signature := ed25519.Sign(key, anchorMessage(out.AnchorSeq, lastHash))
		if _, err := tx.Exec(ctx, "INSERT INTO audit_anchors (seq, prev_hash, signature, public_key) VALUES ($1, $2, $3, $4)",
			out.AnchorSeq, lastHash, signature, []byte(public)); err != nil {
			return err
		}
		// In this transaction only: a failure brings the protection back with the rollback.
		if _, err := tx.Exec(ctx, "ALTER TABLE audit_events DISABLE TRIGGER audit_events_no_update_delete"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM audit_personal WHERE seq <= $1", out.AnchorSeq); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM audit_events WHERE seq <= $1", out.AnchorSeq)
		if err != nil {
			return err
		}
		out.Removed = tag.RowsAffected()
		_, err = tx.Exec(ctx, "ALTER TABLE audit_events ENABLE TRIGGER audit_events_no_update_delete")
		return err
	})
	return out, err
}
