// SPDX-License-Identifier: Apache-2.0

// Package setup guards the first setup of a new instance (spec 038). The setup screen
// creates the first administrator, so a stranger who reaches a fresh install first must not
// be able to use it. The screen is open only while no administrator exists, and only with a
// one-time code that the server writes to its log and `diffluence setup-code` prints: the
// person who installed the server can read it, a visitor cannot.
package setup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/ids"
)

// CodeLifetime is how long a code works. An install is usually finished within minutes,
// and a code that lies around in a log for days is a code someone else may find.
const CodeLifetime = time.Hour

// MaxWrongTries ends a code after this many wrong guesses, so it cannot be guessed even
// slowly. The person who installed the server asks for a new one.
const MaxWrongTries = 10

// codeBytes is 80 bits: far beyond guessing within MaxWrongTries.
const codeBytes = 10

// lockKey serialises every attempt to finish the setup, so two people cannot both
// become the first administrator.
const lockKey int64 = 0x6466_6c75_7365_7475 // "dflusetu"

var (
	// ErrClosed means the setup is over: an administrator exists, or a code was used.
	ErrClosed = errors.New("setup: closed")
	// ErrRefused means the code is missing, wrong, used, expired or out of tries. The
	// caller tells the person no more than that, so a guess learns nothing.
	ErrRefused = errors.New("setup: code refused")
)

// encoding has no letters that look alike in a terminal (no 0, 1, 8, 9).
var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// Attempt says where a request came from, for the audit log. It never holds the code.
type Attempt struct {
	IP        string
	UserAgent string
	RequestID string
}

// Service issues and checks setup codes.
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// New returns a Service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool, now: time.Now} }

// WithClock returns a copy that reads the time from now, for tests of expiry.
func (s *Service) WithClock(now func() time.Time) *Service { return &Service{pool: s.pool, now: now} }

// Open reports whether the first setup can still happen: no administrator exists and no
// code has been used. A grant to a group counts too, so a setup can never add a second way in.
func (s *Service) Open(ctx context.Context) (bool, error) {
	return open(ctx, s.pool)
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func open(ctx context.Context, q querier) (bool, error) {
	var closed bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM global_grants WHERE permission = 'administrator')
		OR EXISTS (SELECT 1 FROM setup_codes WHERE used_at IS NOT NULL)`).Scan(&closed)
	return !closed, err
}

// Issue makes a new code, which replaces every code issued before, and returns it once.
// Only its hash is stored. It refuses when the setup is over.
func (s *Service) Issue(ctx context.Context, by string) (string, error) {
	raw := make([]byte, codeBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	code := format(encoding.EncodeToString(raw))
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
			return err
		}
		ok, err := open(ctx, tx)
		if err != nil {
			return err
		}
		if !ok {
			return ErrClosed
		}
		if _, err := tx.Exec(ctx, "DELETE FROM setup_codes WHERE used_at IS NULL"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO setup_codes (id, code_hash, expires_at) VALUES ($1, $2, $3)",
			ids.New(), digest(code), s.now().Add(CodeLifetime)); err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, audit.Event{ActorType: audit.ActorSystem, ActorID: by,
			Action: "setup.code_issued", Result: audit.ResultSuccess})
		return err
	})
	if err != nil {
		return "", err
	}
	return code, nil
}

// Check tells whether a code would be accepted now, without using it. A wrong code counts
// as a wrong try, also when it is checked again later, and is audited without the code.
func (s *Service) Check(ctx context.Context, code string, from Attempt) error {
	var result error
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var verr error
		result, verr = s.verify(ctx, tx, code, false)
		if verr != nil {
			return verr
		}
		if result == nil {
			return nil
		}
		return recordRefusal(ctx, tx, result, from)
	})
	if err != nil {
		return err
	}
	return result
}

// Complete uses the code and runs create in the same transaction, under one lock, so the
// code works once and two people at the same moment cannot both finish. If create fails,
// the code stays unused.
func (s *Service) Complete(ctx context.Context, code string, from Attempt, create func(tx pgx.Tx) error) error {
	var result error
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var verr error
		result, verr = s.verify(ctx, tx, code, true)
		if verr != nil {
			return verr
		}
		if result != nil {
			// Undo the count of this try here; Check below counts it once, and audits it.
			return errRefusedInside
		}
		if err := create(tx); err != nil {
			return err
		}
		_, err := audit.Record(ctx, tx, audit.Event{ActorType: audit.ActorAnonymous, Action: "setup.completed",
			Result: audit.ResultSuccess, IP: from.IP, UserAgent: from.UserAgent, RequestID: from.RequestID})
		return err
	})
	if errors.Is(err, errRefusedInside) {
		return s.Check(ctx, code, from)
	}
	return err
}

// errRefusedInside rolls back a Complete that refused the code.
var errRefusedInside = errors.New("setup: refused inside the transaction")

// verify takes the lock and checks the code. The first result is the verdict, nil,
// ErrClosed or ErrRefused; the second is set only when the database fails. A wrong code
// raises the count of wrong tries; use marks a right code as used.
func (s *Service) verify(ctx context.Context, tx pgx.Tx, code string, use bool) (verdict, err error) {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
		return nil, err
	}
	ok, err := open(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return ErrClosed, nil
	}
	var (
		id        string
		hash      []byte
		expiresAt time.Time
		wrong     int
	)
	err = tx.QueryRow(ctx, `SELECT id, code_hash, expires_at, wrong_tries FROM setup_codes
		WHERE used_at IS NULL ORDER BY created_at DESC LIMIT 1 FOR UPDATE`).Scan(&id, &hash, &expiresAt, &wrong)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRefused, nil
	}
	if err != nil {
		return nil, err
	}
	given := digest(format(code))
	// Both are digests of the same length, so this compares in constant time.
	matches := subtle.ConstantTimeCompare(given, hash) == 1
	if !matches {
		if _, err := tx.Exec(ctx, "UPDATE setup_codes SET wrong_tries = wrong_tries + 1 WHERE id = $1", id); err != nil {
			return nil, err
		}
		return ErrRefused, nil
	}
	if wrong >= MaxWrongTries || !s.now().Before(expiresAt) {
		return ErrRefused, nil
	}
	if use {
		if _, err := tx.Exec(ctx, "UPDATE setup_codes SET used_at = $2 WHERE id = $1", id, s.now()); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func recordRefusal(ctx context.Context, tx pgx.Tx, verdict error, from Attempt) error {
	reason := "code_refused"
	if errors.Is(verdict, ErrClosed) {
		reason = "closed"
	}
	_, err := audit.Record(ctx, tx, audit.Event{ActorType: audit.ActorAnonymous, Action: "setup.refused",
		Result: audit.ResultDenied, IP: from.IP, UserAgent: from.UserAgent, RequestID: from.RequestID,
		Details: map[string]any{"reason": reason}})
	return err
}

// format makes a typed code comparable: upper case, without spaces or dashes, then in
// groups of four, as it was printed.
func format(code string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		case (r >= 'A' && r <= 'Z') || (r >= '2' && r <= '7'):
			return r
		}
		return -1
	}, code)
	var b strings.Builder
	for i, r := range clean {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func digest(code string) []byte {
	sum := sha256.Sum256([]byte("diffluence-setup:" + code))
	return sum[:]
}

// Announcement is the line a server logs while the setup is open, with the code in it.
// The code is meant for the log: whoever reads the log of the server installed it.
func Announcement(address, code string) string {
	return fmt.Sprintf("first setup: open %s/setup and enter the code %s (valid for %s; `diffluence setup-code` gives a new one)",
		strings.TrimRight(address, "/"), code, CodeLifetime)
}
