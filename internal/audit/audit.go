// SPDX-License-Identifier: Apache-2.0

// Package audit implements the append-only, hash-chained audit log
// (docs/blueprint/06). Every event's hash covers the previous event's hash,
// so changing, removing or reordering a row breaks the chain.
package audit

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/csvout"
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

// personalSaltSize keeps a digest from being tried against a list of addresses.
const personalSaltSize = 16

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
	// PersonalDigest is how the hash covers the personal part, which lives apart so
	// it can be erased while the chain holds (SEC-821). Events older than this field
	// keep their personal data in the payload.
	PersonalDigest string `json:"personal_digest,omitempty"`
}

// CensoredTitle stands in for the title of a page the reader may not see (ADR-029).
const CensoredTitle = "(Censored title)"

// personalDetails are the keys of Details that can identify someone. A title or a
// filename can hold a name as well.
var personalDetails = []string{"email", "name", "display_name", "title", "filename"}

// personalPart is what lives in audit_personal.
type personalPart struct {
	IP        string         `json:"ip,omitempty"`
	UserAgent string         `json:"user_agent,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

func personalDigest(salt []byte, data string) string {
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// splitPersonal gives the event as it goes into the chain, and what goes apart.
func splitPersonal(e Event) (Event, personalPart) {
	part := personalPart{IP: e.IP, UserAgent: e.UserAgent}
	e.IP, e.UserAgent = "", ""
	kept := make(map[string]any, len(e.Details))
	for key, value := range e.Details {
		if !slices.Contains(personalDetails, key) {
			kept[key] = value
			continue
		}
		if part.Details == nil {
			part.Details = map[string]any{}
		}
		part.Details[key] = value
	}
	e.Details = kept
	return e, part
}

func (p personalPart) isEmpty() bool { return p.IP == "" && p.UserAgent == "" && len(p.Details) == 0 }

// restorePersonal puts an event together again for a reader, as far as the part is
// still there.
func (e *Event) restorePersonal(data *string) error {
	if data == nil {
		return nil
	}
	var part personalPart
	if err := json.Unmarshal([]byte(*data), &part); err != nil {
		return fmt.Errorf("audit: personal part: %w", err)
	}
	e.IP, e.UserAgent = part.IP, part.UserAgent
	if len(part.Details) > 0 && e.Details == nil {
		e.Details = map[string]any{}
	}
	maps.Copy(e.Details, part.Details)
	return nil
}

// ErasePersonal removes the personal parts of the events by and about a person. The
// chain holds, because it only knows their digest.
func ErasePersonal(ctx context.Context, tx pgx.Tx, personID string) error {
	_, err := tx.Exec(ctx, `DELETE FROM audit_personal p USING audit_events e
		WHERE e.seq = p.seq AND (e.actor_id = $1 OR e.object_id = $1)`, personID)
	return err
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
	chained, part := splitPersonal(e)
	var salt []byte
	var partData string
	if !part.isEmpty() {
		body, err := json.Marshal(part)
		if err != nil {
			return e, fmt.Errorf("audit: encode: %w", err)
		}
		salt = make([]byte, personalSaltSize)
		if _, err := rand.Read(salt); err != nil {
			return e, fmt.Errorf("audit: salt: %w", err)
		}
		partData = string(body)
		chained.PersonalDigest = personalDigest(salt, partData)
	}
	payload, err := json.Marshal(chained)
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

	var seq int64
	err = tx.QueryRow(ctx, `INSERT INTO audit_events
		(id, occurred_at, actor_type, actor_id, action, object_type, object_id, space_id, result, ip, request_id, payload, prev_hash, hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING seq`,
		e.ID, e.OccurredAt, e.ActorType, e.ActorID, e.Action, e.ObjectType, e.ObjectID, e.SpaceID,
		e.Result, chained.IP, e.RequestID, string(payload), prev, hash).Scan(&seq)
	if err != nil {
		return e, fmt.Errorf("audit: insert: %w", err)
	}
	if salt != nil {
		if _, err := tx.Exec(ctx, "INSERT INTO audit_personal (seq, salt, data) VALUES ($1, $2, $3)", seq, salt, partData); err != nil {
			return e, fmt.Errorf("audit: insert personal part: %w", err)
		}
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
	// The payload is an audit event, never a password: no event holds one (SEC-823). A
	// chain needs a fast hash, so this is the right tool, not a password hash.
	// codeql[go/weak-sensitive-data-hashing]
	h.Write(payload)
	return h.Sum(nil)
}

// VerifyResult is the outcome of a chain verification.
type VerifyResult struct {
	Checked     int64  `json:"checked"`
	OK          bool   `json:"ok"`
	FirstBadSeq int64  `json:"first_bad_seq,omitempty"`
	Reason      string `json:"reason,omitempty"`
	// Checkpoints counts the signed checkpoints that held, when Check looked at them.
	Checkpoints int `json:"checkpoints"`
}

// Verify walks the chain and reports the first inconsistency. After pruning it starts
// at the signed anchor that pruning left.
func Verify(ctx context.Context, q db.Querier) (VerifyResult, error) {
	res := VerifyResult{OK: true}
	prev := genesis
	start, err := latestAnchor(ctx, q)
	if err != nil {
		return VerifyResult{}, err
	}
	if start != nil {
		if !start.valid() {
			return VerifyResult{FirstBadSeq: start.seq, Reason: "the anchor's signature does not match (anchor changed)"}, nil
		}
		prev = start.prevHash
	}
	rows, err := q.Query(ctx, `SELECT e.seq, e.occurred_at, e.actor_type, e.actor_id, e.action, e.object_type, e.object_id,
		e.space_id, e.result, e.ip, e.request_id, e.payload, e.prev_hash, e.hash, p.salt, p.data
		FROM audit_events e LEFT JOIN audit_personal p ON p.seq = e.seq ORDER BY e.seq`)
	if err != nil {
		return VerifyResult{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			seq                                                         int64
			at                                                          time.Time
			actorType, actorID, action, objType, objID, spaceID, result string
			ip, reqID, payload                                          string
			prevHash, hash, salt                                        []byte
			part                                                        *string
		)
		if err := rows.Scan(&seq, &at, &actorType, &actorID, &action, &objType, &objID, &spaceID,
			&result, &ip, &reqID, &payload, &prevHash, &hash, &salt, &part); err != nil {
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
		// A part may be gone, because it was erased. It may not be different.
		if part != nil && personalDigest(salt, *part) != e.PersonalDigest {
			return fail("a personal part does not match its digest (part changed)"), nil
		}
		prev = hash
	}
	return res, rows.Err()
}

// Filter selects events. Zero values mean "no filter".
type Filter struct {
	ActorID    string
	Action     string
	ObjectType string
	SpaceID    string
	Result     string
	Since      time.Time
	Until      time.Time
	BeforeSeq  int64 // for paging backwards: only events with seq < BeforeSeq
	Limit      int
	ObjectID   string
	// Censor removes what the reader may not see, on the screen and in an export.
	Censor func(*Event)
}

func optTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// The query is static; optional filters are switched off with NULL/empty params.
const selectSQL = `SELECT e.seq, e.payload, p.data
	FROM audit_events e LEFT JOIN audit_personal p ON p.seq = e.seq
	WHERE ($1 = '' OR e.actor_id = $1)
	  AND ($2 = '' OR e.action = $2)
	  AND ($3::timestamptz IS NULL OR e.occurred_at >= $3)
	  AND ($4::timestamptz IS NULL OR e.occurred_at < $4)
	  AND ($5::bigint = 0 OR e.seq < $5)
	  AND ($7 = '' OR e.object_type = $7)
	  AND ($8 = '' OR e.space_id = $8)
	  AND ($9 = '' OR e.result = $9)
	  AND ($10 = '' OR e.object_id = $10)`

const listSQL = selectSQL + " ORDER BY e.seq DESC LIMIT $6"
const exportSQL = selectSQL + " ORDER BY e.seq LIMIT $6"

// read turns a row into the event a reader may see.
func (f Filter) read(payload string, part *string) (Event, error) {
	var e Event
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		return e, err
	}
	if err := e.restorePersonal(part); err != nil {
		return e, err
	}
	if f.Censor != nil {
		f.Censor(&e)
	}
	return e, nil
}

// List returns matching events, newest first.
func List(ctx context.Context, q db.Querier, f Filter) ([]Stored, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	rows, err := q.Query(ctx, listSQL, f.ActorID, f.Action, optTime(f.Since), optTime(f.Until), f.BeforeSeq, f.Limit,
		f.ObjectType, f.SpaceID, f.Result, f.ObjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Stored
	for rows.Next() {
		var s Stored
		var payload string
		var part *string
		if err := rows.Scan(&s.Seq, &payload, &part); err != nil {
			return nil, err
		}
		if s.Event, err = f.read(payload, part); err != nil {
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
	rows, err := q.Query(ctx, exportSQL, f.ActorID, f.Action, optTime(f.Since), optTime(f.Until), f.BeforeSeq, maxRows,
		f.ObjectType, f.SpaceID, f.Result, f.ObjectID)
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
		var part *string
		if err := rows.Scan(&seq, &payload, &part); err != nil {
			return n, err
		}
		e, err := f.read(payload, part)
		if err != nil {
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
			rec[i] = csvout.Cell(rec[i])
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
