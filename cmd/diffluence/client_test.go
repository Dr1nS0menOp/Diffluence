// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeAPI(t *testing.T) (*httptest.Server, func(string) string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dfl_good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/spaces":
			_, _ = io.WriteString(w, `{"spaces":[{"id":"a1","key":"ENG"}]}`)
		case "/api/v1/spaces/a1/tree":
			_, _ = io.WriteString(w, `{"pages":[]}`)
		case "/api/v1/spaces/a1/export.zip":
			_, _ = io.WriteString(w, "zipped")
		case "/api/v1/spaces/a1/imports":
			if r.URL.Query().Get("filename") != "a.md" {
				t.Errorf("filename %q", r.URL.Query().Get("filename"))
			}
			body, _ := io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"got":"`+string(body)+`"}`)
		case "/api/v1/spaces/broken/export.zip":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	env := func(name string) string {
		switch name {
		case "DIFFLUENCE_URL":
			return srv.URL
		case "DIFFLUENCE_TOKEN":
			return "dfl_good"
		}
		return ""
	}
	return srv, env
}

func clientRun(env func(string) string, args ...string) (int, string, string) {
	var out, errs bytes.Buffer
	code := cmdClient(context.Background(), args, env, &out, &errs)
	return code, out.String(), errs.String()
}

func TestAListGoesToStandardOutput(t *testing.T) {
	_, env := fakeAPI(t)
	if code, out, _ := clientRun(env, "spaces", "list"); code != exitDone || !strings.Contains(out, "ENG") {
		t.Fatalf("%d %s", code, out)
	}
	if code, out, _ := clientRun(env, "pages", "list", "a1"); code != exitDone || !strings.Contains(out, "pages") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestExportAndImportMoveFiles(t *testing.T) {
	_, env := fakeAPI(t)
	dir := t.TempDir()
	zip := filepath.Join(dir, "eng.zip")
	if code, _, errs := clientRun(env, "export", "a1", zip); code != exitDone {
		t.Fatalf("%d %s", code, errs)
	}
	if got, _ := os.ReadFile(zip); string(got) != "zipped" {
		t.Fatalf("got %q", got)
	}
	file := filepath.Join(dir, "a.md")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := clientRun(env, "import", "a1", file); code != exitDone || !strings.Contains(out, "hello") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestABackupHoldsEverySpace(t *testing.T) {
	_, env := fakeAPI(t)
	dir := filepath.Join(t.TempDir(), "backup")
	if code, _, errs := clientRun(env, "backup-content", dir); code != exitDone {
		t.Fatalf("%d %s", code, errs)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "ENG.zip")); string(got) != "zipped" {
		t.Fatalf("got %q", got)
	}
}

func TestAFailedExportLeavesNoFile(t *testing.T) {
	_, env := fakeAPI(t)
	zip := filepath.Join(t.TempDir(), "x.zip")
	if code, _, _ := clientRun(env, "export", "broken", zip); code != exitFailed {
		t.Fatalf("code %d", code)
	}
	if _, err := os.Stat(zip); err == nil {
		t.Fatal("a broken file stayed")
	}
}

func TestExitCodesSayWhatWentWrong(t *testing.T) {
	srv, env := fakeAPI(t)
	if code, _, _ := clientRun(env, "nonsense"); code != exitUsage {
		t.Fatalf("code %d", code)
	}
	if code, _, _ := clientRun(func(string) string { return "" }, "spaces", "list"); code != exitUsage {
		t.Fatalf("code %d", code)
	}
	bad := func(name string) string {
		if name == "DIFFLUENCE_URL" {
			return srv.URL
		}
		if name == "DIFFLUENCE_TOKEN" {
			return "dfl_bad"
		}
		return ""
	}
	if code, _, _ := clientRun(bad, "spaces", "list"); code != exitRefused {
		t.Fatalf("code %d", code)
	}
}

func TestTheTokenCanComeFromAFile(t *testing.T) {
	_, env := fakeAPI(t)
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte("dfl_good\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fromFile := func(name string) string {
		switch name {
		case "DIFFLUENCE_TOKEN":
			return ""
		case "DIFFLUENCE_TOKEN_FILE":
			return file
		}
		return env(name)
	}
	if code, _, errs := clientRun(fromFile, "spaces", "list"); code != exitDone {
		t.Fatalf("%d %s", code, errs)
	}
}
