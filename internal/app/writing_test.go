// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

func TestTypingHelpIsOnUntilSwitchedOff(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	prefs := anna.ok("GET", "/api/v1/me/preferences", nil)
	if prefs["typing_help"] != true {
		t.Fatalf("defaults: %v", prefs)
	}
	prefs["typing_help"] = false
	anna.ok("PUT", "/api/v1/me/preferences", prefs)
	if anna.ok("GET", "/api/v1/me/preferences", nil)["typing_help"] != false {
		t.Fatal("the switch did not stay off")
	}
}

func TestPastedMarkdownBecomesACheckedDocument(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	got := mustJSON(t, anna.ok("POST", "/api/v1/markdown/document", map[string]any{
		"markdown": "# Plan\n\n- one\n- two\n\n[bad](javascript:evil())\n\n<script>alert(1)</script>\n"}))
	for _, want := range []string{`"heading"`, `"bulletList"`, "Plan"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%s is missing: %s", want, got)
		}
	}
	for _, refused := range []string{"javascript:", "<script"} {
		if strings.Contains(got, refused) {
			t.Fatalf("%s got through: %s", refused, got)
		}
	}
}

func TestPastedMarkdownHasALimit(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	anna.ok("POST", "/api/v1/markdown/document", map[string]any{"markdown": strings.Repeat("long text ", 20_000)})
	anna.want(400, "POST", "/api/v1/markdown/document", map[string]any{"markdown": strings.Repeat("x", 1<<20+1)})
}

func TestAHiddenPageAndADeletedPageLookTheSameToAReader(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	open := owner.page(space, nil, "Open", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	deleted := owner.page(space, nil, "Deleted", "x")
	owner.restrict(hidden, "view", owner.id)
	owner.ok("DELETE", "/api/v1/pages/"+deleted, nil)
	never := "00000000-0000-7000-8000-000000000001"

	ask := map[string]any{"ids": []string{open, hidden, deleted, never}}
	pages := reader.ok("POST", "/api/v1/pages/reachable", ask)["pages"].([]any)
	if len(pages) != 1 || pages[0] != open {
		t.Fatalf("a reader may only hear of the open page: %v", pages)
	}
	if got := stranger.ok("POST", "/api/v1/pages/reachable", ask)["pages"].([]any); len(got) != 0 {
		t.Fatalf("someone outside the space heard of a page: %v", got)
	}
	if got := owner.ok("POST", "/api/v1/pages/reachable", ask)["pages"].([]any); len(got) != 2 {
		t.Fatalf("the owner reaches the hidden page too: %v", got)
	}
}

func TestAskingAboutTooManyPagesIsRefused(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	ids := make([]string, 501)
	for i := range ids {
		ids[i] = "00000000-0000-7000-8000-000000000001"
	}
	anna.want(400, "POST", "/api/v1/pages/reachable", map[string]any{"ids": ids})
}
