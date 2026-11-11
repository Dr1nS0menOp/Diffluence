// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/permissions"
)

// fakeMail catches account mail, so a test can follow the link in it.
type fakeMail struct {
	mu   sync.Mutex
	sent []identity.AccountMail
}

func (f *fakeMail) Send(_ context.Context, _ db.Querier, m identity.AccountMail) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeMail) lastToken(t *testing.T) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		t.Fatal("no mail was sent")
	}
	u, err := url.Parse(f.sent[len(f.sent)-1].Link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("token")
}

func (f *fakeMail) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

func withMail(e *env) *fakeMail {
	m := &fakeMail{}
	e.app.Identity().SetMailer(m, "http://example.test")
	return m
}

func (e *env) sql(query string, args ...any) {
	e.t.Helper()
	if _, err := e.app.Pool().Exec(context.Background(), query, args...); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
}

func admin(e *env) *user {
	return e.user("admin@example.com", identity.AllGlobalPermissions...)
}

func TestManagerRunsPeopleButNeverReachesAdminister(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	manager := e.user("manager@example.com", identity.PermUseProduct)
	viewer := e.user("viewer@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", manager.id, permissions.TemplateManager)

	set := func(u *user, who, template string) (int, string) {
		return u.do("PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{
			"subject": map[string]any{"type": "user", "id": who}, "permissions": permissions.Templates[template]})
	}
	if status, out := set(manager, viewer.id, permissions.TemplateViewer); status != 204 {
		t.Fatalf("a manager gives access: %d %s", status, out)
	}
	if status, out := set(manager, viewer.id, permissions.TemplateSpaceAdmin); status != 409 || !strings.Contains(out, "needs_administer") {
		t.Fatalf("a manager cannot make an administrator: %d %s", status, out)
	}
	if status, out := set(manager, manager.id, permissions.TemplateSpaceAdmin); status != 409 {
		t.Fatalf("a manager cannot promote themselves: %d %s", status, out)
	}
	if status, out := set(manager, owner.id, permissions.TemplateViewer); status != 409 {
		t.Fatalf("a manager cannot change an administrator: %d %s", status, out)
	}
	manager.want(403, "PATCH", "/api/v1/spaces/"+space, map[string]any{"name": "x"})
	manager.want(403, "POST", "/api/v1/spaces/"+space+"/archive", nil)
	manager.want(200, "GET", "/api/v1/spaces/"+space+"/grants", nil)
}

func TestAnExpiredAccountHasNoAccessFromThatMoment(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	guest := e.userOfType("guest@example.com", identity.TypeGuest, identity.PermUseProduct)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Page", "x")
	owner.grant(space, "user", guest.id, permissions.TemplateViewer)
	guest.want(200, "GET", "/api/v1/pages/"+page, nil)

	e.sql("UPDATE users SET expires_at = now() - interval '1 minute' WHERE id = $1", guest.id)
	guest.want(401, "GET", "/api/v1/pages/"+page, nil)
	if got := e.login(e.client(), "guest@example.com", adminPassword); got != 401 {
		t.Fatalf("an expired account logged in: %d", got)
	}
	var listed int
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM perm_visible_pages($1)", guest.id).Scan(&listed); err != nil {
		t.Fatal(err)
	}
	if listed != 0 {
		t.Fatalf("an expired account still sees %d pages in lists", listed)
	}
	if err := e.app.Identity().EndExpiredAccounts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.auditActions("user.expired") != 1 {
		t.Fatal("the end of an account must be audited")
	}
}

func (e *env) bearer(method, path, token, body string) (int, string) {
	e.t.Helper()
	resp, out := e.do(e.client(), method, path, body, "Authorization", "Bearer "+token, "Content-Type", "application/json")
	return resp.StatusCode, out
}

func TestTokensStayInsideTheirScope(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermAdministrator)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Page", "x")
	ends := time.Now().Add(24 * time.Hour).Format(time.RFC3339)

	made := owner.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "script", "expires_at": ends})
	read := made["secret"].(string)
	if !strings.HasPrefix(read, "dfl_") {
		t.Fatalf("a scanner cannot find this token: %s", read)
	}
	if status, out := e.bearer("GET", "/api/v1/pages/"+page, read, ""); status != 200 {
		t.Fatalf("a token reads: %d %s", status, out)
	}
	if status, out := e.bearer("PATCH", "/api/v1/pages/"+page, read, `{"title":"x"}`); status != 403 || !strings.Contains(out, "token_read_only") {
		t.Fatalf("a read token cannot write: %d %s", status, out)
	}
	written := owner.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "writer", "can_write": true, "expires_at": ends})["secret"].(string)
	if status, out := e.bearer("PATCH", "/api/v1/pages/"+page, written, `{"title":"New"}`); status != 200 {
		t.Fatalf("a write token writes: %d %s", status, out)
	}
	for _, path := range []string{"/api/v1/me/tokens", "/api/v1/me/logins", "/api/v1/me/sessions", "/api/v1/admin/users", "/api/v1/admin/summary"} {
		if status, out := e.bearer("GET", path, written, ""); status != 403 || !strings.Contains(out, "session_only") {
			t.Errorf("%s with a token: %d %s", path, status, out)
		}
	}

	id := made["token"].(map[string]any)["id"].(string)
	owner.ok("DELETE", "/api/v1/me/tokens/"+id, nil)
	if status, _ := e.bearer("GET", "/api/v1/pages/"+page, read, ""); status != 401 {
		t.Fatalf("a revoked token works: %d", status)
	}

	owner.want(400, "POST", "/api/v1/me/tokens", map[string]any{"name": "long", "expires_at": time.Now().Add(400 * 24 * time.Hour).Format(time.RFC3339)})
	settingsAdmin := admin(e)
	features := settingsAdmin.ok("GET", "/api/v1/admin/settings/features", nil)
	features["tokens"] = false
	settingsAdmin.ok("PUT", "/api/v1/admin/settings/features", features)
	if status, _ := e.bearer("GET", "/api/v1/pages/"+page, written, ""); status != 401 {
		t.Fatalf("turning tokens off must stop them: %d", status)
	}
}

// Walks the real route table: a new sensitive route is covered the moment it exists.
func TestEverySensitiveRouteAsksForProof(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	checked := 0
	e.sql("UPDATE sessions SET created_at = now() - interval '11 minutes'")
	for _, r := range e.app.Routes() {
		if !strings.HasPrefix(r.Access.String(), "sensitive ") {
			continue
		}
		checked++
		method, path, _ := strings.Cut(r.Pattern, " ")
		path = strings.ReplaceAll(path, "{id}", root.id)
		status, out := root.do(method, path, "{}")
		if status != 403 || !strings.Contains(out, "reauth_required") {
			t.Errorf("%s: %d %s", r.Pattern, status, out)
		}
	}
	if checked < 7 {
		t.Fatalf("only %d sensitive routes", checked)
	}
	root.want(409, "POST", "/api/v1/auth/reauth", map[string]any{"password": "wrong password here"})
	root.ok("POST", "/api/v1/auth/reauth", map[string]any{"password": adminPassword})
	root.ok("PUT", "/api/v1/admin/maintenance", map[string]any{"on": false, "message": ""})
	if e.auditActions("auth.reauthenticated") != 1 {
		t.Fatal("proving who you are must be audited")
	}
}

