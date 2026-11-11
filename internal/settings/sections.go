// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
)

// The sections of the settings. Each has its own limits and one audit event.
const (
	SectionGeneral   = "general"
	SectionFeatures  = "features"
	SectionAccess    = "access"
	SectionRetention = "retention"
	SectionLook      = "look"

	sectionKeyPrefix = "section."
	keyMaintenance   = "maintenance"
)

// How a new space starts.
const ()

// Who needs a second factor.
const (
	FactorNone           = "none"
	FactorAdministrators = "administrators"
	FactorMembers        = "members"
	// FactorAfterGrace is the rule of a new instance: administrators, once it has had
	// time to set up a second factor (SEC-164).
	FactorAfterGrace = "administrators_after_grace"
)

const (
	minIdleMinutes    = 5
	maxIdleMinutes    = 7 * 24 * 60
	maxAuditDays      = 36500
	maxShortText      = 200
	maxWelcomeText    = 500
	maxBannerText     = 300
	maxSignupDomains  = 50
	maintenanceReread = 5 * time.Second
	normalTextRatio   = 4.5
	hoverShare        = 0.15
	tintShare         = 0.14
)

// General holds what applies to the whole instance by default.
type General struct {
	DefaultLanguage string `json:"default_language"`
	DefaultTimeZone string `json:"default_time_zone"`
	HelpURL         string `json:"help_url"`
	// EmbedHosts are the outside addresses link cards and embeds may use. Empty by
	// default: nothing outside is reached until an operator says so (ADM-024).
	EmbedHosts []string `json:"embed_hosts"`
}

// Features turns parts of the product on or off. Off hides and deletes nothing.
type Features struct {
	Comments       bool `json:"comments"`
	Reactions      bool `json:"reactions"`
	Blogs          bool `json:"blogs"`
	Invitations    bool `json:"invitations"`
	PersonalSpaces bool `json:"personal_spaces"`
	SelfSignup     bool `json:"self_signup"`
	Analytics      bool `json:"analytics"`
	Automation     bool `json:"automation"`
	Tokens         bool `json:"tokens"`
	Presence       bool `json:"presence"`
}

// Access holds the rules for accounts.
type Access struct {
	SecondFactor  string   `json:"second_factor"`
	IdleMinutes   int      `json:"idle_minutes"`
	SignupDomains []string `json:"signup_domains"`
	GuestInvites  bool     `json:"guest_invites"`
}

// Retention says how long audit rows are kept. Zero means for ever. Longer than the
// default needs a stated reason (SEC-831). Only the command "diffluence audit prune"
// acts on it (spec 013, D11).
type Retention struct {
	AuditDays     int    `json:"audit_days"`
	LongerBecause string `json:"longer_because"`
}

// Look is how the instance presents itself. An accent is a pair: no single colour
// passes on both our light and our dark surfaces (spec 013, D6).
type Look struct {
	InstanceName string `json:"instance_name"`
	// OrganisationName is the company or person this instance is for, given at the first
	// setup. The search box says "Search in" it (spec 033).
	OrganisationName string     `json:"organisation_name"`
	AccentLight      string     `json:"accent_light"`
	AccentDark       string     `json:"accent_dark"`
	LoginWelcome     string     `json:"login_welcome"`
	TermsURL         string     `json:"terms_url"`
	BannerText       string     `json:"banner_text"`
	BannerLevel      string     `json:"banner_level"`
	BannerEnds       *time.Time `json:"banner_ends"`
	WelcomePage      string     `json:"welcome_page"`
}

// Defaults of the sections.
var (
	DefaultGeneral = General{DefaultLanguage: "en", DefaultTimeZone: "UTC", EmbedHosts: []string{}}
	// DefaultFeatures: self sign-up stays off until someone chooses it.
	DefaultFeatures = Features{Comments: true, Reactions: true, Blogs: true, Invitations: true, PersonalSpaces: true,
		Analytics: true, Automation: true, Tokens: true, Presence: true}
	DefaultAccess = Access{SecondFactor: FactorAfterGrace, IdleMinutes: 0, SignupDomains: []string{}, GuestInvites: true}
	// DefaultRetention keeps the audit log for 12 months (ADR-029).
	DefaultRetention = Retention{AuditDays: 365}
)

