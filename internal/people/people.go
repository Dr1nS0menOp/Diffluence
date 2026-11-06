// SPDX-License-Identifier: Apache-2.0

// Package people is the directory of colleagues: who can be found, and what a
// person lets others see (spec 013, F4). Lists go through perm_visible_people.
package people

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/storage"
)

// Who may see a field of a profile.
const (
	VisibleEveryone = "everyone"
	VisibleMembers  = "members"
	VisibleMe       = "me"
)

// The fields a person can show or hide.
var profileFields = []string{"email", "job_title", "department", "location", "about"}

const (
	defaultRows     = 50
	maxRows         = 100
	maxFieldRunes   = 200
	maxAboutRunes   = 2000
	maxPictureBytes = 5 << 20
)

// Row is a person in the directory.
type Row struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Active     bool      `json:"active"`
	JobTitle   string    `json:"job_title"`
	Department string    `json:"department"`
	HasPicture bool      `json:"has_picture"`
}

// Profile is what a viewer may see of a person.
type Profile struct {
	Row
	Email        string            `json:"email"`
	Location     string            `json:"location"`
	About        string            `json:"about"`
	TimeZone     string            `json:"time_zone"`
	Visibility   map[string]string `json:"visibility,omitempty"`
	LockedFields []string          `json:"locked_fields,omitempty"`
	IsSelf       bool              `json:"is_self"`
}

// Service is the people directory.
type Service struct {
	pool     *pgxpool.Pool
	perms    *permissions.Service
	store    storage.Store
	identity *identity.Service
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, store storage.Store, id *identity.Service) *Service {
	return &Service{pool: pool, perms: perms, store: store, identity: id}
}

// Query asks the directory for a part of the list.
type Query struct {
	Text    string
	GroupID uuid.UUID
	Limit   int
	Offset  int
}

type stored struct {
	Row
	email      string
	location   string
	about      string
	pictureKey string
	visibility map[string]string
	locked     []string
	anonymised bool
}

func defaultVisibility() map[string]string {
	return map[string]string{"email": VisibleMembers, "job_title": VisibleEveryone, "department": VisibleEveryone,
		"location": VisibleEveryone, "about": VisibleEveryone}
}

const personSQL = `SELECT u.id, u.display_name, u.type, u.status = 'active', u.email, u.anonymised_at IS NOT NULL,
		COALESCE(p.job_title, ''), COALESCE(p.department, ''), COALESCE(p.location, ''), COALESCE(p.about, ''),
		COALESCE(p.picture_key, ''), COALESCE(p.visibility, '{}'), COALESCE(p.locked_fields, '{}')
	FROM users u LEFT JOIN user_profiles p ON p.user_id = u.id`

func scanPerson(r pgx.Row) (stored, error) {
	var v stored
	var raw []byte
	err := r.Scan(&v.ID, &v.Name, &v.Type, &v.Active, &v.email, &v.anonymised, &v.JobTitle, &v.Department,
		&v.location, &v.about, &v.pictureKey, &raw, &v.locked)
	if err != nil {
		return v, err
	}
	v.visibility = defaultVisibility()
	if err := json.Unmarshal(raw, &v.visibility); err != nil {
		return v, fmt.Errorf("people: read visibility: %v", err)
	}
	v.HasPicture = v.pictureKey != ""
	return v, nil
}

// shows says whether a viewer may see one field. The person themselves always may.
func shows(v stored, field string, viewerType string, self bool) bool {
	switch v.visibility[field] {
	case VisibleMe:
		return self
	case VisibleMembers:
		return self || viewerType == identity.TypeMember
	}
	return true
}

func (v stored) row(viewerType string, self bool) Row {
	out := v.Row
	if !shows(v, "job_title", viewerType, self) {
		out.JobTitle = ""
	}
	if !shows(v, "department", viewerType, self) {
		out.Department = ""
	}
	return out
}

func (s *Service) viewerType(ctx context.Context, c identity.Caller) (string, error) {
	u, err := s.perms.LoadUser(ctx, s.pool, c.UserID)
	return u.Type, err
}

