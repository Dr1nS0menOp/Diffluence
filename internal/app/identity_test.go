// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/testutil"
)

// givePassword sets a password for an account directly, as only a test does: no route
// lets anyone choose a password for someone else.
func (e *env) givePassword(userID, password string) {
	e.t.Helper()
	hash, err := identity.HashPassword(context.Background(), password, identity.DefaultParams)
	if err != nil {
		e.t.Fatal(err)
	}
	e.sql(`INSERT INTO local_credentials (user_id, password_hash) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET password_hash = EXCLUDED.password_hash`, userID, hash)
}

func (u *user) password(path string) map[string]any {
	u.e.t.Helper()
	return u.ok("POST", path, map[string]any{"password": adminPassword})
}

func (u *user) enrolTOTP() (secret string, recovery []string) {
	u.e.t.Helper()
	setup := u.password("/api/v1/me/totp/begin")
	secret = setup["secret"].(string)
	if !strings.HasPrefix(setup["qr_code"].(string), "data:image/png;base64,") || !strings.HasPrefix(setup["url"].(string), "otpauth://totp/") {
		u.e.t.Fatalf("setup: %+v", setup)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		u.e.t.Fatal(err)
	}
	m := u.ok("POST", "/api/v1/me/totp/confirm", map[string]any{"code": code})
	for _, c := range m["recovery_codes"].([]any) {
		recovery = append(recovery, c.(string))
	}
	return secret, recovery
}

func (e *env) loginBody(c *http.Client, email string) map[string]any {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "password": adminPassword})
	resp, body := e.do(c, "POST", "/api/v1/auth/login", string(b))
	if resp.StatusCode != 200 {
		e.t.Fatalf("login: %d %s", resp.StatusCode, body)
	}
	m := map[string]any{}
	_ = json.Unmarshal([]byte(body), &m)
	return m
}

func (e *env) secondStep(c *http.Client, token, code string) int {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"mfa_token": token, "code": code})
	resp, _ := e.do(c, "POST", "/api/v1/auth/login/totp", string(b))
	return resp.StatusCode
}

func TestTOTP(t *testing.T) {
	e := newEnv(t)
	u := e.user("a@example.com", identity.PermUseProduct)
	secret, recovery := u.enrolTOTP()
	if len(recovery) != 10 {
		t.Fatalf("recovery codes: %d", len(recovery))
	}

	var sealed []byte
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT secret_sealed FROM totp_credentials").Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte(secret)) {
		t.Fatal("the TOTP seed is stored in the clear")
	}
	var stored int
	_ = e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM recovery_codes WHERE code_hash = $1", []byte(recovery[0])).Scan(&stored)
	if stored != 0 {
		t.Fatal("recovery codes are stored in the clear")
	}

	// The password alone yields no session.
	c := e.client()
	first := e.loginBody(c, "a@example.com")
	if first["mfa_required"] != true || first["user"] != nil {
		t.Fatalf("first step: %+v", first)
	}
	if resp, _ := e.do(c, "GET", "/api/v1/me", ""); resp.StatusCode != 401 {
		t.Fatalf("a session exists after the password step: %d", resp.StatusCode)
	}
	token := first["mfa_token"].(string)

	if got := e.secondStep(c, token, "000000"); got != 401 {
		t.Fatalf("wrong code: %d", got)
	}
	// The code from enrolment was already used, so the next step's code is needed.
	next, _ := totp.GenerateCode(secret, time.Now().Add(30*time.Second))
	if got := e.secondStep(c, token, next); got != 200 {
		t.Fatalf("right code: %d", got)
	}
	if resp, _ := e.do(c, "GET", "/api/v1/me", ""); resp.StatusCode != 200 {
		t.Fatal("no session after the second step")
	}

	// A code cannot be used twice, and a challenge cannot be used twice.
	if got := e.secondStep(e.client(), token, next); got != 401 {
		t.Fatalf("challenge reused: %d", got)
	}
	c2 := e.client()
	again := e.loginBody(c2, "a@example.com")["mfa_token"].(string)
	if got := e.secondStep(c2, again, next); got != 401 {
		t.Fatalf("code replayed: %d", got)
	}

	// A recovery code works exactly once.
	if got := e.secondStep(c2, again, strings.ToUpper(recovery[0])); got != 200 {
		t.Fatalf("recovery code: %d", got)
	}
	c3 := e.client()
	third := e.loginBody(c3, "a@example.com")["mfa_token"].(string)
	if got := e.secondStep(c3, third, recovery[0]); got != 401 {
		t.Fatalf("recovery code reused: %d", got)
	}
	if e.auditActions("auth.recovery_code_used") != 1 || e.auditActions("auth.totp_enabled") != 1 {
		t.Fatal("factors must be audited")
	}

	// Guessing ends the challenge after a few tries, even with the right code then.
	c4 := e.client()
	fourth := e.loginBody(c4, "a@example.com")["mfa_token"].(string)
	for range 6 {
		e.secondStep(c4, fourth, "000000")
	}
	if got := e.secondStep(c4, fourth, recovery[1]); got != 401 {
		t.Fatalf("challenge still works after too many tries: %d", got)
	}

	// Removing it needs the password.
	u.want(409, "POST", "/api/v1/me/totp/disable", map[string]any{"password": "wrong password here"})
	u.password("/api/v1/me/totp/disable")
	if m := e.loginBody(e.client(), "a@example.com"); m["mfa_required"] == true {
		t.Fatal("TOTP still active after disabling")
	}
}

func TestPasswordChangeAndSessions(t *testing.T) {
	e := newEnv(t)
	u := e.user("a@example.com", identity.PermUseProduct)
	other := e.client()
	if e.login(other, "a@example.com", adminPassword) != 200 {
		t.Fatal("second login failed")
	}
	sessions := u.ok("GET", "/api/v1/me/sessions", nil)["sessions"].([]any)
	if len(sessions) != 2 {
		t.Fatalf("sessions: %+v", sessions)
	}

	next := "a brand new long passphrase"
	u.want(409, "POST", "/api/v1/me/password", map[string]any{"current_password": "wrong password here", "new_password": next})
	u.want(400, "POST", "/api/v1/me/password", map[string]any{"current_password": adminPassword, "new_password": "short"})
	u.ok("POST", "/api/v1/me/password", map[string]any{"current_password": adminPassword, "new_password": next})

	if resp, _ := e.do(other, "GET", "/api/v1/me", ""); resp.StatusCode != 401 {
		t.Fatal("the other session must end when the password changes")
	}
	u.ok("GET", "/api/v1/me", nil)
	if e.login(e.client(), "a@example.com", adminPassword) != 401 || e.login(e.client(), "a@example.com", next) != 200 {
		t.Fatal("the new password must replace the old one")
	}
}

