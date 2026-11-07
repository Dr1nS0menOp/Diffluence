// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPastesWaitForAFreeSlot(t *testing.T) {
	s := &Server{markdownBusy: make(chan struct{}, 1)}
	send := func() int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/markdown/document", strings.NewReader(`{"markdown":"# Hi"}`))
		r.Header.Set("Content-Type", "application/json")
		s.handleMarkdownDocument(w, r)
		return w.Code
	}
	s.markdownBusy <- struct{}{}
	if got := send(); got != http.StatusTooManyRequests {
		t.Fatalf("with every slot taken: %d", got)
	}
	<-s.markdownBusy
	if got := send(); got != http.StatusOK {
		t.Fatalf("with a free slot: %d", got)
	}
	if len(s.markdownBusy) != 0 {
		t.Fatal("the slot was not given back")
	}
}
