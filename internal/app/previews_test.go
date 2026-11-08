// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAnOfficePreviewIsMadeOncePerVersionAndDroppedWhenUnused(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner, _, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	report := owner.file(page, "report.docx", "word content")
	copyOf := owner.file(page, "copy.docx", "word content")
	var calls atomic.Int32
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("%PDF-made"))
	}))
	defer helper.Close()
	e.app.Exchange().SetPDFHelper(helper.URL)
	defer e.app.Exchange().SetPDFHelper("")
	get := func(id string) {
		t.Helper()
		resp, body := e.do(viewer.c, http.MethodGet, "/api/v1/attachments/"+id+"/preview.pdf", "")
		if resp.StatusCode != 200 || body != "%PDF-made" {
			t.Fatalf("preview: %d %q", resp.StatusCode, body)
		}
	}

	get(report)
	get(report)
	get(copyOf)
	if n := calls.Load(); n != 1 {
		t.Fatalf("the same content must be converted once, the helper was asked %d times", n)
	}

	// A PDF whose source no file has any more goes on the next purge, and one in use stays.
	gone := sha256.Sum256([]byte("a file that was removed"))
	write := func(pdf io.Reader) error { _, err := io.Copy(io.Discard, pdf); return err }
	if err := e.app.Exchange().OfficePreview(ctx, hex.EncodeToString(gone[:]), "old.docx", strings.NewReader("x"), write); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("a new source is converted: %d", n)
	}
	if err := e.app.Exchange().PurgePreviews(ctx); err != nil {
		t.Fatal(err)
	}
	get(report)
	if n := calls.Load(); n != 2 {
		t.Fatalf("a PDF in use must stay after a purge: %d", n)
	}
	if err := e.app.Exchange().OfficePreview(ctx, hex.EncodeToString(gone[:]), "old.docx", strings.NewReader("x"), write); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 3 {
		t.Fatalf("a PDF nothing used must be gone after a purge: %d", n)
	}
}
