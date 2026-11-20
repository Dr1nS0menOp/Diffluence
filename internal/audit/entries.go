// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/db"
)

// Entries of the chain as an auditor checks them outside Kenwerk (spec 064, DOC-741,
// DOC-742): the exact payload the chain hashes, the hash of the entry before it, and its
// own hash, so hash = SHA-256(prev_hash || payload) can be recomputed by anyone.

// Entry is one event as the chain holds it. An event that still waits for the sealer has no
// Seq and no hashes yet.
type Entry struct {
	ID       uuid.UUID `json:"id"`
	Seq      int64     `json:"seq"`
	Action   string    `json:"action"`
	At       time.Time `json:"occurred_at"`
	Payload  string    `json:"payload"`
	PrevHash string    `json:"prev_hash,omitempty"`
	Hash     string    `json:"hash,omitempty"`
	// Linked says the stored hashes hold: the entry's hash covers the hash before it and its
	// payload, the entry before it ends in that hash, and the entry after it names this one.
	// A neighbour removed by pruning is not asked about.
	Linked bool `json:"linked"`
}

// Sealed says the entry is in the chain.
func (e Entry) Sealed() bool { return e.Seq != 0 }

// EntryByID finds one event, sealed or still waiting, with whether its links hold. found is
// false when there is no such event at all.
func EntryByID(ctx context.Context, q db.Querier, id uuid.UUID) (Entry, bool, error) {
	e := Entry{ID: id}
	var prev, hash []byte
	err := q.QueryRow(ctx, "SELECT seq, action, occurred_at, payload, prev_hash, hash FROM audit_events WHERE id = $1", id).
		Scan(&e.Seq, &e.Action, &e.At, &e.Payload, &prev, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		err = q.QueryRow(ctx, "SELECT action, occurred_at, payload FROM audit_pending WHERE id = $1", id).Scan(&e.Action, &e.At, &e.Payload)
		if errors.Is(err, pgx.ErrNoRows) {
			return e, false, nil
		}
		// A waiting event is not in the chain yet, so nothing can break its links.
		e.Linked = err == nil
		return e, err == nil, err
	}
	if err != nil {
		return e, false, err
	}
	e.PrevHash, e.Hash = hex.EncodeToString(prev), hex.EncodeToString(hash)
	e.Linked, err = linked(ctx, q, e.Seq, prev, hash, []byte(e.Payload))
	return e, true, err
}

// linked checks one sealed entry against its own hash and its neighbours.
func linked(ctx context.Context, q db.Querier, seq int64, prev, hash, payload []byte) (bool, error) {
	if !bytes.Equal(chainHash(prev, payload), hash) {
		return false, nil
	}
	var before, after []byte
	err := q.QueryRow(ctx, `SELECT (SELECT hash FROM audit_events WHERE seq = $1 - 1), (SELECT prev_hash FROM audit_events WHERE seq = $1 + 1)`, seq).
		Scan(&before, &after)
	if err != nil {
		return false, err
	}
	if before != nil && !bytes.Equal(before, prev) {
		return false, nil
	}
	return after == nil || bytes.Equal(after, hash), nil
}

// PageEntries hands every sealed or waiting event about the given pages within a period to fn,
// oldest first, with whether its links hold. Since and until may be zero. It is for the
// auditor's export and the control history, which name their pages.
func PageEntries(ctx context.Context, q db.Querier, pageIDs []string, since, until time.Time, fn func(Entry) error) error {
	rows, err := q.Query(ctx, `SELECT e.id, e.seq, e.action, e.occurred_at, e.payload, e.prev_hash, e.hash,
			(SELECT b.hash FROM audit_events b WHERE b.seq = e.seq - 1), (SELECT a.prev_hash FROM audit_events a WHERE a.seq = e.seq + 1)
		FROM audit_events e
		WHERE e.object_type = 'page' AND e.object_id = ANY($1) AND ($2::timestamptz IS NULL OR e.occurred_at >= $2) AND ($3::timestamptz IS NULL OR e.occurred_at < $3)
		ORDER BY e.seq`, pageIDs, optTime(since), optTime(until))
	if err != nil {
		return err
	}
	defer rows.Close()
	// An event can be in both tables for a moment while the sealer moves it.
	seen := map[uuid.UUID]bool{}
	for rows.Next() {
		var e Entry
		var prev, hash, before, after []byte
		if err := rows.Scan(&e.ID, &e.Seq, &e.Action, &e.At, &e.Payload, &prev, &hash, &before, &after); err != nil {
			return err
		}
		e.PrevHash, e.Hash = hex.EncodeToString(prev), hex.EncodeToString(hash)
		e.Linked = bytes.Equal(chainHash(prev, []byte(e.Payload)), hash) && (before == nil || bytes.Equal(before, prev)) &&
			(after == nil || bytes.Equal(after, hash))
		seen[e.ID] = true
		if err := fn(e); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	waiting, err := q.Query(ctx, `SELECT id, action, occurred_at, payload FROM audit_pending
		WHERE object_type = 'page' AND object_id = ANY($1) AND ($2::timestamptz IS NULL OR occurred_at >= $2) AND ($3::timestamptz IS NULL OR occurred_at < $3)
		ORDER BY pend`, pageIDs, optTime(since), optTime(until))
	if err != nil {
		return err
	}
	defer waiting.Close()
	for waiting.Next() {
		e := Entry{Linked: true}
		if err := waiting.Scan(&e.ID, &e.Action, &e.At, &e.Payload); err != nil {
			return err
		}
		if seen[e.ID] {
			continue
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	return waiting.Err()
}