// List returns active people the caller may see, and how many there are.
func (s *Service) List(ctx context.Context, c identity.Caller, q Query) ([]Row, int, error) {
	if q.Limit <= 0 || q.Limit > maxRows {
		q.Limit = defaultRows
	}
	viewer, err := s.viewerType(ctx, c)
	if err != nil {
		return nil, 0, err
	}
	var group *uuid.UUID
	if q.GroupID != uuid.Nil {
		group = &q.GroupID
	}
	const where = ` JOIN perm_visible_people($1) v ON v.user_id = u.id
		WHERE u.status = 'active' AND u.anonymised_at IS NULL
		  AND ($2 = '' OR position(lower($2) IN lower(u.display_name)) > 0)
		  AND ($3::uuid IS NULL OR u.id IN (SELECT m.user_id FROM group_members m WHERE m.group_id = $3))`
	var total int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM users u"+where, c.UserID, q.Text, group).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, personSQL+where+" ORDER BY lower(u.display_name), u.id LIMIT $4 OFFSET $5",
		c.UserID, q.Text, group, q.Limit, max(q.Offset, 0))
	if err != nil {
		return nil, 0, err
	}
	people, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (stored, error) { return scanPerson(r) })
	if err != nil {
		return nil, 0, err
	}
	out := make([]Row, 0, len(people))
	for _, p := range people {
		out = append(out, p.row(viewer, p.ID == c.UserID))
	}
	return out, total, nil
}

// require answers "not found" for a person the caller may not see.
func (s *Service) require(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	ok, err := s.perms.CheckPerson(ctx, s.pool, c.UserID, id)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.NotFound()
	}
	return nil
}

