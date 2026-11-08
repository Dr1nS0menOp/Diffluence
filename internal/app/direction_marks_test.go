// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"strings"
	"testing"
)

// A right-to-left override can make a name read as something else (SEC-319).
func TestNamesLoseTheMarksThatReverseReading(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "Invoice\u202efdp.exe", "x")
	got := owner.ok("GET", "/api/v1/pages/"+page, nil)
	if title, _ := got["title"].(string); title != "Invoicefdp.exe" {
		t.Fatalf("title: %q", title)
	}
	file := owner.file(page, "report\u202efdp.exe", "words")
	resp, _ := e.do(owner.c, "GET", "/api/v1/attachments/"+file+"/content", "")
	if got := resp.Header.Get("Content-Disposition"); strings.ContainsRune(got, '\u202e') || !strings.Contains(got, "reportfdp.exe") {
		t.Fatalf("file name: %q", got)
	}
}
