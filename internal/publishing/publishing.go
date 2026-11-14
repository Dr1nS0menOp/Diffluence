// SPDX-License-Identifier: Apache-2.0

// Package publishing owns draft and published spaces (spec 073): the pair of two spaces, the
// facts that say what is out of step, and releases. Writers work in the draft space; readers
// have the published space, which only a release changes. A twin in the published space is
// an ordinary page on every read path, so this package adds nothing to reading (PUB-203).
//
// Every method takes the Caller and checks permissions itself. The pages, attachments and
// spaces modules do the writing, inside the transaction of this one.
package publishing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/spaces"
)

// Limits (chapter 11).
const (
	// maxNoteRunes is the longest release note (PUB-1056).
	maxNoteRunes = 2000
	// previewLife is how long a preview may be confirmed (PUB-433).
	previewLife = time.Hour
)

// Service manages pairs and releases.
type Service struct {
	pool        *pgxpool.Pool
	perms       *permissions.Service
	pages       *pages.Service
	spaces      *spaces.Service
	attachments *attachments.Service
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, pg *pages.Service, sp *spaces.Service, att *attachments.Service) *Service {
	return &Service{pool: pool, perms: perms, pages: pg, spaces: sp, attachments: att}
}

// Settings are the choices of a pair, kept on the draft side (PUB-240). Who may release is
// not a setting of its own: it is whoever holds space.release in the draft space.
type Settings struct {
	// NotifyWatchers tells the watchers of a published page about a release.
	NotifyWatchers bool `json:"notify_watchers"`
	// MirrorRemovals takes off, in a whole-space release, what was removed in the draft. It
	// is used from PB2 on, and kept from the start so the choice is made once.
	MirrorRemovals bool `json:"mirror_removals"`
	// DefaultNote is the note a release starts with.
	DefaultNote string `json:"default_note"`
}

// defaultSettings are those of a new pair: watchers are told, removals are mirrored.
func defaultSettings() Settings {
	return Settings{NotifyWatchers: true, MirrorRemovals: true}
}

func (s Settings) clean() (Settings, error) {
	s.DefaultNote = strings.TrimSpace(s.DefaultNote)
	if err := checkNote(s.DefaultNote); err != nil {
		return s, err
	}
	return s, nil
}

func checkNote(note string) error {
	if !utf8.ValidString(note) || utf8.RuneCountInString(note) > maxNoteRunes || strings.ContainsRune(note, 0) {
		return apperr.Invalid(fmt.Sprintf("A release note has at most %d characters.", maxNoteRunes))
	}
	return nil
}

// pair is an active pair as the database holds it.
type pair struct {
	ID        uuid.UUID
	Draft     uuid.UUID
	Published uuid.UUID
	Settings  Settings
	CreatedAt time.Time
	CreatedBy uuid.UUID
}

const pairColumns = `id, draft_space_id, published_space_id, settings, created_at, created_by`

func scanPair(r pgx.Row) (pair, error) {
	var p pair
	var raw []byte
	if err := r.Scan(&p.ID, &p.Draft, &p.Published, &raw, &p.CreatedAt, &p.CreatedBy); err != nil {
		return p, err
	}
	p.Settings = defaultSettings()
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p.Settings); err != nil {
			return p, fmt.Errorf("publishing: read the settings of a pair: %v", err)
		}
	}
	return p, nil
}

// codeNotPaired answers a release asked of a space that is in no pair.
const codeNotPaired = "not_paired"

func errNotPaired() error {
	return apperr.Conflict(codeNotPaired, "This space is not linked to a published space.")
}

// hasCode says whether err is an error of this application with that code.
func hasCode(err error, code string) bool {
	e, ok := apperr.As(err)
	return ok && e.Code == code
}

// activePairOf finds the active pair a space is in, on either side. lock takes the row, so
// two runs or two changes of one pair take turns (PUB-1060).
func activePairOf(ctx context.Context, q db.Querier, spaceID uuid.UUID, lock bool) (pair, error) {
	sql := `SELECT ` + pairColumns + ` FROM space_pairs WHERE (draft_space_id = $1 OR published_space_id = $1) AND ended_at IS NULL`
	if lock {
		sql += " FOR UPDATE"
	}
	p, err := scanPair(q.QueryRow(ctx, sql, spaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, errNotPaired()
	}
	return p, err
}