func TestAdminUserManagement(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.PermUseProduct, identity.PermAdministrator)
	root := e.user("root@example.com", identity.AllGlobalPermissions...)
	target := e.user("target@example.com", identity.PermUseProduct)

	m := admin.ok("POST", "/api/v1/admin/users", map[string]any{"email": "new@example.com", "display_name": "New", "password": adminPassword})
	perms := fmt.Sprint(m["global_permissions"])
	if !strings.Contains(perms, identity.PermUseProduct) || !strings.Contains(perms, identity.PermViewProfiles) {
		t.Fatalf("defaults: %+v", m)
	}
	admin.want(409, "POST", "/api/v1/admin/users", map[string]any{"email": "NEW@example.com", "display_name": "x", "password": adminPassword})
	g := admin.ok("POST", "/api/v1/admin/users", map[string]any{"email": "g@example.com", "display_name": "Guest", "type": "guest", "password": adminPassword})
	if strings.Contains(fmt.Sprint(g["global_permissions"]), identity.PermViewProfiles) {
		t.Fatalf("guests must not see profiles by default: %+v", g)
	}

	// Disabling ends the session at once and blocks new logins.
	untouched := e.client()
	if e.login(untouched, "target@example.com", adminPassword) != 200 {
		t.Fatal("second login failed")
	}
	admin.ok("POST", "/api/v1/admin/users/"+target.id+"/disable", nil)
	target.want(401, "GET", "/api/v1/me", nil)
	if e.login(e.client(), "target@example.com", adminPassword) != 401 {
		t.Fatal("a disabled user logged in")
	}
	admin.ok("POST", "/api/v1/admin/users/"+target.id+"/enable", nil)
	if e.login(e.client(), "target@example.com", adminPassword) != 200 {
		t.Fatal("enabling failed")
	}
	// Enabling an account must not bring its old sessions back to life.
	if resp, _ := e.do(untouched, "GET", "/api/v1/me", ""); resp.StatusCode != 401 {
		t.Fatalf("an old session survived disable and enable: %d", resp.StatusCode)
	}

	admin.want(409, "POST", "/api/v1/admin/users/"+admin.id+"/disable", nil)

	// The highest permission stays with those who hold it.
	admin.want(409, "PUT", "/api/v1/admin/users/"+admin.id+"/permissions",
		map[string]any{"permissions": []string{identity.PermUseProduct, identity.PermAdministrator, identity.PermSystemAdministrator}})
	root.ok("PUT", "/api/v1/admin/users/"+admin.id+"/permissions",
		map[string]any{"permissions": []string{identity.PermUseProduct, identity.PermAdministrator, identity.PermSystemAdministrator}})
	if e.auditActions("permission.global.granted") == 0 {
		t.Fatal("global grants must be audited")
	}

	// A non-admin gets nowhere.
	target2 := e.user("t2@example.com", identity.PermUseProduct)
	target2.want(403, "GET", "/api/v1/admin/users", nil)
}

func TestLastAdministratorStays(t *testing.T) {
	e := newEnv(t)
	root := e.user("root@example.com", identity.AllGlobalPermissions...)
	out := root.want(409, "PUT", "/api/v1/admin/users/"+root.id+"/permissions", map[string]any{"permissions": []string{identity.PermUseProduct, identity.PermSystemAdministrator}})
	if !strings.Contains(out, "last_administrator") {
		t.Fatal(out)
	}
}

