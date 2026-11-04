// SPDX-License-Identifier: Apache-2.0

// Package audit implements the append-only, hash-chained audit log
// (docs/blueprint/06). Every event's hash covers the previous event's hash,
// so changing, removing or reordering a row breaks the chain.
package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/ids"
)

// Results.
const (
	ResultSuccess = "success"
	ResultDenied  = "denied"
	ResultFailure = "failure"
)

// Actor types.
const (
	ActorUser      = "user"
	ActorSystem    = "system"
	ActorAnonymous = "anonymous"
)

// chainLockKey serialises appends so each event sees its predecessor.
const chainLockKey int64 = 0x6466_6c75_6175_6474 // "dfluaudt"

// genesis is the "previous hash" of the very first event.
var genesis = make([]byte, sha256.Size)

var actionPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// Event is one audit record. Field order is part of the hash format: append
// new fields at the end only, never reorder or rename.
type Event struct {
	ID         uuid.UUID      `json:"id"`
	OccurredAt time.Time      `json:"occurred_at"`
	ActorType  string         `json:"actor_type"`
	ActorID    string         `json:"actor_id,omitempty"`
	Action     string         `json:"action"`
	ObjectType string         `json:"object_type,omitempty"`
	ObjectID   string         `json:"object_id,omitempty"`
	SpaceID    string         `json:"space_id,omitempty"`
	Result     string         `json:"result"`
	IP         string         `json:"ip,omitempty"`
	UserAgent  string         `json:"user_agent,omitempty"`
	RequestID  string         `json:"request_id,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

// Stored is an event as read back from the database.
type Stored struct {
	Seq int64
	Event
}

func (e *Event) validate() error {
	switch e.ActorType {
	case ActorUser, ActorSystem, ActorAnonymous:
	default:
		return fmt.Errorf("audit: invalid actor type %q", e.ActorType)
	}
	switch e.Result {
	case ResultSuccess, ResultDenied, ResultFailure:
	default:
		return fmt.Errorf("audit: invalid result %q", e.Result)
	}
	if !actionPattern.MatchString(e.Action) {
		return fmt.Errorf("audit: invalid action %q (want e.g. \"auth.login\")", e.Action)
	}
	return nil
}

// Record appends an event inside tx, so it commits or rolls back together with
// the change it describes.
func Record(ctx context.Context, tx pgx.Tx, e Event) (Event, error) {
	if e.ID == uuid.Nil {
		e.ID = ids.New()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now()
	}
	// PostgreSQL stores microseconds; truncate so payload and column agree.
	e.OccurredAt = e.OccurredAt.UTC().Truncate(time.Microsecond)
	if err := e.validate(); err != nil {
		return e, err
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return e, fmt.Errorf("audit: encode: %w", err)
	}

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", chainLockKey); err != nil {
		return e, fmt.Errorf("audit: lock chain: %w", err)
	}
	var prev []byte
	err = tx.QueryRow(ctx, "SELECT hash FROM audit_events ORDER BY seq DESC LIMIT 1").Scan(&prev)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		prev = genesis
	case err != nil:
		return e, fmt.Errorf("audit: read chain head: %w", err)
	}
	hash := chainHash(prev, payload)

	_, err = tx.Exec(ctx, `INSERT INTO audit_events
		(id, occurred_at, actor_type, actor_id, action, object_type, object_id, space_id, result, ip, request_id, payload, prev_hash, hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		e.ID, e.OccurredAt, e.ActorType, e.ActorID, e.Action, e.ObjectType, e.ObjectID, e.SpaceID,
		e.Result, e.IP, e.RequestID, string(payload), prev, hash)
	if err != nil {
		return e, fmt.Errorf("audit: insert: %w", err)
	}
	return e, nil
}

// RecordStandalone records an event in its own transaction. Use it only for
// events that do not belong to a data change (failed logins, denied access,
// read-only admin actions).
func RecordStandalone(ctx context.Context, pool *pgxpool.Pool, e Event) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		_, err := Record(ctx, tx, e)
		return err
	})
}

func chainHash(prev, payload []byte) []byte {
	h := sha256.New()
	h.Write(prev)
	h.Write(payload)
	return h.Sum(nil)
}