// SecondFactorGrace is the time a new instance has before its administrators need a
// second factor (SEC-164).
const SecondFactorGrace = 14 * 24 * time.Hour

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func cleanLine(text string, limit int) (string, bool) {
	text = strings.TrimSpace(text)
	return text, utf8.RuneCountInString(text) <= limit && !strings.ContainsAny(text, "\r\n\x00")
}

// safeAddress accepts an http or https address, or nothing.
func safeAddress(address string) bool {
	if address == "" {
		return true
	}
	u, err := url.Parse(address)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && len(address) <= 2048
}

func (g General) validate() error {
	if err := checkDomains(g.EmbedHosts); err != nil {
		return err
	}
	if g.DefaultLanguage != "en" {
		return apperr.Invalid("English is the only language so far.")
	}
	if _, err := time.LoadLocation(g.DefaultTimeZone); err != nil || g.DefaultTimeZone == "" {
		return apperr.Invalid("That is not a time zone.")
	}
	if !safeAddress(g.HelpURL) {
		return apperr.Invalid("The help address must be an http or https address.")
	}
	return nil
}

func (f Features) validate() error { return nil }

func (a Access) validate() error {
	if !slices.Contains([]string{FactorNone, FactorAdministrators, FactorMembers, FactorAfterGrace}, a.SecondFactor) {
		return apperr.Invalid("A second factor is required for nobody, administrators or all members.")
	}
	if a.IdleMinutes != 0 && (a.IdleMinutes < minIdleMinutes || a.IdleMinutes > maxIdleMinutes) {
		return apperr.Invalid("The idle time is off, or between 5 minutes and 7 days.")
	}
	return checkDomains(a.SignupDomains)
}

// checkDomains keeps a list of domains short and free of anything else.
func checkDomains(list []string) error {
	if len(list) > maxSignupDomains {
		return apperr.Invalid("A list has at most 50 domains.")
	}
	for _, d := range list {
		if d == "" || !strings.Contains(d, ".") || strings.ContainsAny(d, " @/\r\n") {
			return apperr.Invalid("A domain looks like example.com.")
		}
	}
	return nil
}

func (r Retention) validate() error {
	if r.AuditDays < 0 || r.AuditDays > maxAuditDays {
		return apperr.Invalid("Audit rows are kept for ever, or up to 36500 days.")
	}
	if (r.AuditDays == 0 || r.AuditDays > DefaultRetention.AuditDays) && strings.TrimSpace(r.LongerBecause) == "" {
		return apperr.Invalid("Keeping audit rows longer than a year needs a reason.")
	}
	return nil
}

func (l Look) validate() error {
	if name, ok := cleanLine(l.InstanceName, maxShortText); !ok || strings.ContainsAny(name, "<>") {
		return apperr.Invalid("The name is one line of at most 200 characters.")
	}
	if name, ok := cleanLine(l.OrganisationName, maxShortText); !ok || strings.ContainsAny(name, "<>") {
		return apperr.Invalid("The name of the organisation is one line of at most 200 characters, without < or >.")
	}
	if _, ok := cleanLine(l.LoginWelcome, maxWelcomeText); !ok {
		return apperr.Invalid("The welcome text is one line of at most 500 characters.")
	}
	if _, ok := cleanLine(l.BannerText, maxBannerText); !ok {
		return apperr.Invalid("The banner is one line of at most 300 characters.")
	}
	if !slices.Contains([]string{"", "info", "warning"}, l.BannerLevel) {
		return apperr.Invalid("A banner is information or a warning.")
	}
	if !safeAddress(l.TermsURL) {
		return apperr.Invalid("The terms address must be an http or https address.")
	}
	if (l.AccentLight == "") != (l.AccentDark == "") {
		return apperr.Invalid("An accent needs a colour for light and a colour for dark.")
	}
	if l.AccentLight == "" {
		return nil
	}
	if problems := CheckAccent(l.AccentLight, l.AccentDark); len(problems) > 0 {
		return apperr.Invalid("These colours do not have enough contrast: " + strings.Join(problems, ", ") + ".")
	}
	return nil
}

