// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// signInOrSave holds sign-in requests and lets every other request through at once, saying
// so on reached, so a test can fill one class and watch the other.
func signInOrSave(signIn *hold, reached chan string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
			signIn.ServeHTTP(w, r)
			return
		}
		reached <- r.URL.Path
	})
}

// A rush of sign-ins, each waiting for its password hash, keeps to its own share and
// leaves saving free.
func TestSignInsCannotFillTheRoomOfSaving(t *testing.T) {
	a := quickAdmission(4)
	signIn := newHold()
	reached := make(chan string, 1)
	handler := a.middleware(signInOrSave(signIn, reached))
	defer close(signIn.release)
	for range 4 {
		go answer(handler, http.MethodPost, "/api/v1/auth/login")
	}
	<-signIn.in
	time.Sleep(100 * time.Millisecond)

	go answer(handler, http.MethodPost, "/api/v1/pages")
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("a save waited behind sign-ins")
	}
}

// A sign-in still gets in while saving is full.
func TestASignInGetsInWhileSavingIsFull(t *testing.T) {
	a := quickAdmission(4)
	save := newHold()
	handler := a.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" {
			w.WriteHeader(http.StatusOK)
			return
		}
		save.ServeHTTP(w, r)
	}))
	savers := fill(t, handler, http.MethodPost, "/api/v1/pages", a.gates[classSave].size(), save.in)
	defer func() {
		close(save.release)
		savers.Wait()
	}()
	if rec := answer(handler, http.MethodPost, "/api/v1/auth/login"); rec.Code != http.StatusOK {
		t.Fatalf("a sign-in answered %d while saving was full", rec.Code)
	}
}

func TestSignInRoutesHaveTheirOwnClass(t *testing.T) {
	for _, path := range []string{"/api/v1/auth/login", "/api/v1/auth/login/totp", "/api/v1/auth/forgot", "/api/v1/auth/signup", "/api/v1/setup"} {
		if got := classOf(httptest.NewRequest(http.MethodPost, path, nil)); got != classSignIn {
			t.Errorf("POST %s: class %d, want sign-in", path, got)
		}
	}
	if got := classOf(httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)); got != classRead {
		t.Errorf("a read below /auth stays a read: class %d", got)
	}
}
