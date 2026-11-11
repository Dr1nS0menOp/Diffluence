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
	// An event that still waits for the sealer carries its personal part with it. This goes
	// first: if the sealer is moving such a part right now, the update waits for it, and the
	// delete below, a statement of its own, then sees the part in audit_personal.
	if _, err := tx.Exec(ctx, `UPDATE audit_pending SET salt = NULL, data = NULL
		WHERE (actor_id = $1 OR object_id = $1) AND salt IS NOT NULL`, personID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM audit_personal p USING audit_events e
		WHERE e.seq = p.seq AND (e.actor_id = $1 OR e.object_id = $1)`, personID)
	return err
}

// Stored is an event as read back from the database. An event that still waits for the
// sealer has no Seq yet and Sealed is false.
type Stored struct {
	Seq    int64
	Sealed bool
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

// Record writes an event inside tx, so it commits or rolls back together with the
// change it describes. It takes no lock: the event waits in audit_pending until the
// sealer puts it in the chain a moment after it commits (ADR-041), so saves no longer
// queue behind each other's commit.
func Record(ctx context.Context, tx pgx.Tx, e Event) (Event, error) {
	p, err := prepare(e)
	if err != nil {
		return p.event, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_pending
		(id, occurred_at, actor_type, actor_id, action, object_type, object_id, space_id, result, ip, request_id, payload, salt, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		p.event.ID, p.event.OccurredAt, p.event.ActorType, p.event.ActorID, p.event.Action, p.event.ObjectType,
		p.event.ObjectID, p.event.SpaceID, p.event.Result, p.chainedIP, p.event.RequestID, p.payload, p.salt, p.data)
	if err != nil {
		return p.event, fmt.Errorf("audit: insert: %w", err)
	}
	return p.event, nil
}

// prepared is an event as it goes into the log: the payload the chain hashes and the
// personal part kept apart.
type prepared struct {
	event     Event
	chainedIP string
	payload   string
	salt      []byte
	data      *string
}

// prepare fills in what the caller left out, checks the event and splits off its
// personal part.
func prepare(e Event) (prepared, error) {
	if e.ID == uuid.Nil {
		e.ID = ids.New()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now()
	}
	// PostgreSQL stores microseconds; truncate so payload and column agree.
	e.OccurredAt = e.OccurredAt.UTC().Truncate(time.Microsecond)
	if err := e.validate(); err != nil {
		return prepared{event: e}, err
	}
	chained, part := splitPersonal(e)
	out := prepared{event: e, chainedIP: chained.IP}
	if !part.isEmpty() {
		body, err := json.Marshal(part)
		if err != nil {
			return out, fmt.Errorf("audit: encode: %w", err)
		}
		out.salt = make([]byte, personalSaltSize)
		if _, err := rand.Read(out.salt); err != nil {
			return out, fmt.Errorf("audit: salt: %w", err)
		}
		partData := string(body)
		out.data = &partData
		chained.PersonalDigest = personalDigest(out.salt, partData)
	}
	payload, err := json.Marshal(chained)
	if err != nil {
		return out, fmt.Errorf("audit: encode: %w", err)
	}
	out.payload = string(payload)
	return out, nil
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

// RecordSealed appends an event straight to the chain in its own transaction, under the
// lock of the chain, as every event was before ADR-041. It is for the rare events written
// before migrations run, when a database of an older release has no audit_pending yet:
// the marker of an upgrade and the note of a backup. Never use it on the path of a save.
func RecordSealed(ctx context.Context, pool *pgxpool.Pool, e Event) error {
	p, err := prepare(e)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", chainLockKey); err != nil {
			return fmt.Errorf("audit: lock chain: %w", err)
		}
		prev, err := chainHead(ctx, tx)
		if err != nil {
			return err
		}
		var seq int64
		err = tx.QueryRow(ctx, `INSERT INTO audit_events
			(id, occurred_at, actor_type, actor_id, action, object_type, object_id, space_id, result, ip, request_id, payload, prev_hash, hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING seq`,
			p.event.ID, p.event.OccurredAt, p.event.ActorType, p.event.ActorID, p.event.Action, p.event.ObjectType,
			p.event.ObjectID, p.event.SpaceID, p.event.Result, p.chainedIP, p.event.RequestID, p.payload,
			prev, chainHash(prev, []byte(p.payload))).Scan(&seq)
		if err != nil {
			return fmt.Errorf("audit: insert: %w", err)
		}
		if p.data != nil {
			if _, err := tx.Exec(ctx, "INSERT INTO audit_personal (seq, salt, data) VALUES ($1, $2, $3)", seq, p.salt, *p.data); err != nil {
				return fmt.Errorf("audit: insert personal part: %w", err)
			}
		}
		return nil
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
	// Unsealed counts the events that still wait for the sealer, so are not in the chain
	// that was checked yet (ADR-041).
	Unsealed int64 `json:"unsealed"`
}

// Verify walks the chain and reports the first inconsistency. After pruning it starts
// at the signed anchor that pruning left. It also counts the events that wait for the
// sealer; they are not part of the chain yet.
func Verify(ctx context.Context, q db.Querier) (VerifyResult, error) {
	res := VerifyResult{OK: true}
	if err := q.QueryRow(ctx, "SELECT count(*) FROM audit_pending").Scan(&res.Unsealed); err != nil {
		return VerifyResult{}, err
	}
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

// waitingSQL finds events that wait for the sealer, with the same filters. They are the
// newest there are, so a list shows them first and an export writes them last. Paging
// back ($5) only ever reaches sealed events.
const waitingSQL = `SELECT w.payload, w.data
	FROM audit_pending w
	WHERE $5::bigint = 0
	  AND ($1 = '' OR w.actor_id = $1)
	  AND ($2 = '' OR w.action = $2)
	  AND ($3::timestamptz IS NULL OR w.occurred_at >= $3)
	  AND ($4::timestamptz IS NULL OR w.occurred_at < $4)
	  AND ($7 = '' OR w.object_type = $7)
	  AND ($8 = '' OR w.space_id = $8)
	  AND ($9 = '' OR w.result = $9)
	  AND ($10 = '' OR w.object_id = $10)`

const listWaitingSQL = waitingSQL + " ORDER BY w.pend DESC LIMIT $6"
const exportWaitingSQL = waitingSQL + " ORDER BY w.pend LIMIT $6"

// scanned is one row of either table, before it is read for a reader.
type scanned struct {
	seq     int64
	sealed  bool
	payload string
	part    *string
}

// each runs one of the queries above with the filter's arguments and hands every row to
// fn as it comes, so an export of a million rows is never held whole. The sealed queries
// also give the seq. It returns how many rows it handed on.
func (f Filter) each(ctx context.Context, q db.Querier, sql string, sealed bool, limit int, fn func(scanned) error) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	rows, err := q.Query(ctx, sql, f.ActorID, f.Action, optTime(f.Since), optTime(f.Until), f.BeforeSeq, limit,
		f.ObjectType, f.SpaceID, f.Result, f.ObjectID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		s := scanned{sealed: sealed}
		if sealed {
			err = rows.Scan(&s.seq, &s.payload, &s.part)
		} else {
			err = rows.Scan(&s.payload, &s.part)
		}
		if err != nil {
			return n, err
		}
		if err := fn(s); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

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

// List returns matching events, newest first: those still waiting for the sealer, then
// the sealed ones.
func List(ctx context.Context, q db.Querier, f Filter) ([]Stored, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	var out []Stored
	keep := func(row scanned) error {
		event, err := f.read(row.payload, row.part)
		if err != nil {
			return fmt.Errorf("audit: decode seq %d: %w", row.seq, err)
		}
		out = append(out, Stored{Seq: row.seq, Sealed: row.sealed, Event: event})
		return nil
	}
	waiting, err := f.each(ctx, q, listWaitingSQL, false, f.Limit, keep)
	if err != nil {
		return nil, err
	}
	if _, err := f.each(ctx, q, listSQL, true, f.Limit-waiting, keep); err != nil {
		return nil, err
	}
	return out, nil
}

// CSVHeader is the column order of ExportCSV.
var CSVHeader = []string{"seq", "id", "occurred_at", "actor_type", "actor_id", "action", "object_type",
	"object_id", "space_id", "result", "ip", "user_agent", "request_id", "details"}

// ExportCSV writes matching events (oldest first, up to maxRows) as CSV. Events that still
// wait for the sealer come last, with an empty seq.
func ExportCSV(ctx context.Context, q db.Querier, f Filter, maxRows int, w io.Writer) (int, error) {
	cw := csv.NewWriter(w)
	if err := cw.Write(CSVHeader); err != nil {
		return 0, err
	}
	write := func(row scanned) error {
		e, err := f.read(row.payload, row.part)
		if err != nil {
			return fmt.Errorf("audit: decode seq %d: %w", row.seq, err)
		}
		details := ""
		if len(e.Details) > 0 {
			b, _ := json.Marshal(e.Details)
			details = string(b)
		}
		seq := ""
		if row.sealed {
			seq = strconv.FormatInt(row.seq, 10)
		}
		rec := []string{seq, e.ID.String(), e.OccurredAt.Format(time.RFC3339Nano),
			e.ActorType, e.ActorID, e.Action, e.ObjectType, e.ObjectID, e.SpaceID, e.Result, e.IP,
			e.UserAgent, e.RequestID, details}
		for i := range rec {
			rec[i] = csvout.Cell(rec[i])
		}
		return cw.Write(rec)
	}
	sealed, err := f.each(ctx, q, exportSQL, true, maxRows, write)
	if err != nil {
		return sealed, err
	}
	waiting, err := f.each(ctx, q, exportWaitingSQL, false, maxRows-sealed, write)
	n := sealed + waiting
	if err != nil {
		return n, err
	}
	cw.Flush()
	return n, cw.Error()
}
