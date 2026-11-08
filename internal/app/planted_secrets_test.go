// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/diffluence/diffluence/internal/config"
)

// lockedBuffer collects log lines from the handlers that run at the same time.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Secrets must never reach a log line, an audit detail or an event (SEC-1025). The test
// plants a password, a new password, a token and a session, and looks for each afterwards.
func TestPlantedSecretsAppearInNoLogAuditOrEvent(t *testing.T) {
	logs := &lockedBuffer{}
	e := newEnvLogging(t, func(*config.Config) {}, slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	u := e.user("sam@example.com")
	const wrong = "planted-wrong-password-4711"
	const next = "planted new password with words"
	if code := e.login(e.client(), "sam@example.com", wrong); code == http.StatusOK {
		t.Fatal("a wrong password logged in")
	}
	token := u.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "planted"})["secret"].(string)
	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
	}
	u.ok("POST", "/api/v1/me/password", map[string]any{"current_password": adminPassword, "new_password": next})

	serverURL, _ := url.Parse(e.srv.URL)
	planted := map[string]string{"the wrong password": wrong, "the old password": adminPassword, "the new password": next, "the token": token}
	for _, cookie := range u.c.Jar.Cookies(serverURL) {
		planted["cookie "+cookie.Name] = cookie.Value
	}
	var stored strings.Builder
	ctx := context.Background()
	for _, query := range []string{
		"SELECT coalesce(string_agg(payload, ' '), '') FROM audit_events",
		"SELECT coalesce(string_agg(payload::text, ' '), '') FROM outbox_events",
	} {
		var text string
		if err := e.app.Pool().QueryRow(ctx, query).Scan(&text); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		stored.WriteString(text)
	}
	for what, secret := range planted {
		if len(secret) < 8 {
			continue
		}
		if strings.Contains(logs.String(), secret) {
			t.Errorf("%s is in the log", what)
		}
		if strings.Contains(stored.String(), secret) {
			t.Errorf("%s is in an audit detail or an event", what)
		}
	}
	if !strings.Contains(logs.String(), "/api/v1/me/password") {
		t.Fatal("the log caught nothing, so the test proves nothing")
	}
}
