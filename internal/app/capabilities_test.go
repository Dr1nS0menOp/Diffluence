// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"net/http"
	"testing"
)

func TestCapabilitiesSayWhatIsOnAndNothingMore(t *testing.T) {
	e := newEnv(t)
	owner, _, _, _ := e.team("ENG")
	if resp, _ := e.do(http.DefaultClient, http.MethodGet, "/api/v1/system/capabilities", ""); resp.StatusCode != 401 {
		t.Fatalf("a stranger: %d", resp.StatusCode)
	}
	got := owner.ok("GET", "/api/v1/system/capabilities", nil)
	for _, part := range []string{"pdf_export", "office_preview", "drawing", "collaboration"} {
		if got[part] != false {
			t.Fatalf("%s is not set up, so it must say false: %v", part, got)
		}
	}
	if len(got) != 4 {
		t.Fatalf("only whether a part is on, never where it runs: %v", got)
	}
	e.app.Exchange().SetPDFHelper("http://helper.invalid")
	defer e.app.Exchange().SetPDFHelper("")
	got = owner.ok("GET", "/api/v1/system/capabilities", nil)
	if got["pdf_export"] != true || got["office_preview"] != true {
		t.Fatalf("with a PDF helper: %v", got)
	}
}
