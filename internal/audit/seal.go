// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
)

// SealBatch is how many waiting events one sealing transaction takes, so a long backlog
// is sealed in steps and no single transaction holds the chain for long.
const SealBatch = 500

// SealWaiting puts every event that waits in audit_pending into the chain, in steps of
// SealBatch, and returns how many it sealed (ADR-041). Any number of nodes may call it at
// once: each step takes the lock of the chain, so exactly one seals at a time and the next
// sees what is left. A release from before ADR-041 appends under the same lock, so its
// events and sealed ones form one chain during an upgrade.
func SealWaiting(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	// A worker calls this every second; when nothing waits, one cheap read is all it costs.
	var hasWaiting bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM audit_pending)").Scan(&hasWaiting); err != nil {
		return 0, fmt.Errorf("audit: look for waiting events: %w", err)
	}
	if !hasWaiting {
		return 0, nil
	}
	total := 0
	for {
		var n int
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			var err error
			n, err = SealIn(ctx, tx, SealBatch)
			return err
		})
		if err != nil {
			return total, err
		}
		total += n
		if n < SealBatch {
			return total, nil
		}
	}
}

// waitingEvent is a row of audit_pending as the sealer reads it.
type waitingEvent struct {
	pend                                                              int64
	id                                                                uuid.UUID
	occurredAt                                                        time.Time
	actorType, actorID, action, objectType, objectID, spaceID, result string
	ip, requestID, payload                                            string
	salt                                                              []byte
	data                                                              *string
}

// SealIn seals up to limit waiting events inside tx, oldest first; a limit of zero or
// less takes all. It takes the lock of the chain, which tx then holds until it ends.
//
// The order of the chain is the order in which events became visible: a transaction that
// began first but committed last is sealed after the ones that committed before it, so
// nothing is skipped and nothing has to be put in between later. Rows are locked as they
// are read, so an erasure of a personal part either happens before (and the part never
// reaches audit_personal) or waits until the part is there to remove.
func SealIn(ctx context.Context, tx pgx.Tx, limit int) (int, error) {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", chainLockKey); err != nil {
		return 0, fmt.Errorf("audit: lock chain: %w", err)
	}
	waiting, err := readWaiting(ctx, tx, limit)
	if err != nil || len(waiting) == 0 {
		return 0, err
	}
	prev, err := chainHead(ctx, tx)
	if err != nil {
		return 0, err
	}
	batch := &pgx.Batch{}
	sealed := make([]int64, 0, len(waiting))
	for _, w := range waiting {
		hash := chainHash(prev, []byte(w.payload))
		batch.Queue(`INSERT INTO audit_events
			(id, occurred_at, actor_type, actor_id, action, object_type, object_id, space_id, result, ip, request_id, payload, prev_hash, hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			w.id, w.occurredAt, w.actorType, w.actorID, w.action, w.objectType, w.objectID, w.spaceID,
			w.result, w.ip, w.requestID, w.payload, prev, hash)
		if w.data != nil {
			batch.Queue(`INSERT INTO audit_personal (seq, salt, data) SELECT seq, $2, $3 FROM audit_events WHERE id = $1`,
				w.id, w.salt, *w.data)
		}
		prev = hash
		sealed = append(sealed, w.pend)
	}
	// Last, so the trigger of audit_pending finds every copy in the chain.
	batch.Queue("DELETE FROM audit_pending WHERE pend = ANY($1)", sealed)
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return 0, fmt.Errorf("audit: seal: %w", err)
	}
	return len(waiting), nil
}

// readWaiting reads the oldest waiting events, locking them for this transaction.
func readWaiting(ctx context.Context, tx pgx.Tx, limit int) ([]waitingEvent, error) {
	sql := `SELECT pend, id, occurred_at, actor_type, actor_id, action, object_type, object_id, space_id, result,
		ip, request_id, payload, salt, data FROM audit_pending ORDER BY pend`
	args := []any{}
	if limit > 0 {
		sql += " LIMIT $1"
		args = append(args, limit)
	}
	rows, err := tx.Query(ctx, sql+" FOR UPDATE", args...)
	if err != nil {
		return nil, fmt.Errorf("audit: read waiting events: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (waitingEvent, error) {
		var w waitingEvent
		err := r.Scan(&w.pend, &w.id, &w.occurredAt, &w.actorType, &w.actorID, &w.action, &w.objectType, &w.objectID,
			&w.spaceID, &w.result, &w.ip, &w.requestID, &w.payload, &w.salt, &w.data)
		return w, err
	})
}

// chainHead is the hash the next sealed event links to: the newest event in the chain, or
// the anchor pruning left when it removed every event, or the start of a new chain.
func chainHead(ctx context.Context, q db.Querier) ([]byte, error) {
	var prev []byte
	err := q.QueryRow(ctx, "SELECT hash FROM audit_events ORDER BY seq DESC LIMIT 1").Scan(&prev)
	if err == nil {
		return prev, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("audit: read chain head: %w", err)
	}
	start, err := latestAnchor(ctx, q)
	if err != nil {
		return nil, err
	}
	if start != nil {
		return start.prevHash, nil
	}
	return genesis, nil
}
