// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// closedPort returns a port on this machine where nothing listens.
func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func warningFor(warnings []Warning, setting string) (Warning, bool) {
	for _, w := range warnings {
		if w.Setting == setting {
			return w, true
		}
	}
	return Warning{}, false
}

func TestNothingConfiguredIsNothingToTry(t *testing.T) {
	if w := Probe(context.Background(), Config{}); len(w) != 0 {
		t.Errorf("warnings for an installation without mail, login or sites: %v", w)
	}
}

func TestAMailServerThatDoesNotAnswerWarns(t *testing.T) {
	c := Config{SMTP: SMTP{Host: "127.0.0.1", Port: closedPort(t)}}
	w, ok := warningFor(Probe(context.Background(), c), "SMTP_HOST")
	if !ok || !strings.Contains(w.Message, "port "+strconv.Itoa(c.SMTP.Port)) {
		t.Fatalf("want a warning about the port, got %+v", w)
	}
}

func TestAMailServerThatAnswersDoesNotWarn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	c := Config{SMTP: SMTP{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port}}
	if w, ok := warningFor(Probe(context.Background(), c), "SMTP_HOST"); ok {
		t.Errorf("warning for a mail server that answers: %+v", w)
	}
}

func TestAMailServerWhoseNameDoesNotResolveWarns(t *testing.T) {
	// .invalid never resolves (RFC 6761).
	c := Config{SMTP: SMTP{Host: "mail.diffluence.invalid", Port: 587}}
	w, ok := warningFor(Probe(context.Background(), c), "SMTP_HOST")
	if !ok || !strings.Contains(w.Message, "resolve") {
		t.Fatalf("want a warning that the name does not resolve, got %+v", w)
	}
}

func TestAnIdentityProviderWithoutADiscoveryDocumentWarns(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	w, ok := warningFor(Probe(context.Background(), Config{OIDC: OIDC{Issuer: srv.URL}}), "OIDC_ISSUER")
	if !ok || !strings.Contains(w.Message, "404") {
		t.Fatalf("want a warning naming the status, got %+v", w)
	}
}

func TestADiscoveryDocumentOfAnotherIssuerWarns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"https://someone-else.example.com"}`))
	}))
	defer srv.Close()
	if _, ok := warningFor(Probe(context.Background(), Config{OIDC: OIDC{Issuer: srv.URL}}), "OIDC_ISSUER"); !ok {
		t.Fatal("want a warning for a document naming another issuer")
	}
}

func TestAnIdentityProviderThatAnswersDoesNotWarn(t *testing.T) {
	var issuer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"issuer":"` + issuer + `"}`))
	}))
	defer srv.Close()
	issuer = srv.URL
	if w, ok := warningFor(Probe(context.Background(), Config{OIDC: OIDC{Issuer: issuer}}), "OIDC_ISSUER"); ok {
		t.Errorf("warning for a provider that answers: %+v", w)
	}
}

func TestAPublishFolderThatCannotBeWrittenWarns(t *testing.T) {
	notAFolder := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notAFolder, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := warningFor(Probe(context.Background(), Config{PublishDir: notAFolder}), "PUBLISH_DIR"); !ok {
		t.Fatal("want a warning for a publish folder that cannot be written")
	}
}

func TestTryingThePublishFolderLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	if w, ok := warningFor(Probe(context.Background(), Config{PublishDir: dir}), "PUBLISH_DIR"); ok {
		t.Fatalf("warning for a folder that can be written: %+v", w)
	}
	left, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("the probe left %d file(s) in the publish folder", len(left))
	}
}

// A probe that hangs must not hold up the start.
func TestAProbeGivesUpInTime(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	start := time.Now()
	w, ok := warningFor(ProbeWithin(context.Background(), Config{OIDC: OIDC{Issuer: srv.URL}}, 300*time.Millisecond), "OIDC_ISSUER")
	if !ok {
		t.Fatal("want a warning for a provider that does not answer in time")
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("the probe took %v", took)
	}
	if strings.Contains(w.Message, srv.URL) {
		t.Errorf("the warning names the address: %q", w.Message)
	}
}
