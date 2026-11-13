// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/tracing"
)

// uuidLike finds an id in a span: a path or a value holds one, a route pattern never does.
var uuidLike = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// Spec 060 D6: with tracing on, the whole application makes spans by route, query, job and
// event, and not one of them holds an email address, a title, page text, a token or an id.
func TestTracesOfTheWholeApplicationHoldNoContent(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	stop, err := tracing.Start(context.Background(), tracing.Options{Enabled: true, Sample: 1, Exporter: exporter})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stop(context.Background()) })

	e := newEnv(t)
	const (
		email = "private.person@example.org"
		title = "Quarterly figures of Rotterdam"
		text  = "the merger with the lighthouse company"
	)
	owner := e.user(email, identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("TRC")
	page := owner.page(space, nil, title, text)
	owner.want(200, "GET", "/api/v1/pages/"+page, nil)
	owner.want(200, "GET", "/api/v1/search?q=lighthouse", nil)
	token := owner.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "script"})["secret"].(string)
	e.bearer("GET", "/api/v1/me", token, "")
	owner.want(404, "GET", "/api/v1/pages/00000000-0000-0000-0000-00000000dead", nil)
	e.runJobs()
	e.deliver()
	if err := e.app.SealAudit(context.Background()); err != nil {
		t.Fatal(err)
	}

	spans := exporter.GetSpans()
	names := map[string]bool{}
	for _, s := range spans {
		names[s.Name] = true
		check := func(where, value string) {
			for _, private := range []string{email, "private.person", title, "Rotterdam", text, "lighthouse", token} {
				if strings.Contains(value, private) {
					t.Errorf("span %q, %s holds %q", s.Name, where, private)
				}
			}
			if uuidLike.MatchString(value) {
				t.Errorf("span %q, %s holds an id: %q", s.Name, where, value)
			}
		}
		check("its name", s.Name)
		check("its status", s.Status.Description)
		for _, kv := range s.Attributes {
			check(string(kv.Key), kv.Value.String())
		}
		for _, ev := range s.Events {
			check("an event "+ev.Name, ev.Name)
			for _, kv := range ev.Attributes {
				check("an event's "+string(kv.Key), kv.Value.String())
			}
		}
	}
	for _, want := range []string{"POST /api/v1/spaces", "GET /api/v1/pages/{id}", "GET /api/v1/search", "INSERT pages", "audit seal"} {
		if !names[want] {
			t.Errorf("no span %q among %d spans", want, len(spans))
		}
	}
}
