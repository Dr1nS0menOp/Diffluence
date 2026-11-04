// SPDX-License-Identifier: Apache-2.0

// Package scim is a SCIM 2.0 service provider (RFC 7643, RFC 7644, spec 004) for
// provisioning from Authentik, Entra ID and others. It translates the protocol
// and leaves every actual change to the identity module, so SCIM changes are
// audited and checked like all others.
package scim

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
)

// Schema URNs.
const (
	schemaUser    = "urn:ietf:params:scim:schemas:core:2.0:User"
	schemaGroup   = "urn:ietf:params:scim:schemas:core:2.0:Group"
	schemaList    = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	schemaError   = "urn:ietf:params:scim:api:messages:2.0:Error"
	schemaConfig  = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	schemaResType = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	schemaSchema  = "urn:ietf:params:scim:schemas:core:2.0:Schema"
	contentType   = "application/scim+json"
	maxBody       = 2 << 20
	tokenPrefix   = "dfl_scim_"
)

// Token is a bearer token, as the admin console shows it.
type Token struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// Service handles SCIM requests.
type Service struct {
	pool     *pgxpool.Pool
	identity *identity.Service
	baseURL  string
	log      *slog.Logger
}

// NewService creates the service. baseURL is the public URL of this installation.
func NewService(pool *pgxpool.Pool, id *identity.Service, baseURL string, log *slog.Logger) *Service {
	return &Service{pool: pool, identity: id, baseURL: strings.TrimRight(baseURL, "/") + "/scim/v2", log: log}
}

func hash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// CreateToken makes a new token. It is returned once and stored only as a hash.
func (s *Service) CreateToken(ctx context.Context, c identity.Caller, name string) (Token, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return Token{}, "", apperr.Invalid("Give the token a name.")
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	secret := tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	t := Token{ID: ids.New(), Name: name}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, "INSERT INTO scim_tokens (id, name, token_hash, created_by) VALUES ($1, $2, $3, $4) RETURNING created_at",
			t.ID, name, hash(secret), c.UserID).Scan(&t.CreatedAt)
		if err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("scim.token_created", "scim_token", t.ID.String(), "", map[string]any{"name": name}))
		return err
	})
	return t, secret, err
}

// Tokens lists all tokens.
func (s *Service) Tokens(ctx context.Context) ([]Token, error) {
	rows, err := s.pool.Query(ctx, "SELECT id, name, created_at, last_used_at, revoked_at FROM scim_tokens ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Token, error) {
		var t Token
		err := r.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt)
		return t, err
	})
	if out == nil {
		out = []Token{}
	}
	return out, err
}

// RevokeToken stops a token at once.
func (s *Service) RevokeToken(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE scim_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL", id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		_, err = audit.Record(ctx, tx, c.Event("scim.token_revoked", "scim_token", id.String(), "", nil))
		return err
	})
}

// Authenticate returns the caller for a bearer token.
func (s *Service) Authenticate(r *http.Request) (identity.Caller, bool) {
	header := r.Header.Get("Authorization")
	if len(header) < 7 || !strings.EqualFold(header[:7], "bearer ") {
		return identity.Caller{}, false
	}
	token := strings.TrimSpace(header[7:])
	if !strings.HasPrefix(token, tokenPrefix) {
		return identity.Caller{}, false
	}
	var id uuid.UUID
	var used *time.Time
	err := s.pool.QueryRow(r.Context(), "SELECT id, last_used_at FROM scim_tokens WHERE token_hash = $1 AND revoked_at IS NULL", hash(token)).Scan(&id, &used)
	if err != nil {
		return identity.Caller{}, false
	}
	if used == nil || time.Since(*used) > time.Minute {
		_, _ = s.pool.Exec(r.Context(), "UPDATE scim_tokens SET last_used_at = now() WHERE id = $1", id)
	}
	return identity.SystemCaller("scim:" + id.String()), true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError sends a SCIM error.
func WriteError(w http.ResponseWriter, status int, scimType, detail string) {
	body := map[string]any{"schemas": []string{schemaError}, "status": strconv.Itoa(status), "detail": detail}
	if scimType != "" {
		body["scimType"] = scimType
	}
	writeJSON(w, status, body)
}

func (s *Service) fail(w http.ResponseWriter, err error) {
	if e, ok := apperr.As(err); ok {
		switch e.Kind {
		case apperr.KindNotFound:
			WriteError(w, http.StatusNotFound, "", "Resource not found.")
		case apperr.KindConflict:
			WriteError(w, http.StatusConflict, "uniqueness", e.Message)
		default:
			WriteError(w, http.StatusBadRequest, "invalidValue", e.Message)
		}
		return
	}
	s.log.Error("scim request failed", "error", err)
	WriteError(w, http.StatusInternalServerError, "", "Something went wrong.")
}

func read(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err == nil {
		err = json.Unmarshal(body, dst)
	}
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalidSyntax", "The request body is not valid JSON.")
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusNotFound, "", "Resource not found.")
		return id, false
	}
	return id, true
}

