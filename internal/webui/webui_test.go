// SPDX-License-Identifier: Apache-2.0

package webui

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const testOrigin = "https://wiki.example.com"

func TestVideoAndAudioFromOurOwnOriginMayPlay(t *testing.T) {
	if got := contentPolicy("", testOrigin); !strings.Contains(got, "media-src 'self'") {
		t.Fatalf("media-src: %s", got)
	}
}

func TestOnlyOurViewersAndTheDrawingServiceMayBeFramed(t *testing.T) {
	if got := contentPolicy("", testOrigin); !strings.Contains(got, "; frame-src "+testOrigin+"/vendor/;") {
		t.Fatalf("only the viewers: %s", got)
	}
	got := contentPolicy("https://draw.example.com", testOrigin)
	if !strings.Contains(got, "; frame-src "+testOrigin+"/vendor/ https://draw.example.com;") {
		t.Fatalf("frame-src: %s", got)
	}
	if !strings.Contains(got, "frame-ancestors 'none'") {
		t.Fatalf("our own page must stay unframeable: %s", got)
	}
}

func TestANewFrontendGetsANewBuildId(t *testing.T) {
	if got := buildOf(fstest.MapFS{}); got != "" {
		t.Fatalf("a binary without a frontend must say nothing: %q", got)
	}
	first := buildOf(fstest.MapFS{"index.html": {Data: []byte(`<script src="/assets/index-a1.js">`)}})
	same := buildOf(fstest.MapFS{"index.html": {Data: []byte(`<script src="/assets/index-a1.js">`)}})
	next := buildOf(fstest.MapFS{"index.html": {Data: []byte(`<script src="/assets/index-b2.js">`)}})
	if first == "" || first != same {
		t.Fatalf("the same build must keep its id: %q %q", first, same)
	}
	if first == next {
		t.Fatal("a new build must get a new id")
	}
	if len(first) != 16 {
		t.Fatalf("the id stays short: %q", first)
	}
}

func serve(t *testing.T, name string) *httptest.ResponseRecorder {
	t.Helper()
	files := fstest.MapFS{
		"index.html":                  {Data: []byte("<html>")},
		"vendor/pdfcraft/host.html":   {Data: []byte("<html>host")},
		"vendor/pdfcraft/app.js":      {Data: []byte("code")},
		"vendor/pdfcraft/app.wasm.gz": {Data: []byte("packed")},
	}
	handler := newHandler(files, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, "", testOrigin)
	rec := httptest.NewRecorder()
	// The response starts with the headers the server sets on everything.
	rec.Header().Set("X-Frame-Options", "DENY")
	rec.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, name, nil))
	return rec
}

func TestAViewerRunsInASandboxThatCannotReachTheSession(t *testing.T) {
	rec := serve(t, "/vendor/pdfcraft/host.html")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	policy := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"sandbox allow-scripts allow-downloads", "'wasm-unsafe-eval'", "default-src 'none'", "frame-ancestors 'self'"} {
		if !strings.Contains(policy, want) {
			t.Errorf("the policy lacks %q: %s", want, policy)
		}
	}
	for _, never := range []string{"allow-same-origin", "allow-top-navigation", "allow-popups", "allow-forms", "'unsafe-eval'"} {
		if strings.Contains(strings.ReplaceAll(policy, "'wasm-unsafe-eval'", ""), never) {
			t.Errorf("the policy must not hold %q: %s", never, policy)
		}
	}
	if rec.Header().Get("X-Frame-Options") != "" {
		t.Fatal("our own page must be able to frame a viewer")
	}
}

func TestOnlyTheViewersGetTheLoosePolicy(t *testing.T) {
	policy := serve(t, "/index.html").Header().Get("Content-Security-Policy")
	if strings.Contains(policy, "wasm-unsafe-eval") || strings.Contains(policy, "sandbox") {
		t.Fatalf("the app itself must stay strict: %s", policy)
	}
}

func TestALargeProgramFileIsSentPacked(t *testing.T) {
	rec := serve(t, "/vendor/pdfcraft/app.wasm")
	if rec.Code != http.StatusOK || rec.Body.String() != "packed" {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Content-Type") != "application/wasm" {
		t.Fatalf("headers: %v", rec.Header())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("a sandboxed frame has no origin of its own, so it needs this to load the file")
	}
}

func TestAMissingViewerFileIsNotTheAppPage(t *testing.T) {
	if rec := serve(t, "/vendor/pdfcraft/none.js"); rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	if rec := serve(t, "/vendor/../index.html"); strings.Contains(rec.Header().Get("Content-Security-Policy"), "wasm") {
		t.Fatal("a path that leaves the folder must not get the viewer's policy")
	}
}

// The builds are someone else's code, pinned by hash: a change must be a new release.
func TestTheViewerFilesAreThePinnedOnes(t *testing.T) {
	list, err := os.ReadFile("../../scripts/craft-checksums.sha256")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(list)), "\n")
	if len(lines) < 2 {
		t.Fatalf("too few files: %d", len(lines))
	}
	for _, line := range lines {
		want, name, found := strings.Cut(line, "  ")
		if !found {
			t.Fatalf("bad line: %q", line)
		}
		data, err := os.ReadFile(filepath.Join("../../web/public/vendor", name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			t.Errorf("%s is not the pinned file", name)
		}
	}
}
