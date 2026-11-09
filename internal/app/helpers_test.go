// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func (e *env) runTask(name string) {
	e.t.Helper()
	for _, task := range e.app.PeriodicTasks() {
		if task.Name == name {
			if err := task.Run(context.Background()); err != nil {
				e.t.Fatalf("%s: %v", name, err)
			}
			return
		}
	}
	e.t.Fatalf("no periodic task %s", name)
}

// A helper that stops answering is shown to administrators and is no longer offered to
// anyone, and a wiki without it still says it is ready (ARC-071, ARC-072).
func TestAHelperThatDoesNotAnswerIsShownAndNoLongerOffered(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	var healthy atomic.Bool
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" && healthy.Load() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer helper.Close()
	e.app.Exchange().SetPDFHelper(helper.URL)
	defer e.app.Exchange().SetPDFHelper("")

	e.runTask("check-helpers")
	if got := root.ok("GET", "/api/v1/system/capabilities", nil); got["pdf_export"] != false || got["office_preview"] != false {
		t.Fatalf("a helper that does not answer must not be offered: %v", got)
	}
	if out := root.want(200, "GET", "/api/v1/admin/attention", nil); !strings.Contains(out, "helpers_down") {
		t.Fatalf("administrators must be told: %s", out)
	}
	system := root.want(200, "GET", "/api/v1/admin/system", nil)
	if !strings.Contains(system, `"name":"pdf"`) || !strings.Contains(system, `"ok":false`) {
		t.Fatalf("the system screen must show the helper: %s", system)
	}
	if strings.Contains(system, helper.URL) {
		t.Fatalf("the address of a helper must never be shown: %s", system)
	}
	if resp, _ := e.do(http.DefaultClient, http.MethodGet, "/readyz", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("a wiki without its PDF helper is still ready: %d", resp.StatusCode)
	}

	healthy.Store(true)
	e.runTask("check-helpers")
	if got := root.ok("GET", "/api/v1/system/capabilities", nil); got["pdf_export"] != true || got["office_preview"] != true {
		t.Fatalf("a helper that answers again is offered again: %v", got)
	}
	if out := root.want(200, "GET", "/api/v1/admin/attention", nil); strings.Contains(out, "helpers_down") {
		t.Fatalf("nothing to warn about: %s", out)
	}

	// A helper that is switched off is forgotten, not reported as broken.
	e.app.Exchange().SetPDFHelper("")
	e.runTask("check-helpers")
	if system := root.want(200, "GET", "/api/v1/admin/system", nil); strings.Contains(system, `"name":"pdf"`) {
		t.Fatalf("a helper that is off is not a problem: %s", system)
	}
}