type section interface{ validate() error }

// sectionValue gives the zero state of a section: its defaults.
func sectionValue(name string) (section, error) {
	switch name {
	case SectionGeneral:
		return new(General).withDefaults(), nil
	case SectionFeatures:
		f := DefaultFeatures
		return &f, nil
	case SectionAccess:
		a := DefaultAccess
		return &a, nil
	case SectionRetention:
		r := DefaultRetention
		return &r, nil
	case SectionLook:
		return &Look{}, nil
	}
	return nil, apperr.NotFound()
}

func (g *General) withDefaults() *General {
	*g = DefaultGeneral
	return g
}

// ReadSection fills one section. What was never set keeps its default.
func (s *Service) ReadSection(ctx context.Context, name string, into any) error {
	return s.ReadSectionIn(ctx, s.pool, name, into)
}

// ReadSectionIn is ReadSection through q, for code that holds a transaction.
func (s *Service) ReadSectionIn(ctx context.Context, q db.Querier, name string, into any) error {
	var raw []byte
	err := q.QueryRow(ctx, "SELECT value FROM settings WHERE key = $1", sectionKeyPrefix+name).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("settings: read %s: %v", name, err)
	}
	return nil
}

// Section returns one section by name, for the console.
func (s *Service) Section(ctx context.Context, name string) (any, error) {
	v, err := sectionValue(name)
	if err != nil {
		return nil, err
	}
	return v, s.Read(ctx, name, v)
}

// SetSection stores a section from JSON. Unknown fields are refused, as they are
// everywhere else in the API. Only an administrator gets here (route rule).
func (s *Service) SetSection(ctx context.Context, c identity.Caller, name string, body io.Reader) (any, error) {
	v, err := sectionValue(name)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return nil, apperr.Invalid("The settings are not valid.")
	}
	if err := v.validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		stored, err := keepNewerFields(ctx, tx, sectionKeyPrefix+name, raw, v)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, sectionKeyPrefix+name, stored); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("settings.updated", "settings", name, "", map[string]any{"section": name})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "settings.updated", map[string]any{"section": name})
		return err
	})
	return v, err
}

// keepNewerFields adds to raw the fields of the stored section that this release does not
// know. During an upgrade a node of the previous release may save a section a newer node
// already extended; without this it would erase what the newer node stored (OPS-070).
func keepNewerFields(ctx context.Context, tx pgx.Tx, key string, raw []byte, section any) ([]byte, error) {
	var old []byte
	err := tx.QueryRow(ctx, "SELECT value FROM settings WHERE key = $1 FOR UPDATE", key).Scan(&old)
	if errors.Is(err, pgx.ErrNoRows) {
		return raw, nil
	}
	if err != nil {
		return nil, err
	}
	var stored map[string]json.RawMessage
	if json.Unmarshal(old, &stored) != nil {
		return raw, nil // not an object: there are no fields to keep
	}
	var fresh map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fresh); err != nil {
		return nil, err
	}
	known := jsonFields(reflect.TypeOf(section))
	for name, value := range stored {
		if _, ok := fresh[name]; !ok && !known[name] {
			fresh[name] = value
		}
	}
	return json.Marshal(fresh)
}

// jsonFields names the fields a struct reads and writes as JSON, so a field this release
// knows but left out (omitempty) is not mistaken for one of a newer release.
func jsonFields(t reflect.Type) map[string]bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	out := map[string]bool{}
	if t.Kind() != reflect.Struct {
		return out
	}
	for i := range t.NumField() {
		f := t.Field(i)
		tag, hasTag := f.Tag.Lookup("json")
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && !hasTag {
			for inner := range jsonFields(f.Type) {
				out[inner] = true
			}
			continue
		}
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = true
	}
	return out
}