func TestGroups(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	member := e.user("member@example.com", identity.PermUseProduct)
	outsider := e.user("outsider@example.com", identity.PermUseProduct)

	grp := admin.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Finance"})["id"].(string)
	admin.want(409, "POST", "/api/v1/admin/groups", map[string]any{"name": "finance"})
	admin.ok("PUT", "/api/v1/admin/groups/"+grp+"/members/"+member.id, nil)

	// A global permission through a group.
	member.want(403, "POST", "/api/v1/spaces", map[string]any{"key": "VIAGROUP", "name": "x"})
	admin.ok("PUT", "/api/v1/admin/groups/"+grp+"/permissions", map[string]any{"permissions": []string{identity.PermCreateSpace}})
	member.ok("POST", "/api/v1/spaces", map[string]any{"key": "VIAGROUP", "name": "x"})

	// A space grant and a restriction through a group.
	space, _ := owner.space("ENG")
	owner.grant(space, "group", grp, permissions.TemplateViewer)
	owner.grant(space, "user", outsider.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Budget", "x")
	owner.ok("PUT", "/api/v1/pages/"+page+"/restrictions", map[string]any{"kind": "view", "subjects": []any{
		map[string]any{"type": "user", "id": owner.id}, map[string]any{"type": "group", "id": grp}}})
	member.ok("GET", "/api/v1/pages/"+page, nil)
	outsider.want(404, "GET", "/api/v1/pages/"+page, nil)

	// Leaving the group ends access at once.
	admin.ok("DELETE", "/api/v1/admin/groups/"+grp+"/members/"+member.id, nil)
	member.want(404, "GET", "/api/v1/pages/"+page, nil)
	admin.ok("PUT", "/api/v1/admin/groups/"+grp+"/members/"+member.id, nil)

	// A page restricted to only this group must stay closed when the group goes.
	only := owner.page(space, nil, "Closed", "x")
	owner.ok("PUT", "/api/v1/pages/"+only+"/restrictions", map[string]any{"kind": "view", "subjects": []any{
		map[string]any{"type": "user", "id": owner.id}, map[string]any{"type": "group", "id": grp}}})
	owner.restrict(only, "view", owner.id)
	owner.ok("PUT", "/api/v1/pages/"+only+"/restrictions", map[string]any{"kind": "view", "subjects": []any{
		map[string]any{"type": "user", "id": owner.id}, map[string]any{"type": "group", "id": grp}}})
	admin.ok("DELETE", "/api/v1/admin/groups/"+grp, nil)
	outsider.want(404, "GET", "/api/v1/pages/"+only, nil)
	var rows int
	_ = e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM page_restrictions WHERE subject_type = 'group' AND subject_id = $1", grp).Scan(&rows)
	if rows == 0 {
		t.Fatal("deleting a group must not lift restrictions")
	}
}

func TestRestrictionCannotLockYouOut(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	other := e.user("other@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Page", "x")
	owner.ok("PUT", "/api/v1/pages/"+page+"/restrictions", map[string]any{"kind": "view",
		"subjects": []any{map[string]any{"type": "user", "id": other.id}}})
	// The person who sets a restriction is added to it, so the page stays theirs.
	if status, out := owner.do("GET", "/api/v1/pages/"+page, nil); status != 200 {
		t.Fatalf("locked out: %d %s", status, out)
	}
	if out := owner.want(200, "GET", "/api/v1/pages/"+page+"/restrictions", nil); !strings.Contains(out, owner.id) {
		t.Fatalf("the person who set it is not on it: %s", out)
	}
}

func TestGuestsCannotBrowseTheDirectory(t *testing.T) {
	e := newEnv(t)
	member := e.user("member@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	guest := e.userOfType("guest@example.com", identity.TypeGuest, identity.PermUseProduct)
	if out := member.want(200, "GET", "/api/v1/directory?q=guest", nil); !strings.Contains(out, "guest@example.com") {
		t.Fatal(out)
	}
	guest.want(403, "GET", "/api/v1/directory?q=member", nil)
}

func TestTheUserListNamesTheSpacesOfAGuestOnly(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	owner, _, _, space := e.team("ENG")
	owner.ok("POST", "/api/v1/spaces", map[string]any{"key": "OPS", "name": "Operations"})
	guest := e.userOfType("guest@example.com", identity.TypeGuest, identity.PermUseProduct)
	owner.grant(space, "user", guest.id, permissions.TemplateViewer)

	spacesOf := func(email string) []string {
		list := admin.ok("GET", "/api/v1/admin/users?q="+email, nil)["users"].([]any)
		if len(list) != 1 {
			t.Fatalf("users for %s: %v", email, list)
		}
		return strs(list[0].(map[string]any)["spaces"])
	}
	if got := spacesOf("guest@example.com"); len(got) != 1 || got[0] != "ENG" {
		t.Fatalf("a guest shows the spaces it can open, and only those: %v", got)
	}
	if got := spacesOf("owner@example.com"); len(got) != 0 {
		t.Fatalf("members show no list: %v", got)
	}
	owner.want(403, "GET", "/api/v1/admin/users", nil)
}

func TestTheUserListFiltersByRoleAndBySpace(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	_, _, _, space := e.team("ENG")
	e.user("outsider@example.com", identity.PermUseProduct)

	emails := func(query string) []string {
		out := []string{}
		for _, u := range admin.ok("GET", "/api/v1/admin/users?"+query, nil)["users"].([]any) {
			out = append(out, u.(map[string]any)["email"].(string))
		}
		return out
	}
	if got := emails("role=administrator"); len(got) != 1 || got[0] != "admin@example.com" {
		t.Fatalf("administrators: %v", got)
	}
	inSpace := emails("space_id=" + space)
	if !slices.Contains(inSpace, "owner@example.com") || !slices.Contains(inSpace, "editor@example.com") || slices.Contains(inSpace, "outsider@example.com") {
		t.Fatalf("people who reach the space: %v", inSpace)
	}
	admin.want(400, "GET", "/api/v1/admin/users?role=anything", nil)
	admin.want(400, "GET", "/api/v1/admin/users?space_id=not-an-id", nil)
}

func TestAnAdministratorSeesTheReachOfAPersonButNoTitles(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	_, editor, _, space := e.team("ENG")
	editor.page(space, nil, "Secret plans", "x")
	editor.page(space, nil, "More plans", "x")

	editor.want(403, "GET", "/api/v1/admin/users/"+editor.id+"/reach", nil)
	admin.want(404, "GET", "/api/v1/admin/users/00000000-0000-0000-0000-000000000001/reach", nil)
	out := admin.want(200, "GET", "/api/v1/admin/users/"+editor.id+"/reach", nil)
	if strings.Contains(out, "Secret plans") {
		t.Fatalf("a title reached the administrator: %s", out)
	}
	reach := admin.ok("GET", "/api/v1/admin/users/"+editor.id+"/reach", nil)
	if reach["pages_owned"].(float64) != 2 || !has(strs(reach["spaces"]), "ENG") {
		t.Fatalf("reach: %v", reach)
	}
}

// idp is a small OpenID Connect provider for tests.
type idp struct {
	t      *testing.T
	srv    *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	codes  map[string]map[string]any // code -> claims
	nonces map[string]string
	// challenge the client sent, to check PKCE at the token step
	challenges map[string]string
	badKey     *rsa.PrivateKey // when set, sign with this one
	wrongNonce bool
}

func newIDP(t *testing.T) *idp {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &idp{t: t, key: key, codes: map[string]map[string]any{}, nonces: map[string]string{}, challenges: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": p.srv.URL, "authorization_endpoint": p.srv.URL + "/authorize",
			"token_endpoint": p.srv.URL + "/token", "jwks_uri": p.srv.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"}})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		b64 := base64.RawURLEncoding.EncodeToString
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
			"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes())}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		code := r.Form.Get("code")
		p.mu.Lock()
		claims, ok := p.codes[code]
		challenge := p.challenges[code]
		nonce := p.nonces[code]
		p.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if p.wrongNonce {
			nonce = "not-the-nonce"
		}
		all := map[string]any{"iss": p.srv.URL, "aud": "client", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": nonce}
		for k, v := range claims {
			all[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "x", "token_type": "Bearer", "id_token": p.sign(all)})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *idp) sign(claims map[string]any) string {
	b64 := base64.RawURLEncoding.EncodeToString
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	input := b64(header) + "." + b64(payload)
	key := p.key
	if p.badKey != nil {
		key = p.badKey
	}
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		p.t.Fatal(err)
	}
	return input + "." + b64(sig)
}

func noRedirect() *http.Client {
	c := (&env{}).client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// oidcLogin walks the browser's side of the flow and returns where it ends up.
func (e *env) oidcLogin(p *idp, c *http.Client, claims map[string]any) string {
	e.t.Helper()
	resp, _ := e.do(c, "GET", "/api/v1/auth/oidc/start", "")
	if resp.StatusCode != http.StatusFound {
		e.t.Fatalf("start: %d", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		e.t.Fatal(err)
	}
	q := loc.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" || q.Get("state") == "" {
		e.t.Fatalf("authorization request lacks PKCE, nonce or state: %s", loc)
	}
	code := fmt.Sprintf("code-%d", time.Now().UnixNano())
	p.mu.Lock()
	p.codes[code] = claims
	p.nonces[code] = q.Get("nonce")
	p.challenges[code] = q.Get("code_challenge")
	p.mu.Unlock()
	resp, _ = e.do(c, "GET", "/api/v1/auth/oidc/callback?state="+url.QueryEscape(q.Get("state"))+"&code="+code, "")
	return resp.Header.Get("Location")
}

func newEnvWithOIDC(t *testing.T, p *idp) *env {
	return newEnvWith(t, func(c *config.Config) {
		c.OIDC = config.OIDC{Issuer: p.srv.URL, ClientID: "client", ClientSecret: "secret", DisplayName: "Authentik", AutoCreate: true, GroupsClaim: "groups"}
	})
}

// SEC-153: on https the cookie of the login flow carries the __Secure- prefix, so a browser
// takes it only from a secure page (ASVS 3.3.1). On plain http the prefix cannot work.
func TestTheLoginFlowCookieIsSecureByNameOnHTTPS(t *testing.T) {
	p := newIDP(t)
	flowCookie := func(public string) string {
		e := newEnvWith(t, func(c *config.Config) {
			c.OIDC = config.OIDC{Issuer: p.srv.URL, ClientID: "client", ClientSecret: "secret", DisplayName: "Authentik"}
			c.PublicURL, _ = url.Parse(public)
		})
		resp, _ := e.do(noRedirect(), "GET", "/api/v1/auth/oidc/start", "")
		if resp.StatusCode != http.StatusFound {
			t.Fatalf("%s: start: %d", public, resp.StatusCode)
		}
		return resp.Header.Get("Set-Cookie")
	}
	if got := flowCookie("https://wiki.example.com"); !strings.HasPrefix(got, "__Secure-diffluence_oidc=") || !strings.Contains(got, "; Secure") {
		t.Fatalf("on https: %q", got)
	}
	if got := flowCookie("http://localhost"); !strings.HasPrefix(got, "diffluence_oidc=") {
		t.Fatalf("on http: %q", got)
	}
}

func TestOIDCLogin(t *testing.T) {
	p := newIDP(t)
	e := newEnvWithOIDC(t, p)
	verified := true

	if resp, body := e.do(e.client(), "GET", "/api/v1/auth/providers", ""); resp.StatusCode != 200 || !strings.Contains(body, "Authentik") {
		t.Fatalf("providers: %s", body)
	}

	c := noRedirect()
	if to := e.oidcLogin(p, c, map[string]any{"sub": "abc", "email": "new@example.com", "name": "New Person", "email_verified": verified, "groups": []string{"Team A"}}); to != "/" {
		t.Fatalf("first login ended at %q", to)
	}
	resp, body := e.do(c, "GET", "/api/v1/me", "")
	if resp.StatusCode != 200 || !strings.Contains(body, "New Person") {
		t.Fatalf("me: %d %s", resp.StatusCode, body)
	}
	var groups int
	if err := e.app.Pool().QueryRow(context.Background(), `SELECT count(*) FROM group_members m JOIN groups g ON g.id = m.group_id
		WHERE g.name = 'Team A' AND g.source = 'oidc'`).Scan(&groups); err != nil || groups != 1 {
		t.Fatalf("group sync: %d %v", groups, err)
	}

	// Same subject again: same account, and a removed group goes away.
	c2 := noRedirect()
	if to := e.oidcLogin(p, c2, map[string]any{"sub": "abc", "email": "changed@example.com", "name": "New Person"}); to != "/" {
		t.Fatalf("second login ended at %q", to)
	}
	var users int
	_ = e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM users").Scan(&users)
	if users != 1 {
		t.Fatalf("users: %d", users)
	}
	_ = e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM group_members").Scan(&groups)
	if groups != 0 {
		t.Fatal("a removed group claim must end the membership")
	}
}

// A new account takes its address from the provider, so the provider must say it verified
// that address: else someone could take the address of a colleague who never logged in.
func TestSingleSignOnMakesNoAccountFromAnAddressNobodyVerified(t *testing.T) {
	p := newIDP(t)
	e := newEnvWithOIDC(t, p)
	for i, claims := range []map[string]any{
		{"sub": "u-false", "email": "colleague@example.com", "email_verified": false},
		{"sub": "u-missing", "email": "colleague@example.com"},
		{"sub": "u-text", "email": "colleague@example.com", "email_verified": "true"},
	} {
		if to := e.oidcLogin(p, noRedirect(), claims); to == "/" {
			t.Fatalf("case %d: an account was made from an unverified address", i)
		}
	}
	if got := e.count("SELECT count(*) FROM users WHERE email = 'colleague@example.com'"); got != 0 {
		t.Fatalf("%d accounts were made", got)
	}
	if to := e.oidcLogin(p, noRedirect(), map[string]any{"sub": "u-ok", "email": "colleague@example.com", "email_verified": true}); to != "/" {
		t.Fatalf("a verified address ended at %q", to)
	}
}

// An operator whose provider never states the claim may accept unverified addresses.
func TestAnOperatorMayAcceptUnverifiedAddresses(t *testing.T) {
	p := newIDP(t)
	e := newEnvWith(t, func(c *config.Config) {
		c.OIDC = config.OIDC{Issuer: p.srv.URL, ClientID: "client", ClientSecret: "secret", AutoCreate: true, AllowUnverifiedEmail: true}
	})
	if to := e.oidcLogin(p, noRedirect(), map[string]any{"sub": "u-1", "email": "someone@example.com"}); to != "/" {
		t.Fatalf("ended at %q", to)
	}
}

func TestOIDCRefusals(t *testing.T) {
	p := newIDP(t)
	e := newEnvWithOIDC(t, p)
	e.user("local@example.com", identity.PermUseProduct)
	verified := true

	// A local account is never taken over by e-mail, not even a verified one.
	c := noRedirect()
	if to := e.oidcLogin(p, c, map[string]any{"sub": "attacker", "email": "local@example.com", "email_verified": verified}); !strings.Contains(to, "oidc_account_exists") {
		t.Fatalf("takeover attempt ended at %q", to)
	}
	if resp, _ := e.do(c, "GET", "/api/v1/me", ""); resp.StatusCode != 401 {
		t.Fatal("takeover produced a session")
	}

	// A wrong signature.
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	p.badKey = other
	if to := e.oidcLogin(p, noRedirect(), map[string]any{"sub": "x", "email": "x@example.com"}); !strings.Contains(to, "oidc_failed") {
		t.Fatalf("bad signature ended at %q", to)
	}
	p.badKey = nil

	// A wrong nonce.
	p.wrongNonce = true
	if to := e.oidcLogin(p, noRedirect(), map[string]any{"sub": "x", "email": "x@example.com"}); !strings.Contains(to, "oidc_failed") {
		t.Fatalf("bad nonce ended at %q", to)
	}
	p.wrongNonce = false

	// A forged state.
	c = noRedirect()
	resp, _ := e.do(c, "GET", "/api/v1/auth/oidc/callback?state=forged&code=x", "")
	if !strings.Contains(resp.Header.Get("Location"), "oidc_failed") {
		t.Fatalf("forged state: %s", resp.Header.Get("Location"))
	}

	// A state started in another browser: the login must not land in this one.
	victim, attacker := noRedirect(), noRedirect()
	resp, _ = e.do(attacker, "GET", "/api/v1/auth/oidc/start", "")
	loc, _ := url.Parse(resp.Header.Get("Location"))
	q := loc.Query()
	p.mu.Lock()
	p.codes["stolen"] = map[string]any{"sub": "attacker2", "email": "a2@example.com"}
	p.nonces["stolen"] = q.Get("nonce")
	p.challenges["stolen"] = q.Get("code_challenge")
	p.mu.Unlock()
	resp, _ = e.do(victim, "GET", "/api/v1/auth/oidc/callback?state="+url.QueryEscape(q.Get("state"))+"&code=stolen", "")
	if !strings.Contains(resp.Header.Get("Location"), "oidc_failed") {
		t.Fatalf("login CSRF: %s", resp.Header.Get("Location"))
	}

	// A disabled account cannot use OIDC.
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	c = noRedirect()
	if to := e.oidcLogin(p, c, map[string]any{"sub": "later", "email": "later@example.com", "name": "Later", "email_verified": true}); to != "/" {
		t.Fatalf("login ended at %q", to)
	}
	var id string
	_ = e.app.Pool().QueryRow(context.Background(), "SELECT id::text FROM users WHERE email = 'later@example.com'").Scan(&id)
	admin.ok("POST", "/api/v1/admin/users/"+id+"/disable", nil)
	if to := e.oidcLogin(p, noRedirect(), map[string]any{"sub": "later", "email": "later@example.com"}); !strings.Contains(to, "oidc_failed") {
		t.Fatalf("disabled user ended at %q", to)
	}
	if e.auditActions("auth.login") == 0 {
		t.Fatal("OIDC logins must be audited")
	}
}

type scimClient struct {
	e     *env
	token string
}

func (s *scimClient) do(method, path string, body any) (int, map[string]any) {
	s.e.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, s.e.srv.URL+"/scim/v2"+path, r)
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/scim+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	out := map[string]any{}
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func (s *scimClient) want(status int, method, path string, body any) map[string]any {
	s.e.t.Helper()
	got, out := s.do(method, path, body)
	if got != status {
		s.e.t.Fatalf("SCIM %s %s: %d, want %d: %+v", method, path, got, status, out)
	}
	return out
}

func newScim(t *testing.T, e *env, admin *user) (*scimClient, string) {
	t.Helper()
	m := admin.ok("POST", "/api/v1/admin/scim-tokens", map[string]any{"name": "Authentik"})
	return &scimClient{e: e, token: m["secret"].(string)}, m["token"].(map[string]any)["id"].(string)
}

func TestSCIMTokens(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	s, id := newScim(t, e, admin)

	s.want(200, "GET", "/ServiceProviderConfig", nil)
	(&scimClient{e: e, token: "dfl_scim_wrong"}).want(401, "GET", "/Users", nil)
	(&scimClient{e: e, token: ""}).want(401, "GET", "/Users", nil)

	var stored int
	_ = e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM scim_tokens WHERE token_hash = $1", []byte(s.token)).Scan(&stored)
	if stored != 0 {
		t.Fatal("the token is stored in the clear")
	}
	if out := admin.want(200, "GET", "/api/v1/admin/scim-tokens", nil); strings.Contains(out, s.token) {
		t.Fatal("the token can be read back")
	}
	admin.ok("DELETE", "/api/v1/admin/scim-tokens/"+id, nil)
	s.want(401, "GET", "/Users", nil)
	if e.auditActions("scim.token_revoked") != 1 {
		t.Fatal("token changes must be audited")
	}
}

// What Authentik sends: a user name that is not an e-mail address, and a PUT to change.
func TestSCIMAuthentikUserFlow(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	s, _ := newScim(t, e, admin)

	created := s.want(201, "POST", "/Users", map[string]any{
		"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"userName":    "jdoe",
		"displayName": "Jane Doe",
		"active":      true,
		"emails":      []any{map[string]any{"value": "jane@example.com", "type": "work", "primary": true}},
		"externalId":  "ak-1234",
	})
	id := created["id"].(string)
	if created["meta"].(map[string]any)["resourceType"] != "User" || created["userName"] != "jdoe" {
		t.Fatalf("created: %+v", created)
	}

	// Authentik looks users up before creating them.
	list := s.want(200, "GET", "/Users?filter="+url.QueryEscape(`userName eq "jdoe"`), nil)
	if list["totalResults"] != float64(1) || list["Resources"].([]any)[0].(map[string]any)["id"] != id {
		t.Fatalf("filter: %+v", list)
	}
	if none := s.want(200, "GET", "/Users?filter="+url.QueryEscape(`userName eq "nobody"`), nil); none["totalResults"] != float64(0) {
		t.Fatalf("an unknown user must not match: %+v", none)
	}
	s.want(400, "GET", "/Users?filter="+url.QueryEscape(`userName co "j"`), nil)
	s.want(409, "POST", "/Users", map[string]any{"userName": "other", "emails": []any{map[string]any{"value": "JANE@example.com"}}})

	s.want(200, "PUT", "/Users/"+id, map[string]any{"userName": "jdoe", "displayName": "Jane Smith", "active": true,
		"emails": []any{map[string]any{"value": "jane@example.com", "primary": true}}, "externalId": "ak-1234"})
	if got := s.want(200, "GET", "/Users/"+id, nil); got["displayName"] != "Jane Smith" {
		t.Fatalf("after PUT: %+v", got)
	}

	// Deactivating ends the session at once.
	e.givePassword(id, adminPassword)
	c := e.client()
	if e.login(c, "jane@example.com", adminPassword) != 200 {
		t.Fatal("login failed")
	}
	s.want(200, "PUT", "/Users/"+id, map[string]any{"userName": "jdoe", "displayName": "Jane Smith", "active": false,
		"emails": []any{map[string]any{"value": "jane@example.com", "primary": true}}})
	if resp, _ := e.do(c, "GET", "/api/v1/me", ""); resp.StatusCode != 401 {
		t.Fatal("deactivating must end the session")
	}
	if e.login(e.client(), "jane@example.com", adminPassword) != 401 {
		t.Fatal("a deactivated user logged in")
	}

	// Coming back must not revive the session from before.
	s.want(200, "PUT", "/Users/"+id, map[string]any{"userName": "jdoe", "displayName": "Jane Smith", "active": true,
		"emails": []any{map[string]any{"value": "jane@example.com", "primary": true}}})
	if resp, _ := e.do(c, "GET", "/api/v1/me", ""); resp.StatusCode != 401 {
		t.Fatal("a session survived deactivation through SCIM")
	}

	// Delete disables and keeps the account for its content and audit trail.
	s.want(204, "DELETE", "/Users/"+id, nil)
	if got := s.want(200, "GET", "/Users/"+id, nil); got["active"] != false {
		t.Fatalf("after delete: %+v", got)
	}
	evs := e.auditActions("user.disabled")
	if evs == 0 {
		t.Fatal("SCIM changes must be audited")
	}
}

// What Entra ID sends: PATCH with capitalised operations and booleans as strings.
func TestSCIMEntraUserAndGroupFlow(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	s, _ := newScim(t, e, admin)

	// Entra ID probes with a random filter first.
	if got := s.want(200, "GET", "/Users?filter="+url.QueryEscape(`userName eq "non-existent-user"`), nil); got["totalResults"] != float64(0) {
		t.Fatal("probe must find nothing")
	}
	user := s.want(201, "POST", "/Users", map[string]any{
		"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:User"}, "externalId": "obj-1",
		"userName": "ryan@example.com", "active": true,
		"name":   map[string]any{"formatted": "Ryan Leenay", "familyName": "Leenay", "givenName": "Ryan"},
		"emails": []any{map[string]any{"primary": true, "type": "work", "value": "ryan@example.com"}},
	})
	id := user["id"].(string)
	if user["displayName"] != "Ryan Leenay" {
		t.Fatalf("name: %+v", user)
	}

	s.want(200, "PATCH", "/Users/"+id, map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []any{
			map[string]any{"op": "Replace", "path": "displayName", "value": "Ryan L."},
			map[string]any{"op": "Add", "path": `emails[type eq "work"].value`, "value": "ryan2@example.com"},
			map[string]any{"op": "Replace", "path": "active", "value": "False"},
		}})
	got := s.want(200, "GET", "/Users/"+id, nil)
	if got["displayName"] != "Ryan L." || got["active"] != false || !strings.Contains(fmt.Sprint(got["emails"]), "ryan2@example.com") {
		t.Fatalf("after PATCH: %+v", got)
	}
	s.want(200, "PATCH", "/Users/"+id, map[string]any{"Operations": []any{map[string]any{"op": "replace", "value": map[string]any{"active": true}}}})
	if got := s.want(200, "GET", "/Users/"+id, nil); got["active"] != true {
		t.Fatalf("no-path PATCH: %+v", got)
	}
	if got := s.want(200, "GET", "/Users?filter="+url.QueryEscape(`externalId eq "obj-1"`), nil); got["totalResults"] != float64(1) {
		t.Fatalf("externalId filter: %+v", got)
	}

	group := s.want(201, "POST", "/Groups", map[string]any{
		"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:Group"}, "displayName": "Engineering", "externalId": "grp-1",
		"members": []any{map[string]any{"value": id}}})
	gid := group["id"].(string)
	if len(group["members"].([]any)) != 1 {
		t.Fatalf("group: %+v", group)
	}
	if got := s.want(200, "GET", "/Groups?filter="+url.QueryEscape(`displayName eq "Engineering"`)+"&excludedAttributes=members", nil); got["totalResults"] != float64(1) {
		t.Fatalf("group filter: %+v", got)
	}
	// Entra ID's membership check.
	f := fmt.Sprintf(`id eq "%s" and members.value eq "%s"`, gid, id)
	if got := s.want(200, "GET", "/Groups?filter="+url.QueryEscape(f)+"&excludedAttributes=members", nil); got["totalResults"] != float64(1) {
		t.Fatalf("membership check: %+v", got)
	}
	s.want(200, "PATCH", "/Groups/"+gid, map[string]any{"Operations": []any{
		map[string]any{"op": "Remove", "path": fmt.Sprintf(`members[value eq "%s"]`, id)}}})
	if got := s.want(200, "GET", "/Groups/"+gid, nil); len(got["members"].([]any)) != 0 {
		t.Fatalf("after remove: %+v", got)
	}
	s.want(200, "PATCH", "/Groups/"+gid, map[string]any{"Operations": []any{
		map[string]any{"op": "Add", "path": "members", "value": []any{map[string]any{"value": id}}},
		map[string]any{"op": "Replace", "path": "displayName", "value": "Eng"}}})
	got = s.want(200, "GET", "/Groups/"+gid, nil)
	if got["displayName"] != "Eng" || len(got["members"].([]any)) != 1 {
		t.Fatalf("after add: %+v", got)
	}

	// A group from SCIM cannot be changed by hand: the next sync would undo it.
	admin.want(409, "PATCH", "/api/v1/admin/groups/"+gid, map[string]any{"name": "Hacked"})
	admin.want(409, "PUT", "/api/v1/admin/groups/"+gid+"/members/"+admin.id, nil)
	admin.want(409, "DELETE", "/api/v1/admin/groups/"+gid, nil)

	s.want(204, "DELETE", "/Groups/"+gid, nil)
	s.want(404, "GET", "/Groups/"+gid, nil)
}

// A provider that created an account through SCIM must be able to log that account
// in over OIDC. Checked against real Authentik 2026.8: it sends no verified e-mail,
// but its SCIM externalId equals the token's subject, which users cannot change.
func TestOIDCLinksAccountsCreatedThroughSCIM(t *testing.T) {
	p := newIDP(t)
	e := newEnvWithOIDC(t, p)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	s, _ := newScim(t, e, admin)
	s.want(201, "POST", "/Users", map[string]any{"userName": "jdoe", "displayName": "Jane", "active": true, "externalId": "uid-jane",
		"emails": []any{map[string]any{"value": "jane@example.com", "primary": true}}})

	// Someone else sets their address at the provider to Jane's. The address is
	// not verified and the subject is not hers: no takeover.
	if to := e.oidcLogin(p, noRedirect(), map[string]any{"sub": "uid-mallory", "email": "jane@example.com", "email_verified": false}); !strings.Contains(to, "oidc_account_exists") {
		t.Fatalf("an unverified address must not link: %q", to)
	}

	c := noRedirect()
	if to := e.oidcLogin(p, c, map[string]any{"sub": "uid-jane", "email": "jane@example.com", "email_verified": false}); to != "/" {
		t.Fatalf("link by id: %q", to)
	}
	resp, body := e.do(c, "GET", "/api/v1/me", "")
	if resp.StatusCode != 200 || !strings.Contains(body, "jane@example.com") {
		t.Fatalf("me: %d %s", resp.StatusCode, body)
	}
	var users int
	_ = e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM users WHERE email = 'jane@example.com'").Scan(&users)
	if users != 1 {
		t.Fatalf("users: %d", users)
	}

	// Entra ID: the id in the token is "oid", while "sub" differs per application.
	s.want(201, "POST", "/Users", map[string]any{"userName": "ryan@example.com", "active": true, "externalId": "object-1",
		"emails": []any{map[string]any{"value": "ryan@example.com", "primary": true}}})
	if to := e.oidcLogin(p, noRedirect(), map[string]any{"sub": "pairwise", "oid": "object-1", "email": "ryan@example.com"}); to != "/" {
		t.Fatalf("link by object id: %q", to)
	}

	// A provider without a shared id can still link on an address it verified.
	s.want(201, "POST", "/Users", map[string]any{"userName": "no-id", "active": true,
		"emails": []any{map[string]any{"value": "noid@example.com", "primary": true}}})
	if to := e.oidcLogin(p, noRedirect(), map[string]any{"sub": "x1", "email": "noid@example.com", "email_verified": true}); to != "/" {
		t.Fatalf("link by verified address: %q", to)
	}

	// An account with a password is never linked, whatever the id says.
	e.givePassword(s.want(201, "POST", "/Users", map[string]any{"userName": "pw", "active": true, "externalId": "uid-pw",
		"emails": []any{map[string]any{"value": "pw@example.com", "primary": true}}})["id"].(string), adminPassword)
	if to := e.oidcLogin(p, noRedirect(), map[string]any{"sub": "uid-pw", "email": "pw@example.com", "email_verified": true}); !strings.Contains(to, "oidc_account_exists") {
		t.Fatalf("an account with a password must not link: %q", to)
	}
}

// When the provider refuses, its reason must end up in the audit log: "invalid_request"
// tells an operator where to look, a generic failure does not.
func TestOIDCProviderErrorIsRecorded(t *testing.T) {
	p := newIDP(t)
	e := newEnvWithOIDC(t, p)
	c := noRedirect()
	resp, _ := e.do(c, "GET", "/api/v1/auth/oidc/start", "")
	loc, _ := url.Parse(resp.Header.Get("Location"))
	resp, _ = e.do(c, "GET", "/api/v1/auth/oidc/callback?error=invalid_request&state="+url.QueryEscape(loc.Query().Get("state")), "")
	if !strings.Contains(resp.Header.Get("Location"), "oidc_failed") {
		t.Fatalf("redirect: %s", resp.Header.Get("Location"))
	}
	evs, err := audit.List(context.Background(), e.app.Pool(), audit.Filter{Action: "auth.login"})
	if err != nil || len(evs) != 1 || evs[0].Details["reason"] != "provider_error:invalid_request" {
		t.Fatalf("audit: %+v %v", evs, err)
	}

	// Whatever a provider or an attacker puts in the parameter stays out of the log.
	e.do(noRedirect(), "GET", "/api/v1/auth/oidc/callback?state=x&error="+url.QueryEscape("<script>alert(1)</script>"), "")
	evs, _ = audit.List(context.Background(), e.app.Pool(), audit.Filter{Action: "auth.login"})
	if evs[0].Details["reason"] != "provider_error:unknown" {
		t.Fatalf("unfiltered: %+v", evs[0].Details)
	}
}

func TestContractMigrationsWaitForOlderNodes(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.ContractGrace = 24 * time.Hour })
	ctx := context.Background()
	pool := e.app.Pool()
	if err := e.app.RegisterNode(ctx); err != nil {
		t.Fatal(err)
	}
	var known int
	if err := pool.QueryRow(ctx, "SELECT max(version) FROM schema_migrations").Scan(&known); err != nil {
		t.Fatal(err)
	}
	next := known + 1
	e.app.AddMigrationForTest(db.Migration{Version: next, Name: "drop_old", Phase: db.PhaseContract, Checksum: "x",
		SQL: "-- phase: contract\nCREATE TABLE contract_ran (id int);"})

	// This node now knows the migration, but an older one is still alive. Both started
	// longer ago than the grace, so only the older node holds the migration back.
	if _, err := pool.Exec(ctx, "UPDATE nodes SET max_migration = $1, started_at = now() - interval '2 days'", next); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO nodes (id, role, version, max_migration, started_at)
		VALUES ('00000000-0000-0000-0000-00000000000a', 'all', 'old', $1, now() - interval '3 days')`, known); err != nil {
		t.Fatal(err)
	}
	if n, err := e.app.RunContractMigrations(ctx); err != nil || n != 0 {
		t.Fatalf("ran with an older node alive: %d %v", n, err)
	}

	// The old node stops. Every node must have run the release for a day first (D1): an
	// hour, or ten minutes as before, is not enough.
	if _, err := pool.Exec(ctx, "DELETE FROM nodes WHERE version = 'old'"); err != nil {
		t.Fatal(err)
	}
	for _, since := range []string{"0 seconds", "1 hour", "23 hours 59 minutes"} {
		if _, err := pool.Exec(ctx, "UPDATE nodes SET started_at = now() - $1::interval", since); err != nil {
			t.Fatal(err)
		}
		if n, err := e.app.RunContractMigrations(ctx); err != nil || n != 0 {
			t.Fatalf("ran %s into the grace period of a day: %d %v", since, n, err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE nodes SET started_at = now() - interval '24 hours 1 minute'"); err != nil {
		t.Fatal(err)
	}
	if n, err := e.app.RunContractMigrations(ctx); err != nil || n != 1 {
		t.Fatalf("did not run: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, "SELECT * FROM contract_ran"); err != nil {
		t.Fatal(err)
	}
	if e.auditActions("system.contract_migrated") != 1 {
		t.Fatal("automatic migrations must be audited")
	}
}

func TestAuditCheckpointsAndDailyVerification(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.user("a@example.com", identity.PermUseProduct)
	if err := e.app.SignCheckpointForTest(ctx); err != nil {
		t.Fatal(err)
	}
	if res, err := e.app.CheckAudit(ctx); res.Checkpoints != 1 || !res.OK || err != nil {
		t.Fatalf("verify: %+v %v", res, err)
	}
	if e.auditActions("audit.checkpoint_signed") != 1 {
		t.Fatal("the checkpoint must be audited")
	}
	if _, err := e.app.Pool().Exec(ctx, "UPDATE audit_checkpoints SET signature = decode(repeat('00', 64), 'hex')"); err != nil {
		t.Fatal(err)
	}
	if res, err := e.app.CheckAudit(ctx); err != nil || res.OK {
		t.Fatalf("a forged signature was accepted: %+v %v", res, err)
	}

	// Someone with database access tampers with a row: the daily job must notice.
	pool := e.app.Pool()
	if _, err := pool.Exec(ctx, "ALTER TABLE audit_events DISABLE TRIGGER audit_events_no_update_delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_events SET payload = replace(payload, 'user.created', 'user.renamed') WHERE action = 'user.created'`); err != nil {
		t.Fatal(err)
	}
	if err := e.app.VerifyAuditForTest(ctx); err != nil {
		t.Fatal(err)
	}
	var result string
	if err := pool.QueryRow(ctx, "SELECT result FROM audit_entries WHERE action = 'audit.verified' ORDER BY occurred_at DESC LIMIT 1").Scan(&result); err != nil {
		t.Fatal(err)
	}
	if result != "failure" {
		t.Fatalf("tampering not reported: %s", result)
	}
}