// paging reads startIndex (1-based) and count.
func paging(r *http.Request) (start, count int) {
	start, count = 1, 100
	if v, err := strconv.Atoi(r.URL.Query().Get("startIndex")); err == nil && v > 0 {
		start = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("count")); err == nil && v >= 0 {
		count = min(v, 500)
	}
	return start, count
}

var (
	clausePattern = regexp.MustCompile(`(?i)^\s*([A-Za-z.\[\]" ]+?)\s+eq\s+"((?:[^"\\]|\\.)*)"\s*$`)
	andSplit      = regexp.MustCompile(`(?i)\s+and\s+`)
)

// parseFilter understands "attr eq value", joined by "and": what provisioning
// clients actually send. Anything else is refused and not silently ignored,
// because an ignored filter returns everyone and creates duplicates.
func parseFilter(filter string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(filter) == "" {
		return out, nil
	}
	for _, clause := range andSplit.Split(filter, -1) {
		m := clausePattern.FindStringSubmatch(clause)
		if m == nil {
			return nil, errors.New("unsupported filter")
		}
		var value string
		if err := json.Unmarshal([]byte(`"`+m[2]+`"`), &value); err != nil {
			return nil, errors.New("bad filter value")
		}
		attr := strings.ToLower(strings.TrimSpace(m[1]))
		if strings.HasPrefix(attr, "emails") {
			attr = "emails.value"
		}
		out[attr] = value
	}
	return out, nil
}

type userResource struct {
	Schemas     []string `json:"schemas"`
	ID          string   `json:"id,omitempty"`
	ExternalID  string   `json:"externalId,omitempty"`
	UserName    string   `json:"userName"`
	DisplayName string   `json:"displayName,omitempty"`
	Name        *name    `json:"name,omitempty"`
	Emails      []email  `json:"emails,omitempty"`
	Active      *boolish `json:"active,omitempty"`
	Meta        *meta    `json:"meta,omitempty"`
}

type name struct {
	Formatted  string `json:"formatted,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

type email struct {
	Value   string   `json:"value"`
	Type    string   `json:"type,omitempty"`
	Primary *boolish `json:"primary,omitempty"`
}

type meta struct {
	ResourceType string    `json:"resourceType"`
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	Location     string    `json:"location"`
}

// boolish accepts true and "True": some clients send booleans as strings.
type boolish bool

// UnmarshalJSON implements json.Unmarshaler.
func (b *boolish) UnmarshalJSON(data []byte) error {
	switch strings.ToLower(strings.Trim(string(data), `"`)) {
	case "true":
		*b = true
	case "false":
		*b = false
	default:
		return fmt.Errorf("not a boolean: %s", data)
	}
	return nil
}

func boolPtr(v bool) *boolish {
	b := boolish(v)
	return &b
}

func (s *Service) userOut(u identity.UserDetail) userResource {
	userName := u.Username
	if userName == "" {
		userName = u.Email
	}
	return userResource{Schemas: []string{schemaUser}, ID: u.ID.String(), ExternalID: u.ExternalID, UserName: userName,
		DisplayName: u.DisplayName, Name: &name{Formatted: u.DisplayName},
		Emails: []email{{Value: u.Email, Type: "work", Primary: boolPtr(true)}},
		Active: boolPtr(u.Status == identity.StatusActive),
		Meta:   &meta{ResourceType: "User", Created: u.CreatedAt, LastModified: u.UpdatedAt, Location: s.baseURL + "/Users/" + u.ID.String()}}
}

