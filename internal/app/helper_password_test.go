// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The PDF helper reads hostile files. With a password set it answers only the app (SEC-503).
func TestThePDFHelperGetsItsPasswordAndRefusesWithoutIt(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	report := owner.file(page, "report.docx", "word content")
	const password = "helper-password-for-tests"
	var user, given string
	var sent bool
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, given, sent = r.BasicAuth()
		if !sent || user != "diffluence" || given != password {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("%PDF-fake"))
	}))
	defer helper.Close()
	e.app.Exchange().SetPDFHelper(helper.URL)
	defer e.app.Exchange().SetPDFHelper("")
	address := "/api/v1/attachments/" + report + "/preview.pdf"

	// Without the password the helper refuses, and the person gets a clear answer, not the helper's.
	if resp, _ := e.do(owner.c, http.MethodGet, address, ""); resp.StatusCode != 400 {
		t.Fatalf("without the password: %d", resp.StatusCode)
	}
	if sent {
		t.Fatalf("a password went out while none was set")
	}

	e.app.Exchange().SetPDFPassword(password)
	defer e.app.Exchange().SetPDFPassword("")
	resp, body := e.do(owner.c, http.MethodGet, address, "")
	if resp.StatusCode != 200 || body != "%PDF-fake" {
		t.Fatalf("with the password: %d %q", resp.StatusCode, body)
	}
	if user != "diffluence" || given != password {
		t.Fatalf("the helper got %q", user)
	}
}