func TestPollingDoesNotKeepASessionAlive(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	seen := func() time.Time {
		var at time.Time
		if err := e.app.Pool().QueryRow(context.Background(), "SELECT last_seen_at FROM sessions WHERE user_id = $1", anna.id).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	e.sql("UPDATE sessions SET last_seen_at = now() - interval '5 minutes' WHERE user_id = $1", anna.id)
	before := seen()
	resp, _ := e.do(anna.c, "GET", "/api/v1/me", "", "X-Background", "1")
	if resp.StatusCode != 200 {
		t.Fatalf("a background request must work: %d", resp.StatusCode)
	}
	if !seen().Equal(before) {
		t.Fatal("a background request counted as activity")
	}
	anna.want(200, "GET", "/api/v1/me", nil)
	if seen().Equal(before) {
		t.Fatal("a normal request must count as activity")
	}
}

func TestIdleTimeoutFromTheSettingsEndsASession(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	access := root.ok("GET", "/api/v1/admin/settings/access", nil)
	access["idle_minutes"] = 10
	root.ok("PUT", "/api/v1/admin/settings/access", access)
	e.sql("UPDATE sessions SET last_seen_at = now() - interval '11 minutes' WHERE user_id = $1", anna.id)
	anna.want(401, "GET", "/api/v1/me", nil)
}

func TestMaintenanceStopsEveryWriteAndCanBeTurnedOff(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	space, _ := root.space("ENG")
	page := root.page(space, nil, "Page", "x")
	notSystem := e.user("plain@example.com", identity.PermUseProduct, identity.PermAdministrator)
	notSystem.want(403, "PUT", "/api/v1/admin/maintenance", map[string]any{"on": true})

	root.ok("PUT", "/api/v1/admin/maintenance", map[string]any{"on": true, "message": "Back at noon"})
	status, out := root.do("PATCH", "/api/v1/pages/"+page, map[string]any{"title": "x"})
	if status != 503 || !strings.Contains(out, "maintenance") {
		t.Fatalf("a write during maintenance: %d %s", status, out)
	}
	root.want(200, "GET", "/api/v1/pages/"+page, nil)
	if got := e.login(e.client(), "admin@example.com", adminPassword); got != 200 {
		t.Fatalf("login must keep working: %d", got)
	}
	resp, body := e.do(e.client(), "POST", "/scim/v2/Users", "{}", "Content-Type", "application/json")
	if resp.StatusCode != 503 {
		t.Fatalf("SCIM must wait too: %d %s", resp.StatusCode, body)
	}
	look := root.ok("GET", "/api/v1/appearance", nil)
	if m := look["maintenance"].(map[string]any); m["on"] != true || m["message"] != "Back at noon" {
		t.Fatalf("the message must reach the screen: %v", m)
	}
	root.ok("PUT", "/api/v1/admin/maintenance", map[string]any{"on": false, "message": ""})
	root.ok("PATCH", "/api/v1/pages/"+page, map[string]any{"title": "Again"})
	if e.auditActions("maintenance.changed") != 2 {
		t.Fatal("both switches must be audited")
	}
}

func TestSettingsAreCheckedPerSection(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	plain := e.user("plain@example.com", identity.PermUseProduct)
	plain.want(403, "GET", "/api/v1/admin/settings/general", nil)
	root.want(404, "GET", "/api/v1/admin/settings/secret", nil)

	general := root.ok("GET", "/api/v1/admin/settings/general", nil)
	if general["default_time_zone"] != "UTC" {
		t.Fatalf("defaults: %v", general)
	}
	before := e.auditActions("settings.updated")
	for name, change := range map[string]map[string]any{
		"zone": {"default_time_zone": "Mars/Base"},
		"help": {"help_url": "javascript:alert(1)"},
	} {
		body := map[string]any{}
		for k, v := range general {
			body[k] = v
		}
		for k, v := range change {
			body[k] = v
		}
		if status, out := root.do("PUT", "/api/v1/admin/settings/general", body); status != 400 {
			t.Errorf("%s: %d %s", name, status, out)
		}
	}
	root.want(400, "PUT", "/api/v1/admin/settings/general", map[string]any{"unknown": true})
	if e.auditActions("settings.updated") != before {
		t.Fatal("a refusal must leave no event")
	}
	general["help_url"] = "https://help.example.com"
	root.ok("PUT", "/api/v1/admin/settings/general", general)
	if e.auditActions("settings.updated") != before+1 {
		t.Fatal("a change must be audited")
	}
}

func TestTheOrganisationNameIsPlainTextOfOneLine(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	if got := root.ok("GET", "/api/v1/appearance", nil)["organisation"]; got != "" {
		t.Fatalf("without a name the search box says only Search: %v", got)
	}
	root.ok("PUT", "/api/v1/admin/settings/look", map[string]any{"organisation_name": "Acme & Sons"})
	if got := root.ok("GET", "/api/v1/appearance", nil)["organisation"]; got != "Acme & Sons" {
		t.Fatalf("organisation %v", got)
	}
	root.want(400, "PUT", "/api/v1/admin/settings/look", map[string]any{"organisation_name": "<b>Acme</b>"})
	root.want(400, "PUT", "/api/v1/admin/settings/look", map[string]any{"organisation_name": "Two\nlines"})
	root.want(400, "PUT", "/api/v1/admin/settings/look", map[string]any{"organisation_name": strings.Repeat("a", 201)})
}

func TestAnAccentMustKeepContrastInBothThemes(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	look := map[string]any{"instance_name": "Team wiki", "accent_light": "#ffe600", "accent_dark": "#7fb0f5"}
	out := root.want(400, "PUT", "/api/v1/admin/settings/look", look)
	if !strings.Contains(out, "light accent on the page") {
		t.Fatalf("the failing pair must be named: %s", out)
	}
	root.want(400, "PUT", "/api/v1/admin/settings/look", map[string]any{"accent_light": "#1d5fbf", "accent_dark": "#1d5fbf"})
	root.want(400, "PUT", "/api/v1/admin/settings/look", map[string]any{"accent_light": "#1d5fbf"})
	root.ok("PUT", "/api/v1/admin/settings/look", map[string]any{"instance_name": "Team wiki", "accent_light": "#1d5fbf", "accent_dark": "#7fb0f5",
		"login_welcome": "Hello", "banner_text": "Note this", "banner_level": "info"})

	resp, body := e.do(e.client(), "GET", "/api/v1/appearance", "")
	if resp.StatusCode != 200 {
		t.Fatalf("the login page reads this without a session: %d", resp.StatusCode)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	accent := got["accent"].(map[string]any)["light"].(map[string]any)
	if got["name"] != "Team wiki" || accent["accent"] != "#1d5fbf" || accent["tint"] == "" {
		t.Fatalf("appearance: %v", got)
	}
	root.ok("POST", "/api/v1/admin/appearance/reset", nil)
	if root.ok("GET", "/api/v1/appearance", nil)["name"] != "" {
		t.Fatal("reset must bring the defaults back")
	}
}

func (u *user) putRaw(path string, body []byte) (int, string) {
	u.e.t.Helper()
	resp, out := u.e.do(u.c, "PUT", path, string(body), "Content-Type", "application/octet-stream")
	return resp.StatusCode, out
}

func pngOf(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := range width {
		img.Set(x, 0, color.RGBA{R: 200, A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// withTextChunk adds a valid tEXt chunk behind the header of a PNG.
func withTextChunk(data []byte, text string) []byte {
	body := append([]byte("tEXt"), text...)
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(text)))
	chunk = append(chunk, body...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(body))
	const headerEnd = 33
	return append(append(append([]byte{}, data[:headerEnd]...), chunk...), data[headerEnd:]...)
}

func TestALogoIsAnImageAndNeverSVG(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	if status, out := root.putRaw("/api/v1/admin/appearance/logo", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>x</script></svg>`)); status != 400 {
		t.Fatalf("SVG as a logo: %d %s", status, out)
	}
	if status, out := root.putRaw("/api/v1/admin/appearance/logo", pngOf(16, 16)); status != 204 {
		t.Fatalf("a PNG logo: %d %s", status, out)
	}
	resp, _ := e.do(e.client(), "GET", "/api/v1/appearance/logo", "")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("logo: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	root.ok("DELETE", "/api/v1/admin/appearance/logo", nil)
	resp, _ = e.do(e.client(), "GET", "/api/v1/appearance/logo", "")
	if resp.StatusCode != 404 {
		t.Fatalf("a removed logo: %d", resp.StatusCode)
	}
}

func namesIn(m map[string]any) []string {
	list, _ := m["people"].([]any)
	out := []string{}
	for _, item := range list {
		out = append(out, item.(map[string]any)["name"].(string))
	}
	return out
}

func TestAGuestOnlyFindsWhoTheyShareASpaceWith(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermViewProfiles)
	colleague := e.user("colleague@example.com", identity.PermUseProduct)
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	guest := e.userOfType("guest@example.com", identity.TypeGuest, identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", guest.id, permissions.TemplateViewer)
	owner.grant(space, "user", colleague.id, permissions.TemplateViewer)
	open, _ := owner.space("OPEN")
	owner.grant(open, "all_members", "", permissions.TemplateViewer)

	names := namesIn(guest.ok("GET", "/api/v1/people", nil))
	if !has(names, "Test colleague@example.com") || has(names, "Test stranger@example.com") {
		t.Fatalf("a guest finds: %v", names)
	}
	guest.want(200, "GET", "/api/v1/people/"+colleague.id, nil)
	guest.want(404, "GET", "/api/v1/people/"+stranger.id, nil)
	guest.want(404, "GET", "/api/v1/people/"+stranger.id+"/picture", nil)
	if !has(namesIn(owner.ok("GET", "/api/v1/people", nil)), "Test stranger@example.com") {
		t.Fatal("view profiles shows everyone")
	}
}

func TestAPersonChoosesWhatOthersSee(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermViewProfiles)
	member := e.user("member@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	guest := e.userOfType("guest@example.com", identity.TypeGuest, identity.PermUseProduct)
	space, _ := anna.space("ENG")
	anna.grant(space, "user", guest.id, permissions.TemplateViewer)

	anna.ok("PUT", "/api/v1/me/profile", map[string]any{"job_title": "Writer", "department": "Docs", "location": "Delft", "about": "Hello",
		"visibility": map[string]any{"location": "me", "about": "members"}})
	got := member.ok("GET", "/api/v1/people/"+anna.id, nil)
	if got["job_title"] != "Writer" || got["about"] != "Hello" || got["location"] != "" || got["email"] != "anna@example.com" {
		t.Fatalf("a member sees: %v", got)
	}
	got = guest.ok("GET", "/api/v1/people/"+anna.id, nil)
	if got["about"] != "" || got["email"] != "" || got["job_title"] != "Writer" {
		t.Fatalf("a guest sees: %v", got)
	}
	if anna.ok("GET", "/api/v1/people/"+anna.id, nil)["location"] != "Delft" {
		t.Fatal("a person sees everything of themselves")
	}
	anna.want(400, "PUT", "/api/v1/me/profile", map[string]any{"visibility": map[string]any{"password": "everyone"}})
	anna.want(400, "PUT", "/api/v1/me/profile", map[string]any{"visibility": map[string]any{"about": "world"}})
	anna.want(400, "PUT", "/api/v1/me/profile", map[string]any{"job_title": strings.Repeat("x", 201)})
}

func TestFieldsFromTheIdentityProviderStayLocked(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	e.sql(`INSERT INTO user_profiles (user_id, job_title, locked_fields) VALUES ($1, 'From SCIM', '{job_title}')`, anna.id)
	anna.ok("PUT", "/api/v1/me/profile", map[string]any{"job_title": "Mine", "department": "Mine"})
	got := anna.ok("GET", "/api/v1/people/"+anna.id, nil)
	if got["job_title"] != "From SCIM" || got["department"] != "Mine" {
		t.Fatalf("locked fields: %v", got)
	}
}

func TestAPictureIsDrawnAgainWithoutWhatWasInTheFile(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	raw := pngOf(300, 200)
	// A text chunk after the header, as a camera or an editor would leave.
	withMeta := withTextChunk(raw, "GPS\x0052.0N 4.3")

	status, out := anna.putRaw("/api/v1/me/picture?x=10&y=20&size=100", withMeta)
	if status != 204 {
		t.Fatalf("a picture: %d %s", status, out)
	}
	resp, body := e.do(anna.c, "GET", "/api/v1/people/"+anna.id+"/picture", "")
	if resp.StatusCode != 200 || strings.Contains(body, "52.0N") {
		t.Fatalf("picture: %d, metadata survived: %v", resp.StatusCode, strings.Contains(body, "52.0N"))
	}
	img, err := png.Decode(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
		t.Fatalf("size: %v", img.Bounds())
	}
	if status, _ := anna.putRaw("/api/v1/me/picture?x=250&y=0&size=100", raw); status != 400 {
		t.Fatalf("a crop outside the picture: %d", status)
	}
	if status, _ := anna.putRaw("/api/v1/me/picture", []byte("<svg></svg>")); status != 400 {
		t.Fatalf("something else: %d", status)
	}
	header := pngOf(1, 1)
	// A header that claims 100,000 by 100,000 pixels must be refused before decoding.
	copy(header[16:24], []byte{0x00, 0x01, 0x86, 0xA0, 0x00, 0x01, 0x86, 0xA0})
	if status, _ := anna.putRaw("/api/v1/me/picture", header); status != 413 && status != 400 {
		t.Fatalf("a giant picture: %d", status)
	}
	anna.ok("DELETE", "/api/v1/me/picture", nil)
	anna.want(404, "GET", "/api/v1/people/"+anna.id+"/picture", nil)
}

func publicPost(e *env, path string, body map[string]any) (int, string) {
	e.t.Helper()
	b, _ := json.Marshal(body)
	resp, out := e.do(e.client(), "POST", path, string(b), "Content-Type", "application/json")
	return resp.StatusCode, out
}

func TestAResetLinkWorksOnceAndRevealsNothing(t *testing.T) {
	e := newEnv(t)
	mail := withMail(e)
	anna := e.user("anna@example.com", identity.PermUseProduct)

	status, unknown := publicPost(e, "/api/v1/auth/forgot", map[string]any{"email": "nobody@example.com"})
	if status != 202 || mail.count() != 0 {
		t.Fatalf("unknown address: %d %q, mails %d", status, unknown, mail.count())
	}
	status, known := publicPost(e, "/api/v1/auth/forgot", map[string]any{"email": "ANNA@example.com"})
	if status != 202 || known != unknown {
		t.Fatalf("the answer differs: %d %q %q", status, known, unknown)
	}
	token := mail.lastToken(t)

	if status, out := publicPost(e, "/api/v1/auth/reset", map[string]any{"token": token, "password": "short"}); status != 400 {
		t.Fatalf("a weak password: %d %s", status, out)
	}
	// A refused password rolled back: the link still works.
	next := "a brand new long passphrase"
	if status, out := publicPost(e, "/api/v1/auth/reset", map[string]any{"token": token, "password": next}); status != 204 {
		t.Fatalf("reset: %d %s", status, out)
	}
	anna.want(401, "GET", "/api/v1/me", nil)
	if got := e.login(e.client(), "anna@example.com", next); got != 200 {
		t.Fatalf("the new password: %d", got)
	}
	if status, out := publicPost(e, "/api/v1/auth/reset", map[string]any{"token": token, "password": next + "x"}); status != 409 || !strings.Contains(out, "link_invalid") {
		t.Fatalf("a used link: %d %s", status, out)
	}
	if e.auditActions("auth.password_reset") != 1 {
		t.Fatal("a reset must be audited")
	}
}

func TestAResetStillAsksForTheSecondFactor(t *testing.T) {
	e := newEnv(t)
	mail := withMail(e)
	e.user("anna@example.com", identity.PermUseProduct)
	e.sql(`INSERT INTO totp_credentials (user_id, secret_sealed, confirmed_at)
		SELECT id, '\x00', now() FROM users WHERE email = 'anna@example.com'`)
	publicPost(e, "/api/v1/auth/forgot", map[string]any{"email": "anna@example.com"})
	next := "a brand new long passphrase"
	publicPost(e, "/api/v1/auth/reset", map[string]any{"token": mail.lastToken(t), "password": next})
	body, _ := json.Marshal(map[string]any{"email": "anna@example.com", "password": next})
	_, out := e.do(e.client(), "POST", "/api/v1/auth/login", string(body), "Content-Type", "application/json")
	if !strings.Contains(out, "mfa_required") {
		t.Fatalf("a reset must not skip the second factor: %s", out)
	}
}

func TestPasswordRulesRefuseNamesAndKnownPasswords(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna.visser@example.com", identity.PermUseProduct)
	out := anna.want(400, "POST", "/api/v1/me/password", map[string]any{"current_password": adminPassword, "new_password": "my anna.visser password"})
	if !strings.Contains(out, "name") {
		t.Fatalf("a password with the address: %s", out)
	}
	anna.ok("POST", "/api/v1/me/password", map[string]any{"current_password": adminPassword, "new_password": "nothing about me in here"})
}

func TestANewPasswordMustBeLongAndNotLeaked(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	change := func(next string) (int, string) {
		return anna.do("POST", "/api/v1/me/password", map[string]any{"current_password": adminPassword, "new_password": next})
	}
	if status, out := change("fourteen chars"); status != 400 {
		t.Fatalf("too short: %d %s", status, out)
	}
	if status, out := change("1qaz2wsx3edc4rfv"); status != 400 || !strings.Contains(out, "leaked") {
		t.Fatalf("a leaked password: %d %s", status, out)
	}
	if status, out := change("1QAZ2WSX3EDC4RFV"); status != 400 {
		t.Fatalf("case must not help: %d %s", status, out)
	}
	if status, out := change("fifteen letters"); status != 204 {
		t.Fatalf("15 characters: %d %s", status, out)
	}
	// An older, shorter password keeps working until someone changes it.
	short := "twelve chars"
	hash, err := identity.HashPassword(short, identity.DefaultParams)
	if err != nil {
		t.Fatal(err)
	}
	e.sql("UPDATE local_credentials SET password_hash = $2 WHERE user_id = $1", anna.id, hash)
	if got := e.login(e.client(), "anna@example.com", short); got != 200 {
		t.Fatalf("an existing password must stay valid: %d", got)
	}
}

func TestPeopleAreAskedForAStrongerSignInUntilTheyHaveOne(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	signIn := func() map[string]any { return anna.ok("GET", "/api/v1/me", nil)["sign_in"].(map[string]any) }
	if got := signIn(); got["weak"] != true || got["prompt"] != true {
		t.Fatalf("a password alone: %v", got)
	}
	anna.ok("POST", "/api/v1/me/sign-in-prompt/later", nil)
	if got := signIn(); got["weak"] != true || got["prompt"] != false {
		t.Fatalf("put off: %v", got)
	}
	var after time.Time
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT sign_in_prompt_after FROM users WHERE id = $1", anna.id).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if time.Until(after) > 8*24*time.Hour {
		t.Fatalf("put off for too long: %v", after)
	}
	// It comes back by itself: it can be put off, and not switched off.
	e.sql("UPDATE users SET sign_in_prompt_after = now() - interval '1 minute' WHERE id = $1", anna.id)
	if signIn()["prompt"] != true {
		t.Fatal("the question must come back")
	}
	e.sql(`INSERT INTO totp_credentials (user_id, secret_sealed, confirmed_at) VALUES ($1, '\x00', now())`, anna.id)
	if got := signIn(); got["weak"] != false || got["prompt"] != false {
		t.Fatalf("with a second factor: %v", got)
	}
}

func TestAdministratorsAreWarnedAboutWeakSignIn(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	out := root.want(200, "GET", "/api/v1/admin/attention", nil)
	if !strings.Contains(out, "no_identity_provider") || !strings.Contains(out, "no_passkey") {
		t.Fatalf("attention: %s", out)
	}
	root.ok("POST", "/api/v1/me/sign-in-prompt/later", nil)
	var after time.Time
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT sign_in_prompt_after FROM users WHERE id = $1", root.id).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if time.Until(after) > 25*time.Hour {
		t.Fatalf("an administrator is asked again sooner: %v", after)
	}
}

func TestAnInvitationMakesAnAccountOnce(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	link := root.ok("POST", "/api/v1/admin/invitations", map[string]any{"email": "new@example.com", "type": "member"})["link"].(string)
	u, err := url.Parse(link)
	if err != nil || u.Path != "/invitation" {
		t.Fatalf("link: %q", link)
	}
	token := u.Query().Get("token")

	list := root.ok("GET", "/api/v1/admin/invitations", nil)["invitations"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["email"] != "new@example.com" {
		t.Fatalf("list: %v", list)
	}
	root.want(409, "POST", "/api/v1/admin/invitations", map[string]any{"email": "admin@example.com", "type": "member"})

	resp, out := e.do(e.client(), "GET", "/api/v1/auth/invitation?token="+url.QueryEscape(token), "")
	if resp.StatusCode != 200 || !strings.Contains(out, "new@example.com") {
		t.Fatalf("invitation: %d %s", resp.StatusCode, out)
	}
	accept := map[string]any{"token": token, "display_name": "New Person", "password": "a long enough passphrase"}
	if status, out := publicPost(e, "/api/v1/auth/invitation", accept); status != 204 {
		t.Fatalf("accept: %d %s", status, out)
	}
	if got := e.login(e.client(), "new@example.com", "a long enough passphrase"); got != 200 {
		t.Fatalf("login: %d", got)
	}
	if status, _ := publicPost(e, "/api/v1/auth/invitation", accept); status != 409 {
		t.Fatalf("a used link: %d", status)
	}
}

func TestAGuestInvitationCarriesOneSpace(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermInvitePeople)
	viewer := e.user("viewer@example.com", identity.PermUseProduct, identity.PermInvitePeople)
	space, home := owner.space("ENG")
	owner.grant(space, "user", viewer.id, permissions.TemplateViewer)
	viewer.want(403, "POST", "/api/v1/spaces/"+space+"/guests", map[string]any{"email": "guest@example.com"})

	request := owner.ok("POST", "/api/v1/spaces/"+space+"/guests", map[string]any{"email": "guest@example.com"})["id"].(string)
	link := root.ok("POST", "/api/v1/invitation-requests/"+request+"/approve", map[string]any{"reason": "Known to us"})["link"].(string)
	u, _ := url.Parse(link)
	if status, out := publicPost(e, "/api/v1/auth/invitation", map[string]any{"token": u.Query().Get("token"), "display_name": "Guest", "password": "a long enough passphrase"}); status != 204 {
		t.Fatalf("accept: %d %s", status, out)
	}
	c := e.client()
	if got := e.login(c, "guest@example.com", "a long enough passphrase"); got != 200 {
		t.Fatalf("login: %d", got)
	}
	resp, _ := e.do(c, "GET", "/api/v1/pages/"+home, "")
	if resp.StatusCode != 200 {
		t.Fatalf("a guest reaches the space they were invited to: %d", resp.StatusCode)
	}
}

func TestSignUpIsOffByDefaultAndOnlyForListedDomains(t *testing.T) {
	e := newEnv(t)
	mail := withMail(e)
	root := admin(e)
	if status, _ := publicPost(e, "/api/v1/auth/signup", map[string]any{"email": "a@example.com"}); status != 404 {
		t.Fatalf("sign-up must be off: %d", status)
	}
	features := root.ok("GET", "/api/v1/admin/settings/features", nil)
	features["self_signup"] = true
	root.ok("PUT", "/api/v1/admin/settings/features", features)
	access := root.ok("GET", "/api/v1/admin/settings/access", nil)
	access["signup_domains"] = []string{"example.com"}
	root.ok("PUT", "/api/v1/admin/settings/access", access)

	if status, _ := publicPost(e, "/api/v1/auth/signup", map[string]any{"email": "a@other.example.org"}); status != 202 || mail.count() != 0 {
		t.Fatalf("another domain: %d, mails %d", status, mail.count())
	}
	if status, _ := publicPost(e, "/api/v1/auth/signup", map[string]any{"email": "admin@example.com"}); status != 202 || mail.count() != 0 {
		t.Fatalf("a known address: %d, mails %d", status, mail.count())
	}
	if status, _ := publicPost(e, "/api/v1/auth/signup", map[string]any{"email": "new@example.com"}); status != 202 || mail.count() != 1 {
		t.Fatalf("a listed domain: %d, mails %d", status, mail.count())
	}
	if status, out := publicPost(e, "/api/v1/auth/signup/confirm", map[string]any{"token": mail.lastToken(t), "display_name": "New", "password": "a long enough passphrase"}); status != 204 {
		t.Fatalf("confirm: %d %s", status, out)
	}
	if got := e.login(e.client(), "new@example.com", "a long enough passphrase"); got != 200 {
		t.Fatalf("login: %d", got)
	}
}

func TestARequiredSecondFactorLimitsTheSession(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	access := root.ok("GET", "/api/v1/admin/settings/access", nil)
	access["second_factor"] = "members"
	root.ok("PUT", "/api/v1/admin/settings/access", access)

	anna := e.user("anna@example.com", identity.PermUseProduct)
	out := anna.want(403, "GET", "/api/v1/spaces", nil)
	if !strings.Contains(out, "second_factor_required") {
		t.Fatal(out)
	}
	if anna.ok("GET", "/api/v1/me", nil)["needs_second_factor"] != true {
		t.Fatal("the app must know, to show the setup")
	}
	anna.want(200, "GET", "/api/v1/me/security", nil)
}

func TestLoginHistoryKeepsTheLastLogins(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	for range 22 {
		e.login(e.client(), "anna@example.com", adminPassword)
	}
	list := anna.ok("GET", "/api/v1/me/logins", nil)["logins"].([]any)
	if len(list) != 20 || list[0].(map[string]any)["method"] != "password" {
		t.Fatalf("history: %d", len(list))
	}
}

func TestServiceAccountsWorkOnlyThroughTokens(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	account := root.ok("POST", "/api/v1/admin/service-accounts", map[string]any{"name": "Backup script"})
	id := account["id"].(string)
	ends := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	secret := root.ok("POST", "/api/v1/admin/service-accounts/"+id+"/tokens", map[string]any{"name": "t", "expires_at": ends})["secret"].(string)
	if status, _ := e.bearer("GET", "/api/v1/me", secret, ""); status != 200 {
		t.Fatalf("a service token: %d", status)
	}
	if got := e.login(e.client(), account["email"].(string), adminPassword); got == 200 {
		t.Fatal("a service account logged in")
	}
	root.want(409, "POST", "/api/v1/admin/service-accounts/"+root.id+"/tokens", map[string]any{"name": "t", "expires_at": ends})
	if has(namesIn(root.ok("GET", "/api/v1/people", nil)), "Backup script") {
		t.Fatal("a service account must not show in the directory")
	}
}

func TestAPersonalSpaceIsOrdinaryAndPrivate(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct, identity.PermPersonalSpace)
	other := e.user("other@example.com", identity.PermUseProduct)
	root := admin(e)
	other.want(403, "POST", "/api/v1/me/personal-space", nil)

	space := anna.ok("POST", "/api/v1/me/personal-space", nil)
	id := space["id"].(string)
	if space["kind"] != "personal" || !strings.HasPrefix(space["key"].(string), "~") {
		t.Fatalf("space: %v", space)
	}
	anna.want(409, "POST", "/api/v1/me/personal-space", nil)
	other.want(404, "GET", "/api/v1/spaces/"+id, nil)
	if anna.ok("GET", "/api/v1/me", nil)["personal_space"] != space["key"] {
		t.Fatal("the account menu needs the key")
	}
	if list := anna.ok("GET", "/api/v1/space-directory?kind=personal", nil)["spaces"].([]any); len(list) != 1 {
		t.Fatalf("personal filter: %v", list)
	}

	out := anna.want(409, "PUT", "/api/v1/spaces/"+id+"/grants", map[string]any{
		"subject": map[string]any{"type": "user", "id": anna.id}, "permissions": permissions.Templates[permissions.TemplateViewer]})
	if !strings.Contains(out, "owner_grant") {
		t.Fatal(out)
	}

	// The owner holds an ordinary grant, so losing it means losing access (D1).
	root.ok("POST", "/api/v1/admin/users/"+anna.id+"/disable", nil)
	unmanaged := root.ok("GET", "/api/v1/admin/attention", nil)["unmanaged_spaces"].([]any)
	if len(unmanaged) != 1 || unmanaged[0].(map[string]any)["id"] != id {
		t.Fatalf("no active administrator: %v", unmanaged)
	}
	root.ok("POST", "/api/v1/admin/spaces/"+id+"/handover", map[string]any{"to": other.id})
	other.want(200, "GET", "/api/v1/spaces/"+id, nil)
	root.want(404, "GET", "/api/v1/spaces/"+id, nil)
}

func TestWhoHasAccessShowsHowAndExportsSafely(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	space, _ := root.space("ENG")
	evil := e.user("evil@example.com", identity.PermUseProduct)
	viewer := e.user("viewer@example.com", identity.PermUseProduct)
	e.sql("UPDATE users SET display_name = '=HYPERLINK(1)' WHERE id = $1", evil.id)
	root.grant(space, "user", evil.id, permissions.TemplateViewer)
	root.grant(space, "user", viewer.id, permissions.TemplateViewer)
	group := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Team"})["id"].(string)
	root.ok("PUT", "/api/v1/admin/groups/"+group+"/members/"+viewer.id, nil)
	root.grant(space, "group", group, permissions.TemplateEditor)
	root.grant(space, "all_members", "", permissions.TemplateViewer)

	list := root.ok("GET", "/api/v1/spaces/"+space+"/access", nil)["people"].([]any)
	var viaGroup bool
	for _, item := range list {
		row := item.(map[string]any)
		if row["via"] == "Team" && row["source"] == "group" {
			viaGroup = true
		}
	}
	if !viaGroup {
		t.Fatalf("how someone got access must show: %v", list)
	}
	viewer.want(403, "GET", "/api/v1/spaces/"+space+"/access", nil)
	if !has(namesIn(viewer.ok("GET", "/api/v1/spaces/"+space+"/people", nil)), "Team") {
		t.Fatal("someone who can view may see who has access")
	}

	visitor := e.user("visitor@example.com", identity.PermUseProduct)
	if _, err := e.app.Pool().Exec(t.Context(), "UPDATE users SET type = 'guest' WHERE id = $1", visitor.id); err != nil {
		t.Fatal(err)
	}
	root.grant(space, "user", visitor.id, permissions.TemplateViewer)
	guests := map[string]bool{}
	for _, item := range viewer.ok("GET", "/api/v1/spaces/"+space+"/people", nil)["people"].([]any) {
		row := item.(map[string]any)
		guests[row["name"].(string)] = row["guest"] == true
	}
	if len(guests) < 2 || !guests["Test visitor@example.com"] || guests["Team"] {
		t.Fatalf("the people of a space say who is a guest, and only that: %v", guests)
	}

	out := root.want(200, "GET", "/api/v1/spaces/"+space+"/access.csv", nil)
	if strings.Contains(out, "\n=HYPERLINK") || !strings.Contains(out, "'=HYPERLINK") {
		t.Fatalf("a formula passed: %s", out)
	}
	if e.auditActions("space.access_exported") != 1 {
		t.Fatal("an export must be audited")
	}

	preview := root.ok("POST", "/api/v1/spaces/"+space+"/grants/preview", map[string]any{
		"subject": map[string]any{"type": "group", "id": group}, "permissions": []string{}})
	if preview["loses_view"] != true || preview["people"].(float64) != 1 {
		t.Fatalf("preview: %v", preview)
	}
}

func TestGroupOwnersManageMembersOnly(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	owner := e.user("owner@example.com", identity.PermUseProduct)
	other := e.user("other@example.com", identity.PermUseProduct)
	group := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Team"})["id"].(string)
	root.ok("PUT", "/api/v1/admin/groups/"+group+"/owners", map[string]any{"owners": []string{owner.id}})

	other.want(404, "GET", "/api/v1/groups/"+group, nil)
	owner.want(200, "GET", "/api/v1/groups/"+group, nil)
	owner.ok("PUT", "/api/v1/groups/"+group+"/members/"+other.id, nil)
	if members := root.ok("GET", "/api/v1/admin/groups/"+group+"/members", nil)["members"].([]any); len(members) != 1 {
		t.Fatalf("members: %v", members)
	}
	owner.want(403, "PATCH", "/api/v1/admin/groups/"+group, map[string]any{"name": "x"})
	other.want(404, "DELETE", "/api/v1/groups/"+group+"/members/"+other.id, nil)
	owner.ok("DELETE", "/api/v1/groups/"+group+"/members/"+other.id, nil)

	e.sql("UPDATE groups SET source = 'scim' WHERE id = $1", group)
	owner.want(409, "PUT", "/api/v1/groups/"+group+"/members/"+other.id, nil)
}

func TestDeletingAGroupSaysWhatItTouches(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	group := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Team"})["id"].(string)
	space, _ := root.space("ENG")
	root.grant(space, "group", group, permissions.TemplateViewer)
	impact := root.ok("GET", "/api/v1/admin/groups/"+group+"/impact", nil)
	if list := impact["spaces"].([]any); len(list) != 1 || list[0] != "Space ENG" {
		t.Fatalf("impact: %v", impact)
	}
}

func TestASpaceAuditHidesWhatTheAdministratorMayNotSee(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	writer := e.user("writer@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", writer.id, permissions.TemplateManager)
	hidden := writer.page(space, nil, "Secret title", "x")
	writer.restrict(hidden, "view", writer.id)
	writer.ok("POST", "/api/v1/pages/"+hidden+"/publish", map[string]any{"title": "Secret title", "document": para("y"), "base_version": 1})

	out := owner.want(200, "GET", "/api/v1/spaces/"+space+"/audit", nil)
	if strings.Contains(out, "Secret title") || strings.Contains(out, hidden) {
		t.Fatalf("a hidden page shows: %s", out)
	}
	if !strings.Contains(out, "page.published") {
		t.Fatalf("the action and time must stay: %s", out)
	}
	writer.want(403, "GET", "/api/v1/spaces/"+space+"/audit", nil)
}

// After anonymising, the old name and address may be nowhere but the audit log. The
// walk covers every text and JSON column of every table, so a new table counts at once.
func TestAnonymisingLeavesTheNameInNoTable(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	name, email := "Unique Annika Visserius", "annika.visserius@example.com"
	anna := e.user(email, identity.PermUseProduct, identity.PermCreateSpace, identity.PermPersonalSpace)
	root.want(200, "GET", "/api/v1/admin/users/"+anna.id, nil)
	root.ok("PATCH", "/api/v1/admin/users/"+anna.id, map[string]any{"display_name": name, "type": "member"})
	anna.ok("PUT", "/api/v1/me/profile", map[string]any{"job_title": "Keeps a secret title", "about": name})
	space, _ := anna.space("OWN")
	page := anna.page(space, nil, "Her page", "text")
	anna.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "t", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)})
	root.want(400, "POST", "/api/v1/admin/users/"+anna.id+"/anonymise", map[string]any{"reason": ""})
	root.want(409, "POST", "/api/v1/admin/users/"+root.id+"/anonymise", map[string]any{"reason": "no"})

	root.ok("POST", "/api/v1/admin/users/"+anna.id+"/anonymise", map[string]any{"reason": "Request by mail"})
	anna.want(401, "GET", "/api/v1/me", nil)

	ctx := context.Background()
	rows, err := e.app.Pool().Query(ctx, `SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND data_type IN ('text', 'jsonb', 'json', 'character varying', 'ARRAY')
		  AND table_name <> 'schema_migrations'`)
	if err != nil {
		t.Fatal(err)
	}
	type column struct{ table, name string }
	columns, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (column, error) {
		var c column
		err := r.Scan(&c.table, &c.name)
		return c, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) < 30 {
		t.Fatalf("only %d columns walked", len(columns))
	}
	for _, c := range columns {
		ident := pgx.Identifier{c.table}.Sanitize() + "." + pgx.Identifier{c.name}.Sanitize()
		query := fmt.Sprintf("SELECT count(*) FROM %s WHERE %s::text ILIKE '%%' || $1 || '%%' OR %s::text ILIKE '%%' || $2 || '%%'",
			pgx.Identifier{c.table}.Sanitize(), ident, ident)
		var n int
		if err := e.app.Pool().QueryRow(ctx, query, "Visserius", "annika.visserius").Scan(&n); err != nil {
			t.Fatalf("%s: %v", ident, err)
		}
		if n > 0 {
			t.Errorf("the old name is still in %s.%s", c.table, c.name)
		}
	}
	// The work stays, with a neutral name.
	var author string
	if err := e.app.Pool().QueryRow(ctx, `SELECT u.display_name FROM pages p JOIN users u ON u.id = p.created_by WHERE p.id = $1`, page).Scan(&author); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(author, "Former user") {
		t.Fatalf("author: %q", author)
	}
	if e.auditActions("user.anonymised") != 1 {
		t.Fatal("anonymising must be audited")
	}
}

func TestAPersonCanTakeTheirDataAndAskForRemoval(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	anna := e.user("anna@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := anna.space("OWN")
	anna.page(space, nil, "My page", "x")
	out := anna.want(200, "POST", "/api/v1/me/export", nil)
	if !strings.Contains(out, "anna@example.com") || !strings.Contains(out, "My page") {
		t.Fatalf("export: %s", out)
	}
	anna.ok("POST", "/api/v1/me/removal-request", nil)
	items := root.ok("GET", "/api/v1/admin/attention", nil)["items"].([]any)
	found := false
	for _, item := range items {
		found = found || item.(map[string]any)["kind"] == "removal_requests"
	}
	if !found {
		t.Fatalf("an administrator must see the request: %v", items)
	}
}

func TestTheConsoleCountsWithoutShowingContent(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	plain := e.user("plain@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := plain.space("ENG")
	plain.page(space, nil, "Secret page title", "x")
	plain.want(403, "GET", "/api/v1/admin/summary", nil)

	summary := root.ok("GET", "/api/v1/admin/summary", nil)
	if summary["users"].(float64) != 2 || summary["pages"].(float64) != 2 {
		t.Fatalf("summary: %v", summary)
	}
	if out := root.want(200, "GET", "/api/v1/admin/reports/largest", nil); strings.Contains(out, "Secret page title") {
		t.Fatalf("a report shows content: %s", out)
	}
	if !strings.Contains(root.want(200, "GET", "/api/v1/admin/attention", nil), "mail_not_set") {
		t.Fatal("no mail must be on the list")
	}
	users := root.ok("GET", "/api/v1/admin/users?type=guest", nil)
	if users["total"].(float64) != 0 {
		t.Fatalf("filter on type: %v", users)
	}
	bulk := root.ok("POST", "/api/v1/admin/users/bulk", map[string]any{"action": "disable", "users": []string{plain.id, root.id}})
	if bulk["done"].(float64) != 1 || bulk["failed"].(float64) != 1 {
		t.Fatalf("bulk: %v", bulk)
	}
	root.want(400, "POST", "/api/v1/admin/users/bulk", map[string]any{"action": "delete", "users": []string{plain.id}})
}

func TestUnusedFilesAreReportedBeforeAnythingIsRemoved(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	out := root.ok("GET", "/api/v1/admin/storage/unused", nil)
	if len(out["files"].([]any)) != 0 {
		t.Fatalf("a fresh install has no unused files: %v", out)
	}
	if root.ok("POST", "/api/v1/admin/storage/unused", nil)["removed"].(float64) != 0 {
		t.Fatal("nothing to remove")
	}
	if e.auditActions("storage.unused_removed") != 1 {
		t.Fatal("removal must be audited")
	}
}

func TestANewSpaceCanStartOpenAndWithStarterText(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	member := e.user("member@example.com", identity.PermUseProduct)
	space := owner.ok("POST", "/api/v1/spaces", map[string]any{"key": "OPEN", "name": "Open", "preset": "team_space", "document": para("Start here")})
	member.want(200, "GET", "/api/v1/spaces/"+space["id"].(string), nil)
	page := member.ok("GET", "/api/v1/pages/"+space["home_page_id"].(string), nil)
	if !strings.Contains(fmt.Sprint(page["document"]), "Start here") {
		t.Fatalf("starter text: %v", page["document"])
	}
	owner.want(400, "POST", "/api/v1/spaces", map[string]any{"key": "BAD", "name": "Bad", "preset": "world"})
	owner.want(400, "POST", "/api/v1/spaces", map[string]any{"key": "BAD", "name": "Bad",
		"document": map[string]any{"type": "doc", "content": []any{map[string]any{"type": "script"}}}})
}

func TestPermissionSetsOnlyFillInBoxes(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	root.ok("PUT", "/api/v1/admin/permission-sets", map[string]any{"name": "Reviewer", "permissions": []string{"space.view", "comment.add"}})
	root.want(400, "PUT", "/api/v1/admin/permission-sets", map[string]any{"name": "Bad", "permissions": []string{"everything"}})
	sets := root.ok("GET", "/api/v1/admin/permission-sets", nil)["sets"].([]any)
	if len(sets) != 1 {
		t.Fatalf("sets: %v", sets)
	}
	root.ok("DELETE", "/api/v1/admin/permission-sets/"+sets[0].(map[string]any)["id"].(string), nil)
}

func TestCategoriesComeFromOneList(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	space, _ := root.space("ENG")
	id := root.ok("POST", "/api/v1/admin/categories", map[string]any{"name": "Teams"})["id"].(string)
	root.want(409, "POST", "/api/v1/admin/categories", map[string]any{"name": "teams"})
	root.ok("PUT", "/api/v1/spaces/"+space+"/categories", map[string]any{"categories": []string{id}})
	if list := root.ok("GET", "/api/v1/space-directory?category="+id, nil)["spaces"].([]any); len(list) != 1 {
		t.Fatalf("filter: %v", list)
	}
}

func TestPreferencesHoldATimeZoneAndDateFormat(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	p := anna.ok("GET", "/api/v1/me/preferences", nil)
	p["time_zone"], p["date_format"] = "Asia/Tokyo", "iso"
	anna.ok("PUT", "/api/v1/me/preferences", p)
	p["time_zone"] = "Mars/Base"
	anna.want(400, "PUT", "/api/v1/me/preferences", p)
	p["time_zone"], p["date_format"] = "Asia/Tokyo", "odd"
	anna.want(400, "PUT", "/api/v1/me/preferences", p)
}

func TestPruningLeavesAnAnchorThatVerificationStartsFrom(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin(e)
	if err := e.app.SignCheckpointForTest(ctx); err != nil {
		t.Fatal(err)
	}
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	cut := time.Now().Add(time.Hour)
	done, err := audit.Prune(ctx, e.app.Pool(), cut, key)
	if err != nil {
		t.Fatal(err)
	}
	if done.Removed == 0 {
		t.Fatal("nothing was removed")
	}
	res, err := audit.Verify(ctx, e.app.Pool())
	if err != nil || !res.OK {
		t.Fatalf("after pruning: %+v, %v", res, err)
	}
	// The protection is back: a row cannot be deleted afterwards.
	if _, err := e.app.Pool().Exec(ctx, "DELETE FROM audit_events"); err == nil {
		t.Fatal("the append-only trigger stayed off")
	}
	// Changing the anchor is noticed, because its signature no longer fits.
	if _, err := e.app.Pool().Exec(ctx, "UPDATE audit_anchors SET signature = decode(repeat('00', 64), 'hex')"); err != nil {
		t.Fatal(err)
	}
	res, err = audit.Verify(ctx, e.app.Pool())
	if err != nil || res.OK {
		t.Fatalf("a changed anchor passed: %+v, %v", res, err)
	}
}

func TestPruningADamagedChainIsRefused(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin(e)
	if err := e.app.SealAudit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Pool().Exec(ctx, "ALTER TABLE audit_events DISABLE TRIGGER audit_events_no_update_delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Pool().Exec(ctx, "UPDATE audit_events SET action = 'x' WHERE seq = (SELECT min(seq) FROM audit_events)"); err != nil {
		t.Fatal(err)
	}
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	if _, err := audit.Prune(ctx, e.app.Pool(), time.Now().Add(time.Hour), key); err == nil {
		t.Fatal("a damaged chain was pruned")
	}
}

func TestMaintenanceHoldsQueuedJobs(t *testing.T) {
	e := newEnv(t)
	a := admin(e)
	// A kind this node knows: a worker leaves kinds it does not know for a node that does.
	e.app.Jobs().Register("test.nothing", func(context.Context, jobs.Job) (any, error) { return nil, nil })
	e.sql("INSERT INTO jobs (id, kind, status, created_by) VALUES (gen_random_uuid(), 'test.nothing', 'queued', $1)", a.id)
	a.ok("PUT", "/api/v1/admin/maintenance", map[string]any{"on": true, "message": ""})
	ran, err := e.app.Jobs().RunOnce(context.Background())
	if err != nil || ran {
		t.Fatalf("ran=%v err=%v during maintenance", ran, err)
	}
	a.ok("PUT", "/api/v1/admin/maintenance", map[string]any{"on": false, "message": ""})
	if ran, err := e.app.Jobs().RunOnce(context.Background()); err != nil || !ran {
		t.Fatalf("ran=%v err=%v after maintenance", ran, err)
	}
}

func TestImportPreviewsEachLineAndInvitesTheGoodOnes(t *testing.T) {
	e := newEnv(t)
	a := admin(e)
	body := "new.person@example.com,member\nnot an address\nguest.person@example.com,guest\nx@example.com,king\n" + a.email + "\n"
	preview := a.ok("POST", "/api/v1/admin/users/import?preview=1", body)
	want := []string{"", "invalid_address", "", "invalid_type", "exists"}
	rows, _ := preview["rows"].([]any)
	if len(rows) != len(want) {
		t.Fatalf("rows: %v", preview)
	}
	for i, row := range rows {
		if got := row.(map[string]any)["problem"]; got != want[i] {
			t.Errorf("line %d: %v, want %q", i+1, got, want[i])
		}
	}
	if list := a.ok("GET", "/api/v1/admin/invitations", nil); len(list["invitations"].([]any)) != 0 {
		t.Fatal("a preview invited someone")
	}
	a.ok("POST", "/api/v1/admin/users/import", body)
	if list := a.ok("GET", "/api/v1/admin/invitations", nil); len(list["invitations"].([]any)) != 2 {
		t.Fatalf("invitations: %v", list)
	}
}

func TestAPasskeyStepUpOnlyAcceptsAFinishedChallenge(t *testing.T) {
	e := newEnv(t)
	u := e.user("nokey@example.com", identity.PermUseProduct)
	u.want(400, "POST", "/api/v1/auth/reauth/passkey/begin", nil)
	u.want(401, "POST", "/api/v1/auth/reauth/passkey/finish", map[string]any{"token": "made-up", "response": json.RawMessage("{}")})
}

func TestStepUpThroughAnIdentityProviderAsksAgain(t *testing.T) {
	e := newEnvWithOIDC(t, newIDP(t))
	start := func(query string) string {
		resp, _ := e.do(noRedirect(), "GET", "/api/v1/auth/oidc/start"+query, "")
		return resp.Header.Get("Location")
	}
	if got := start("?prompt=login"); !strings.Contains(got, "prompt=login") {
		t.Fatalf("redirect: %s", got)
	}
	if plain := start(""); strings.Contains(plain, "prompt=login") {
		t.Fatalf("a normal login asks again: %s", plain)
	}
}