// SetOrganisationName stores the name of the organisation, as the first setup gives it,
// and keeps the rest of the look as it is.
func (s *Service) SetOrganisationName(ctx context.Context, c identity.Caller, name string) error {
	look := Look{}
	if err := s.ReadSection(ctx, SectionLook, &look); err != nil {
		return err
	}
	look.OrganisationName = name
	body, err := json.Marshal(look)
	if err != nil {
		return err
	}
	_, err = s.SetSection(ctx, c, SectionLook, bytes.NewReader(body))
	return err
}

// CheckFirstSetup refuses what the first setup could not store, before it creates
// anything: the organisation name, the default language and the default time zone.
func CheckFirstSetup(organisation, language, zone string) error {
	if err := (Look{OrganisationName: organisation}).validate(); err != nil {
		return err
	}
	g := DefaultGeneral
	g.DefaultLanguage, g.DefaultTimeZone = language, zone
	return g.validate()
}

// ApplyFirstSetup stores the organisation name and the defaults the first setup gave.
func (s *Service) ApplyFirstSetup(ctx context.Context, c identity.Caller, organisation, language, zone string) error {
	if err := s.SetOrganisationName(ctx, c, organisation); err != nil {
		return err
	}
	g := DefaultGeneral
	if err := s.ReadSection(ctx, SectionGeneral, &g); err != nil {
		return err
	}
	g.DefaultLanguage, g.DefaultTimeZone = language, zone
	body, err := json.Marshal(g)
	if err != nil {
		return err
	}
	_, err = s.SetSection(ctx, c, SectionGeneral, bytes.NewReader(body))
	return err
}

// ResetLook returns everything about the look to the defaults.
func (s *Service) ResetLook(ctx context.Context, c identity.Caller) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM settings WHERE key = $1", sectionKeyPrefix+SectionLook); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("settings.updated", "settings", SectionLook, "", map[string]any{"section": SectionLook, "reset": true})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "settings.updated", map[string]any{"section": SectionLook})
		return err
	})
}

// AuditDays reads how long audit rows stay, for the prune command. Zero means for ever.
func AuditDays(ctx context.Context, q db.Querier) (int, error) {
	var stored []byte
	err := q.QueryRow(ctx, "SELECT value FROM settings WHERE key = $1", sectionKeyPrefix+SectionRetention).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultRetention.AuditDays, nil
	}
	if err != nil {
		return 0, err
	}
	var retention Retention
	if err := json.Unmarshal(stored, &retention); err != nil {
		return 0, fmt.Errorf("settings: retention: %w", err)
	}
	return retention.AuditDays, nil
}

// Maintenance is the state of maintenance mode.
type Maintenance struct {
	On      bool   `json:"on"`
	Message string `json:"message"`
}

// maintenanceCache keeps every write from asking the database. A node reads the
// state at most five seconds late (spec 013, F11).
type maintenanceCache struct {
	mu    sync.Mutex
	value Maintenance
	read  time.Time
}

// Maintenance reports the current state.
func (s *Service) Maintenance(ctx context.Context) (Maintenance, error) {
	s.maintenance.mu.Lock()
	defer s.maintenance.mu.Unlock()
	if time.Since(s.maintenance.read) < maintenanceReread {
		return s.maintenance.value, nil
	}
	var v Maintenance
	var raw []byte
	err := s.pool.QueryRow(ctx, "SELECT value FROM settings WHERE key = $1", keyMaintenance).Scan(&raw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return v, err
	}
	if err == nil {
		if err := json.Unmarshal(raw, &v); err != nil {
			return v, fmt.Errorf("settings: read maintenance: %v", err)
		}
	}
	s.maintenance.value, s.maintenance.read = v, time.Now()
	return v, nil
}

// SetMaintenance turns maintenance mode on or off.
func (s *Service) SetMaintenance(ctx context.Context, c identity.Caller, v Maintenance) error {
	message, ok := cleanLine(v.Message, maxBannerText)
	if !ok {
		return apperr.Invalid("The message is one line of at most 300 characters.")
	}
	v.Message = message
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, keyMaintenance, raw); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("maintenance.changed", "settings", keyMaintenance, "", map[string]any{"on": v.On})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "maintenance.changed", map[string]any{"on": v.On})
		return err
	})
	if err != nil {
		return err
	}
	s.maintenance.mu.Lock()
	defer s.maintenance.mu.Unlock()
	s.maintenance.value, s.maintenance.read = v, time.Now()
	return nil
}