func (u userResource) external() identity.External {
	e := identity.External{Username: u.UserName, ExternalID: u.ExternalID, Source: identity.SourceSCIM, Active: true}
	if u.Active != nil {
		e.Active = bool(*u.Active)
	}
	for _, m := range u.Emails {
		if e.Email == "" || (m.Primary != nil && bool(*m.Primary)) {
			e.Email = m.Value
		}
	}
	if e.Email == "" {
		e.Email = u.UserName // Entra ID: the user name is the e-mail address
	}
	e.DisplayName = u.DisplayName
	if e.DisplayName == "" && u.Name != nil {
		e.DisplayName = u.Name.Formatted
		if e.DisplayName == "" {
			e.DisplayName = strings.TrimSpace(u.Name.GivenName + " " + u.Name.FamilyName)
		}
	}
	return e
}

func listResponse(resources any, total, start, count int) map[string]any {
	return map[string]any{"schemas": []string{schemaList}, "totalResults": total, "startIndex": start, "itemsPerPage": count, "Resources": resources}
}

// ListUsers handles GET /Users.
func (s *Service) ListUsers(w http.ResponseWriter, r *http.Request) {
	filter, err := parseFilter(r.URL.Query().Get("filter"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalidFilter", "Only 'attribute eq \"value\"' filters are supported.")
		return
	}
	start, count := paging(r)
	if id, ok := filter["id"]; ok {
		s.listOneUser(w, r, id, start)
		return
	}
	f := identity.UserFilter{ExternalID: filter["externalid"], Email: filter["emails.value"], Limit: max(count, 1), Offset: start - 1}
	userName, hasUserName := filter["username"]
	users, total, err := s.identity.ListUsers(r.Context(), f)
	if err != nil {
		s.fail(w, err)
		return
	}
	if hasUserName {
		// A user name is the stored one or, for accounts without one, the e-mail address.
		f.Username = userName
		if users, total, err = s.identity.ListUsers(r.Context(), f); err != nil {
			s.fail(w, err)
			return
		}
		if total == 0 {
			f.Username, f.Email = "", userName
			if users, total, err = s.identity.ListUsers(r.Context(), f); err != nil {
				s.fail(w, err)
				return
			}
		}
	}
	out := make([]userResource, 0, len(users))
	if count > 0 {
		for _, u := range users {
			out = append(out, s.userOut(u))
		}
	}
	writeJSON(w, http.StatusOK, listResponse(out, total, start, len(out)))
}

func (s *Service) listOneUser(w http.ResponseWriter, r *http.Request, raw string, start int) {
	out := []userResource{}
	if id, err := uuid.Parse(raw); err == nil {
		if u, err := s.identity.GetUser(r.Context(), s.pool, id); err == nil {
			out = append(out, s.userOut(u))
		} else if !apperr.IsNotFound(err) {
			s.fail(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, listResponse(out, len(out), start, len(out)))
}

// GetUser handles GET /Users/{id}.
func (s *Service) GetUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, err := s.identity.GetUser(r.Context(), s.pool, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.userOut(u))
}

// CreateUser handles POST /Users.
func (s *Service) CreateUser(c identity.Caller, w http.ResponseWriter, r *http.Request) {
	var in userResource
	if !read(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.UserName) == "" {
		WriteError(w, http.StatusBadRequest, "invalidValue", "userName is required.")
		return
	}
	var id uuid.UUID
	err := pgx.BeginFunc(r.Context(), s.pool, func(tx pgx.Tx) error {
		var err error
		id, err = s.identity.CreateExternalUser(r.Context(), tx, c, in.external())
		return err
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	u, err := s.identity.GetUser(r.Context(), s.pool, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Location", s.baseURL+"/Users/"+id.String())
	writeJSON(w, http.StatusCreated, s.userOut(u))
}

// ReplaceUser handles PUT /Users/{id}.
func (s *Service) ReplaceUser(c identity.Caller, w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in userResource
	if !read(w, r, &in) {
		return
	}
	s.applyUser(c, w, r, id, in.external())
}

func (s *Service) applyUser(c identity.Caller, w http.ResponseWriter, r *http.Request, id uuid.UUID, e identity.External) {
	if err := s.identity.UpdateExternalUser(r.Context(), c, id, e); err != nil {
		s.fail(w, err)
		return
	}
	u, err := s.identity.GetUser(r.Context(), s.pool, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.userOut(u))
}

type patchRequest struct {
	Operations []struct {
		Op    string          `json:"op"`
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value"`
	} `json:"Operations"`
}

func patchString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}

// PatchUser handles PATCH /Users/{id}.
func (s *Service) PatchUser(c identity.Caller, w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in patchRequest
	if !read(w, r, &in) {
		return
	}
	u, err := s.identity.GetUser(r.Context(), s.pool, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	e := identity.External{Email: u.Email, DisplayName: u.DisplayName, Username: u.Username, ExternalID: u.ExternalID,
		Source: identity.SourceSCIM, Active: u.Status == identity.StatusActive}
	apply := func(path string, value json.RawMessage) error {
		switch p := strings.ToLower(path); {
		case p == "active":
			var b boolish
			if err := json.Unmarshal(value, &b); err != nil {
				return err
			}
			e.Active = bool(b)
		case p == "username":
			e.Username = patchString(value)
		case p == "displayname" || p == "name.formatted":
			e.DisplayName = patchString(value)
		case p == "externalid":
			e.ExternalID = patchString(value)
		case strings.HasPrefix(p, "emails"):
			var list []email
			if json.Unmarshal(value, &list) == nil && len(list) > 0 {
				e.Email = list[0].Value
			} else {
				e.Email = patchString(value)
			}
		}
		return nil // attributes we do not store are ignored, as RFC 7644 allows
	}
	for _, op := range in.Operations {
		if strings.EqualFold(op.Op, "remove") {
			continue // nothing we store can be removed: every field is required
		}
		if op.Path != "" {
			if err := apply(op.Path, op.Value); err != nil {
				WriteError(w, http.StatusBadRequest, "invalidValue", "A value is not valid.")
				return
			}
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(op.Value, &obj); err != nil {
			WriteError(w, http.StatusBadRequest, "invalidValue", "A value is not valid.")
			return
		}
		for k, v := range obj {
			if err := apply(k, v); err != nil {
				WriteError(w, http.StatusBadRequest, "invalidValue", "A value is not valid.")
				return
			}
		}
	}
	s.applyUser(c, w, r, id, e)
}

// DeleteUser handles DELETE /Users/{id}. Accounts are disabled and not removed:
// content keeps its author and the audit trail stays readable.
func (s *Service) DeleteUser(c identity.Caller, w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.identity.SetUserStatus(r.Context(), c, id, identity.StatusDisabled); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type memberRef struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
	Ref     string `json:"$ref,omitempty"`
}

type groupResource struct {
	Schemas     []string    `json:"schemas"`
	ID          string      `json:"id,omitempty"`
	ExternalID  string      `json:"externalId,omitempty"`
	DisplayName string      `json:"displayName"`
	Members     []memberRef `json:"members"`
	Meta        *meta       `json:"meta,omitempty"`
}

func (s *Service) groupOut(ctx context.Context, g identity.Group, withMembers bool) (groupResource, error) {
	out := groupResource{Schemas: []string{schemaGroup}, ID: g.ID.String(), ExternalID: g.ExternalID, DisplayName: g.Name, Members: []memberRef{},
		Meta: &meta{ResourceType: "Group", Created: g.CreatedAt, LastModified: g.UpdatedAt, Location: s.baseURL + "/Groups/" + g.ID.String()}}
	if !withMembers {
		return out, nil
	}
	members, err := s.identity.GroupMembers(ctx, g.ID)
	if err != nil {
		return out, err
	}
	for _, m := range members {
		out.Members = append(out.Members, memberRef{Value: m.ID.String(), Display: m.DisplayName, Ref: s.baseURL + "/Users/" + m.ID.String()})
	}
	return out, nil
}

func excludesMembers(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.URL.Query().Get("excludedAttributes")), "members")
}

// ListGroups handles GET /Groups.
func (s *Service) ListGroups(w http.ResponseWriter, r *http.Request) {
	filter, err := parseFilter(r.URL.Query().Get("filter"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalidFilter", "Only 'attribute eq \"value\"' filters are supported.")
		return
	}
	start, count := paging(r)
	var groups []identity.Group
	var total int
	if raw, ok := filter["id"]; ok {
		// Entra ID checks membership with: id eq "x" and members.value eq "y".
		id, err := uuid.Parse(raw)
		if err == nil {
			g, err := s.identity.GetGroup(r.Context(), s.pool, id)
			switch {
			case err == nil:
				groups, total = []identity.Group{g}, 1
			case !apperr.IsNotFound(err):
				s.fail(w, err)
				return
			}
		}
		if member, ok := filter["members.value"]; ok && total == 1 {
			members, err := s.identity.GroupMembers(r.Context(), groups[0].ID)
			if err != nil {
				s.fail(w, err)
				return
			}
			found := false
			for _, m := range members {
				found = found || m.ID.String() == member
			}
			if !found {
				groups, total = nil, 0
			}
		}
	} else {
		var err error
		groups, total, err = s.identity.ListGroups(r.Context(), identity.GroupFilter{Name: filter["displayname"], ExternalID: filter["externalid"],
			Limit: max(count, 1), Offset: start - 1})
		if err != nil {
			s.fail(w, err)
			return
		}
	}
	out := make([]groupResource, 0, len(groups))
	if count > 0 {
		for _, g := range groups {
			res, err := s.groupOut(r.Context(), g, !excludesMembers(r))
			if err != nil {
				s.fail(w, err)
				return
			}
			out = append(out, res)
		}
	}
	writeJSON(w, http.StatusOK, listResponse(out, total, start, len(out)))
}

// GetGroup handles GET /Groups/{id}.
func (s *Service) GetGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.sendGroup(w, r, id, http.StatusOK)
}

func (s *Service) sendGroup(w http.ResponseWriter, r *http.Request, id uuid.UUID, status int) {
	g, err := s.identity.GetGroup(r.Context(), s.pool, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.groupOut(r.Context(), g, !excludesMembers(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, status, res)
}

func memberIDs(refs []memberRef) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(refs))
	for _, m := range refs {
		id, err := uuid.Parse(m.Value)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// CreateGroup handles POST /Groups.
func (s *Service) CreateGroup(c identity.Caller, w http.ResponseWriter, r *http.Request) {
	var in groupResource
	if !read(w, r, &in) {
		return
	}
	members, err := memberIDs(in.Members)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalidValue", "A member is not valid.")
		return
	}
	g, err := s.identity.CreateGroup(r.Context(), c, identity.NewGroup{Name: in.DisplayName, Source: identity.SourceSCIM, ExternalID: in.ExternalID})
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.identity.SetGroupMembers(r.Context(), c, g.ID, members); err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Location", s.baseURL+"/Groups/"+g.ID.String())
	s.sendGroup(w, r, g.ID, http.StatusCreated)
}

// ReplaceGroup handles PUT /Groups/{id}.
func (s *Service) ReplaceGroup(c identity.Caller, w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in groupResource
	if !read(w, r, &in) {
		return
	}
	members, err := memberIDs(in.Members)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalidValue", "A member is not valid.")
		return
	}
	if _, err := s.identity.UpdateGroup(r.Context(), c, id, identity.NewGroup{Name: in.DisplayName, ExternalID: in.ExternalID}); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.identity.SetGroupMembers(r.Context(), c, id, members); err != nil {
		s.fail(w, err)
		return
	}
	s.sendGroup(w, r, id, http.StatusOK)
}

var memberPath = regexp.MustCompile(`(?i)^members\[\s*value\s+eq\s+"([^"]+)"\s*\]$`)

// PatchGroup handles PATCH /Groups/{id}.
func (s *Service) PatchGroup(c identity.Caller, w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in patchRequest
	if !read(w, r, &in) {
		return
	}
	ctx := r.Context()
	bad := func() { WriteError(w, http.StatusBadRequest, "invalidValue", "A value is not valid.") }
	for _, op := range in.Operations {
		kind, path := strings.ToLower(op.Op), strings.TrimSpace(op.Path)
		var refs []memberRef
		if strings.EqualFold(path, "members") && len(op.Value) > 0 && json.Unmarshal(op.Value, &refs) != nil {
			bad()
			return
		}
		members, err := memberIDs(refs)
		if err != nil {
			bad()
			return
		}
		switch {
		case strings.EqualFold(path, "members") && kind == "add":
			for _, m := range members {
				if err := s.identity.AddGroupMember(ctx, c, id, m); err != nil {
					s.fail(w, err)
					return
				}
			}
		case strings.EqualFold(path, "members") && kind == "replace":
			if err := s.identity.SetGroupMembers(ctx, c, id, members); err != nil {
				s.fail(w, err)
				return
			}
		case strings.EqualFold(path, "members") && kind == "remove":
			if len(op.Value) == 0 {
				if err := s.identity.SetGroupMembers(ctx, c, id, nil); err != nil {
					s.fail(w, err)
					return
				}
				continue
			}
			for _, m := range members {
				if err := s.identity.RemoveGroupMember(ctx, c, id, m); err != nil {
					s.fail(w, err)
					return
				}
			}
		case memberPath.MatchString(path) && kind == "remove":
			m, err := uuid.Parse(memberPath.FindStringSubmatch(path)[1])
			if err != nil {
				bad()
				return
			}
			if err := s.identity.RemoveGroupMember(ctx, c, id, m); err != nil {
				s.fail(w, err)
				return
			}
		case strings.EqualFold(path, "displayName"):
			if err := s.renameGroup(ctx, c, id, patchString(op.Value)); err != nil {
				s.fail(w, err)
				return
			}
		case path == "":
			var obj struct {
				DisplayName string      `json:"displayName"`
				Members     []memberRef `json:"members"`
			}
			if json.Unmarshal(op.Value, &obj) != nil {
				bad()
				return
			}
			if obj.DisplayName != "" {
				if err := s.renameGroup(ctx, c, id, obj.DisplayName); err != nil {
					s.fail(w, err)
					return
				}
			}
			ids, err := memberIDs(obj.Members)
			if err != nil {
				bad()
				return
			}
			for _, m := range ids {
				if err := s.identity.AddGroupMember(ctx, c, id, m); err != nil {
					s.fail(w, err)
					return
				}
			}
		}
	}
	s.sendGroup(w, r, id, http.StatusOK)
}

func (s *Service) renameGroup(ctx context.Context, c identity.Caller, id uuid.UUID, name string) error {
	g, err := s.identity.GetGroup(ctx, s.pool, id)
	if err != nil {
		return err
	}
	_, err = s.identity.UpdateGroup(ctx, c, id, identity.NewGroup{Name: name, Description: g.Description})
	return err
}

// DeleteGroup handles DELETE /Groups/{id}.
func (s *Service) DeleteGroup(c identity.Caller, w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.identity.DeleteGroup(r.Context(), c, id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ServiceProviderConfig handles GET /ServiceProviderConfig.
func (s *Service) ServiceProviderConfig(w http.ResponseWriter, _ *http.Request) {
	off := map[string]any{"supported": false}
	writeJSON(w, http.StatusOK, map[string]any{
		"schemas":        []string{schemaConfig},
		"patch":          map[string]any{"supported": true},
		"bulk":           map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":         map[string]any{"supported": true, "maxResults": 500},
		"changePassword": off, "sort": off, "etag": off,
		"authenticationSchemes": []map[string]any{{"type": "oauthbearertoken", "name": "Bearer token",
			"description": "A token from the Diffluence admin console."}},
		"meta": map[string]any{"resourceType": "ServiceProviderConfig", "location": s.baseURL + "/ServiceProviderConfig"},
	})
}

// ResourceTypes handles GET /ResourceTypes.
func (s *Service) ResourceTypes(w http.ResponseWriter, _ *http.Request) {
	types := []map[string]any{
		{"schemas": []string{schemaResType}, "id": "User", "name": "User", "endpoint": "/Users", "schema": schemaUser,
			"meta": map[string]any{"resourceType": "ResourceType", "location": s.baseURL + "/ResourceTypes/User"}},
		{"schemas": []string{schemaResType}, "id": "Group", "name": "Group", "endpoint": "/Groups", "schema": schemaGroup,
			"meta": map[string]any{"resourceType": "ResourceType", "location": s.baseURL + "/ResourceTypes/Group"}},
	}
	writeJSON(w, http.StatusOK, listResponse(types, len(types), 1, len(types)))
}

func attribute(name, kind string, required bool) map[string]any {
	return map[string]any{"name": name, "type": kind, "multiValued": false, "required": required, "caseExact": false,
		"mutability": "readWrite", "returned": "default", "uniqueness": "none"}
}

// Schemas handles GET /Schemas.
func (s *Service) Schemas(w http.ResponseWriter, _ *http.Request) {
	list := []map[string]any{
		{"schemas": []string{schemaSchema}, "id": schemaUser, "name": "User", "description": "User account",
			"attributes": []any{attribute("userName", "string", true), attribute("displayName", "string", false),
				attribute("active", "boolean", false), attribute("externalId", "string", false)}},
		{"schemas": []string{schemaSchema}, "id": schemaGroup, "name": "Group", "description": "Group",
			"attributes": []any{attribute("displayName", "string", true), attribute("externalId", "string", false)}},
	}
	writeJSON(w, http.StatusOK, listResponse(list, len(list), 1, len(list)))
}