// VerifyResult is the outcome of a chain verification.
type VerifyResult struct {
	Checked     int64  `json:"checked"`
	OK          bool   `json:"ok"`
	FirstBadSeq int64  `json:"first_bad_seq,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// Verify walks the whole chain and reports the first inconsistency.
func Verify(ctx context.Context, q db.Querier) (VerifyResult, error) {
	rows, err := q.Query(ctx, `SELECT seq, occurred_at, actor_type, actor_id, action, object_type, object_id,
		space_id, result, ip, request_id, payload, prev_hash, hash FROM audit_events ORDER BY seq`)
	if err != nil {
		return VerifyResult{}, err
	}
	defer rows.Close()

	res := VerifyResult{OK: true}
	prev := genesis
	for rows.Next() {
		var (
			seq                                                         int64
			at                                                          time.Time
			actorType, actorID, action, objType, objID, spaceID, result string
			ip, reqID, payload                                          string
			prevHash, hash                                              []byte
		)
		if err := rows.Scan(&seq, &at, &actorType, &actorID, &action, &objType, &objID, &spaceID,
			&result, &ip, &reqID, &payload, &prevHash, &hash); err != nil {
			return res, err
		}
		res.Checked++
		fail := func(reason string) VerifyResult {
			res.OK, res.FirstBadSeq, res.Reason = false, seq, reason
			return res
		}
		if !bytes.Equal(prevHash, prev) {
			return fail("previous hash does not match (row removed or reordered)"), nil
		}
		if !bytes.Equal(hash, chainHash(prev, []byte(payload))) {
			return fail("hash does not match payload (payload changed)"), nil
		}
		var e Event
		if err := json.Unmarshal([]byte(payload), &e); err != nil {
			return fail("payload is not valid JSON"), nil
		}
		if !e.OccurredAt.Equal(at) || e.ActorType != actorType || e.ActorID != actorID || e.Action != action ||
			e.ObjectType != objType || e.ObjectID != objID || e.SpaceID != spaceID || e.Result != result ||
			e.IP != ip || e.RequestID != reqID {
			return fail("indexed columns do not match payload (columns changed)"), nil
		}
		prev = hash
	}
	return res, rows.Err()
}

// Filter selects events. Zero values mean "no filter".
type Filter struct {
	ActorID   string
	Action    string
	Since     time.Time
	Until     time.Time
	BeforeSeq int64 // for paging backwards: only events with seq < BeforeSeq
	Limit     int
}

func optTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// The query is static; optional filters are switched off with NULL/empty params.
const listSQL = `SELECT seq, payload FROM audit_events
	WHERE ($1 = '' OR actor_id = $1)
	  AND ($2 = '' OR action = $2)
	  AND ($3::timestamptz IS NULL OR occurred_at >= $3)
	  AND ($4::timestamptz IS NULL OR occurred_at < $4)
	  AND ($5::bigint = 0 OR seq < $5)
	ORDER BY seq DESC
	LIMIT $6`

// List returns matching events, newest first.
func List(ctx context.Context, q db.Querier, f Filter) ([]Stored, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	rows, err := q.Query(ctx, listSQL, f.ActorID, f.Action, optTime(f.Since), optTime(f.Until), f.BeforeSeq, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Stored
	for rows.Next() {
		var s Stored
		var payload string
		if err := rows.Scan(&s.Seq, &payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(payload), &s.Event); err != nil {
			return nil, fmt.Errorf("audit: decode seq %d: %w", s.Seq, err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CSVHeader is the column order of ExportCSV.
var CSVHeader = []string{"seq", "id", "occurred_at", "actor_type", "actor_id", "action", "object_type",
	"object_id", "space_id", "result", "ip", "user_agent", "request_id", "details"}

// ExportCSV writes matching events (oldest first, up to maxRows) as CSV.
func ExportCSV(ctx context.Context, q db.Querier, f Filter, maxRows int, w io.Writer) (int, error) {
	rows, err := q.Query(ctx, `SELECT seq, payload FROM audit_events
		WHERE ($1 = '' OR actor_id = $1)
		  AND ($2 = '' OR action = $2)
		  AND ($3::timestamptz IS NULL OR occurred_at >= $3)
		  AND ($4::timestamptz IS NULL OR occurred_at < $4)
		ORDER BY seq
		LIMIT $5`, f.ActorID, f.Action, optTime(f.Since), optTime(f.Until), maxRows)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	cw := csv.NewWriter(w)
	if err := cw.Write(CSVHeader); err != nil {
		return 0, err
	}
	n := 0
	for rows.Next() {
		var seq int64
		var payload string
		if err := rows.Scan(&seq, &payload); err != nil {
			return n, err
		}
		var e Event
		if err := json.Unmarshal([]byte(payload), &e); err != nil {
			return n, fmt.Errorf("audit: decode seq %d: %w", seq, err)
		}
		details := ""
		if len(e.Details) > 0 {
			b, _ := json.Marshal(e.Details)
			details = string(b)
		}
		rec := []string{strconv.FormatInt(seq, 10), e.ID.String(), e.OccurredAt.Format(time.RFC3339Nano),
			e.ActorType, e.ActorID, e.Action, e.ObjectType, e.ObjectID, e.SpaceID, e.Result, e.IP,
			e.UserAgent, e.RequestID, details}
		for i := range rec {
			rec[i] = csvSafe(rec[i])
		}
		if err := cw.Write(rec); err != nil {
			return n, err
		}
		n++
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return n, err
	}
	return n, rows.Err()
}

// csvSafe prevents spreadsheet formula injection: a cell starting with
// = + - @ or a control character is prefixed with a single quote.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