type rgb struct{ r, g, b float64 }

func parseColour(hex string) (rgb, bool) {
	if !hexColour.MatchString(hex) {
		return rgb{}, false
	}
	part := func(at int) float64 {
		n, _ := strconv.ParseUint(hex[at:at+2], 16, 8)
		return float64(n)
	}
	return rgb{part(1), part(3), part(5)}, true
}

func (c rgb) hex() string {
	round := func(v float64) int { return int(math.Round(v)) }
	return fmt.Sprintf("#%02x%02x%02x", round(c.r), round(c.g), round(c.b))
}

func (c rgb) mix(other rgb, share float64) rgb {
	blend := func(a, b float64) float64 { return a*(1-share) + b*share }
	return rgb{blend(c.r, other.r), blend(c.g, other.g), blend(c.b, other.b)}
}

func (c rgb) luminance() float64 {
	channel := func(v float64) float64 {
		v /= 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.r) + 0.7152*channel(c.g) + 0.0722*channel(c.b)
}

func contrast(a, b rgb) float64 {
	lighter, darker := a.luminance(), b.luminance()
	if lighter < darker {
		lighter, darker = darker, lighter
	}
	return (lighter + 0.05) / (darker + 0.05)
}

// The surfaces of both themes, from tokens.css. A test compares them with that file.
var (
	lightSurface  = rgb{255, 255, 255}
	lightSide     = rgb{245, 247, 250}
	lightOnAccent = rgb{255, 255, 255}
	darkSurface   = rgb{17, 22, 31}
	darkSide      = rgb{23, 29, 39}
	darkOnAccent  = rgb{11, 20, 32}
	black         = rgb{0, 0, 0}
	white         = rgb{255, 255, 255}
)

// AccentColours are the three values of one theme, derived from one colour.
type AccentColours struct {
	Accent string `json:"accent"`
	Hover  string `json:"hover"`
	Tint   string `json:"tint"`
}

func deriveAccent(accent, surface, towards rgb) (hover, tint rgb) {
	return accent.mix(towards, hoverShare), surface.mix(accent, tintShare)
}

// DerivedAccent gives both themes, for the appearance answer. It is empty without
// an accent.
func DerivedAccent(light, dark string) map[string]AccentColours {
	l, okLight := parseColour(light)
	d, okDark := parseColour(dark)
	if !okLight || !okDark {
		return nil
	}
	lightHover, lightTint := deriveAccent(l, lightSurface, black)
	darkHover, darkTint := deriveAccent(d, darkSurface, white)
	return map[string]AccentColours{
		"light": {Accent: l.hex(), Hover: lightHover.hex(), Tint: lightTint.hex()},
		"dark":  {Accent: d.hex(), Hover: darkHover.hex(), Tint: darkTint.hex()},
	}
}

// CheckAccent checks the pairs of check-design.mjs that hold an accent. It returns
// the names of what fails.
func CheckAccent(light, dark string) []string {
	l, okLight := parseColour(light)
	d, okDark := parseColour(dark)
	if !okLight || !okDark {
		return []string{"not a colour such as #1d5fbf"}
	}
	var problems []string
	need := func(name string, a, b rgb) {
		if contrast(a, b) < normalTextRatio {
			problems = append(problems, name)
		}
	}
	lightHover, lightTint := deriveAccent(l, lightSurface, black)
	need("light accent on the page", l, lightSurface)
	need("light accent on the sidebar", l, lightSide)
	need("light accent on its tint", l, lightTint)
	need("text on the light accent", lightOnAccent, l)
	need("text on the light hover", lightOnAccent, lightHover)
	darkHover, darkTint := deriveAccent(d, darkSurface, white)
	need("dark accent on the page", d, darkSurface)
	need("dark accent on the sidebar", d, darkSide)
	need("dark accent on its tint", d, darkTint)
	need("text on the dark accent", darkOnAccent, d)
	need("text on the dark hover", darkOnAccent, darkHover)
	return problems
}