func (s *Service) load(ctx context.Context, id uuid.UUID) (stored, error) {
	v, err := scanPerson(s.pool.QueryRow(ctx, personSQL+" WHERE u.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, apperr.NotFound()
	}
	return v, err
}

// Get returns what the caller may see of a person.
func (s *Service) Get(ctx context.Context, c identity.Caller, id uuid.UUID) (Profile, error) {
	if err := s.require(ctx, c, id); err != nil {
		return Profile{}, err
	}
	v, err := s.load(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	viewer, err := s.viewerType(ctx, c)
	if err != nil {
		return Profile{}, err
	}
	self := id == c.UserID
	out := Profile{Row: v.row(viewer, self), IsSelf: self}
	if shows(v, "email", viewer, self) && !v.anonymised {
		out.Email = v.email
	}
	if shows(v, "location", viewer, self) {
		out.Location = v.location
	}
	if shows(v, "about", viewer, self) {
		out.About = v.about
	}
	preferences, err := s.identity.Preferences(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	out.TimeZone = preferences.TimeZone
	if self {
		out.Visibility, out.LockedFields = v.visibility, v.locked
	}
	return out, nil
}

// Input is what a person can change about their own profile.
type Input struct {
	JobTitle   string            `json:"job_title"`
	Department string            `json:"department"`
	Location   string            `json:"location"`
	About      string            `json:"about"`
	Visibility map[string]string `json:"visibility"`
}

func (in *Input) validate() error {
	in.JobTitle, in.Department, in.Location = strings.TrimSpace(in.JobTitle), strings.TrimSpace(in.Department), strings.TrimSpace(in.Location)
	in.About = strings.TrimSpace(in.About)
	for _, field := range []string{in.JobTitle, in.Department, in.Location} {
		if utf8.RuneCountInString(field) > maxFieldRunes || strings.ContainsAny(field, "\r\n\x00") {
			return apperr.Invalid("A field is one line of at most 200 characters.")
		}
	}
	if utf8.RuneCountInString(in.About) > maxAboutRunes || strings.Contains(in.About, "\x00") {
		return apperr.Invalid("The text about you has at most 2000 characters.")
	}
	for field, who := range in.Visibility {
		if !slices.Contains(profileFields, field) || !slices.Contains([]string{VisibleEveryone, VisibleMembers, VisibleMe}, who) {
			return apperr.Invalid("That is not a choice for who may see a field.")
		}
	}
	return nil
}

// SetOwn changes the caller's profile. A field an identity provider fills stays as
// it is. A profile is personal, so a change is not audited.
func (s *Service) SetOwn(ctx context.Context, c identity.Caller, in Input) error {
	if err := in.validate(); err != nil {
		return err
	}
	current, err := s.load(ctx, c.UserID)
	if err != nil {
		return err
	}
	if slices.Contains(current.locked, "job_title") {
		in.JobTitle = current.JobTitle
	}
	if slices.Contains(current.locked, "department") {
		in.Department = current.Department
	}
	visibility, err := json.Marshal(in.Visibility)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO user_profiles (user_id, job_title, department, location, about, visibility)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id) DO UPDATE SET job_title = EXCLUDED.job_title, department = EXCLUDED.department,
			location = EXCLUDED.location, about = EXCLUDED.about, visibility = EXCLUDED.visibility, updated_at = now()`,
		c.UserID, in.JobTitle, in.Department, in.Location, in.About, visibility)
	return err
}

// SetFromProvider stores what an identity provider says, and locks those fields.
func (s *Service) SetFromProvider(ctx context.Context, userID uuid.UUID, jobTitle, department string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO user_profiles (user_id, job_title, department, locked_fields)
		VALUES ($1, $2, $3, '{job_title,department}')
		ON CONFLICT (user_id) DO UPDATE SET job_title = EXCLUDED.job_title, department = EXCLUDED.department,
			locked_fields = EXCLUDED.locked_fields, updated_at = now()`, userID, jobTitle, department)
	return err
}

// Crop is a square part of an uploaded picture, in its own pixels.
type Crop struct {
	X, Y, Size int
}

// SetPicture stores the caller's picture, as a new PNG made from the crop.
func (s *Service) SetPicture(ctx context.Context, c identity.Caller, body io.Reader, crop Crop) error {
	data, err := io.ReadAll(io.LimitReader(body, maxPictureBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxPictureBytes {
		return apperr.TooLarge("A picture can be at most 5 MiB.")
	}
	clean, err := reencode(data, crop)
	if err != nil {
		return err
	}
	key := "pictures/" + ids.New().String()
	if _, err := s.store.Put(ctx, key, bytes.NewReader(clean), maxPictureBytes); err != nil {
		return fmt.Errorf("people: store picture: %v", err)
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := queueOldPicture(ctx, tx, c.UserID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO user_profiles (user_id, picture_key) VALUES ($1, $2)
			ON CONFLICT (user_id) DO UPDATE SET picture_key = EXCLUDED.picture_key, updated_at = now()`, c.UserID, key)
		return err
	})
}

func queueOldPicture(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	var old string
	err := tx.QueryRow(ctx, "SELECT picture_key FROM user_profiles WHERE user_id = $1", userID).Scan(&old)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && old == "") {
		return nil
	}
	if err != nil {
		return err
	}
	return storage.QueueDeletion(ctx, tx, old)
}

// RemovePicture goes back to initials.
func (s *Service) RemovePicture(ctx context.Context, c identity.Caller) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := queueOldPicture(ctx, tx, c.UserID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE user_profiles SET picture_key = '', updated_at = now() WHERE user_id = $1", c.UserID)
		return err
	})
}

// OpenPicture returns a person's picture for someone who may see them.
func (s *Service) OpenPicture(ctx context.Context, c identity.Caller, id uuid.UUID) (io.ReadSeekCloser, time.Time, error) {
	if err := s.require(ctx, c, id); err != nil {
		return nil, time.Time{}, err
	}
	var key string
	var changed time.Time
	err := s.pool.QueryRow(ctx, "SELECT picture_key, updated_at FROM user_profiles WHERE user_id = $1 AND picture_key <> ''", id).Scan(&key, &changed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, changed, apperr.NotFound()
	}
	if err != nil {
		return nil, changed, err
	}
	f, err := s.store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, changed, apperr.NotFound()
	}
	return f, changed, err
}

// Forget removes what the directory keeps about a person, for anonymising. It
// runs before the account itself changes.
func (s *Service) Forget(ctx context.Context, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := queueOldPicture(ctx, tx, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM user_profiles WHERE user_id = $1", id)
		return err
	})
}