// The audit log's protection must hold against the application itself: its
// database role can append but not change, and cannot switch the triggers off.
func TestApplicationRoleCannotTamperWithTheAuditLog(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	role := "dfl_app_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if _, err := tdb.Pool.Exec(ctx, "CREATE ROLE "+role+" LOGIN PASSWORD 'app'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = tdb.Pool.Exec(ctx, "DROP OWNED BY "+role)
		_, _ = tdb.Pool.Exec(ctx, "DROP ROLE "+role)
	})
	u, _ := url.Parse(tdb.URL)
	u.User = url.UserPassword(role, "app")

	e := newEnvWith(t, func(c *config.Config) {
		c.DatabaseURL = u.String()
		c.MigrateDatabaseURL = tdb.URL
	})
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	space, _ := admin.space("ENG")
	admin.page(space, nil, "Works", "the application runs normally")

	pool := e.app.Pool()
	for _, sql := range []string{
		"UPDATE audit_events SET action = 'x.y'",
		"DELETE FROM audit_events",
		"TRUNCATE audit_events",
		"ALTER TABLE audit_events DISABLE TRIGGER audit_events_no_update_delete",
		"DROP TRIGGER audit_events_no_update_delete ON audit_events",
		"DROP TABLE audit_events",
		"DELETE FROM schema_migrations",
		"DELETE FROM audit_checkpoints",
		// Events that wait for the sealer (ADR-041): the work above left some.
		"DELETE FROM audit_pending",
		"UPDATE audit_pending SET action = 'x.y'",
		"TRUNCATE audit_pending",
		"ALTER TABLE audit_pending DISABLE TRIGGER audit_pending_guard",
		"DROP TABLE audit_pending",
	} {
		if _, err := pool.Exec(ctx, sql); err == nil {
			t.Errorf("the application role may run: %s", sql)
		}
	}
	for table, privileges := range map[string][]string{
		"audit_events":      {"UPDATE", "DELETE", "TRUNCATE"},
		"audit_checkpoints": {"UPDATE", "DELETE", "TRUNCATE"},
		"audit_anchors":     {"INSERT", "UPDATE", "DELETE", "TRUNCATE"},
		// Only the backup service, as the owner, brings in a checkpoint key (spec 070).
		"audit_signing_keys": {"INSERT", "UPDATE", "DELETE", "TRUNCATE"},
		"audit_personal":     {"UPDATE", "TRUNCATE"},
		"audit_pending":      {"TRUNCATE"},
		"schema_migrations":  {"INSERT", "UPDATE", "DELETE", "TRUNCATE"},
	} {
		for _, privilege := range privileges {
			var may bool
			if err := pool.QueryRow(ctx, "SELECT has_table_privilege(current_user, $1, $2)", table, privilege).Scan(&may); err != nil {
				t.Fatal(err)
			}
			if may {
				t.Errorf("the application role holds %s on %s", privilege, table)
			}
		}
	}
	var owns int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_tables WHERE tableowner = current_user").Scan(&owns); err != nil {
		t.Fatal(err)
	}
	if owns != 0 {
		t.Errorf("the application role owns %d tables", owns)
	}
	// With only its own rights, the application seals what waits.
	if err := e.app.SealAudit(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := audit.Verify(ctx, pool)
	if err != nil || !res.OK || res.Checked == 0 {
		t.Fatalf("chain: %+v %v", res, err)
	}
}
