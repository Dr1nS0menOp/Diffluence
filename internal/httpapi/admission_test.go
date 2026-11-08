// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/permissions"
)

// hold is a handler that keeps its place until released, so a test can fill a class.
type hold struct {
	in      chan struct{}
	release chan struct{}
}

func newHold() *hold { return &hold{in: make(chan struct{}, 100), release: make(chan struct{})} }

func (h *hold) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.in <- struct{}{}
	<-h.release
	w.WriteHeader(http.StatusOK)
}

func fill(t *testing.T, handler http.Handler, method, path string, n int, in chan struct{}) *sync.WaitGroup {
	t.Helper()
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, path, nil))
		}()
		<-in
	}
	return &wg
}

func answer(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// refusedWithin says whether a request is refused within d. A request that gets in waits on
// the held handler, so the test reports it instead of hanging.
func refusedWithin(handler http.Handler, method, path string, d time.Duration) bool {
	done := make(chan int, 1)
	go func() { done <- answer(handler, method, path).Code }()
	select {
	case code := <-done:
		return code == http.StatusServiceUnavailable
	case <-time.After(d):
		return false
	}
}

func quickAdmission(pool int) *admission {
	a := newAdmission(pool)
	for _, g := range a.gates {
		g.patience = 50 * time.Millisecond
	}
	return a
}

func TestASaveFindsRoomWhileReadingIsFull(t *testing.T) {
	a := quickAdmission(8)
	h := newHold()
	handler := a.middleware(h)
	readers := fill(t, handler, http.MethodGet, "/api/v1/pages/x", a.gates[classRead].size(), h.in)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- answer(handler, http.MethodPost, "/api/v1/pages") }()
	select {
	case <-h.in:
	case <-time.After(time.Second):
		t.Fatal("a save waited behind reading")
	}
	close(h.release)
	if rec := <-done; rec.Code != http.StatusOK {
		t.Fatalf("save answered %d", rec.Code)
	}
	readers.Wait()
}

func TestReadingLeavesAQuarterOfThePoolForSaving(t *testing.T) {
	a := newAdmission(20)
	if got := a.gates[classRead].size(); got != 15 {
		t.Errorf("reading may use %d of 20", got)
	}
	if got := a.gates[classHeavy].size(); got != 5 {
		t.Errorf("search and reports may use %d of 20", got)
	}
	if got := a.gates[classSave].size(); got != 20 {
		t.Errorf("saving may use %d of 20", got)
	}
	tiny := newAdmission(1)
	for class, g := range tiny.gates {
		if g.size() < 1 {
			t.Errorf("class %d has no room in a pool of one", class)
		}
	}
}

func TestAFullClassSaysBusyWithRetryAfter(t *testing.T) {
	a := quickAdmission(8)
	h := newHold()
	handler := a.middleware(h)
	readers := fill(t, handler, http.MethodGet, "/api/v1/pages/x", a.gates[classRead].size(), h.in)

	rec := answer(handler, http.MethodGet, "/api/v1/pages/y")
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("got %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	close(h.release)
	readers.Wait()
}

func TestPastTheWaitingLimitTheAnswerComesAtOnce(t *testing.T) {
	a := newAdmission(8)
	g := a.gates[classRead]
	g.patience = 5 * time.Second
	g.maxWaiting = 0
	h := newHold()
	handler := a.middleware(h)
	readers := fill(t, handler, http.MethodGet, "/api/v1/pages/x", g.size(), h.in)

	start := time.Now()
	rec := answer(handler, http.MethodGet, "/api/v1/pages/y")
	if rec.Code != http.StatusServiceUnavailable || time.Since(start) > time.Second {
		t.Fatalf("got %d after %s", rec.Code, time.Since(start))
	}
	close(h.release)
	readers.Wait()
}

func TestSearchNeverTakesTheReserveOfSaving(t *testing.T) {
	a := quickAdmission(8)
	h := newHold()
	handler := a.middleware(h)
	heavy := fill(t, handler, http.MethodGet, "/api/v1/search?q=a", a.gates[classHeavy].size(), h.in)
	defer close(h.release)
	if !refusedWithin(handler, http.MethodGet, "/api/v1/search?q=b", time.Second) {
		t.Fatal("search went past its share")
	}
	readers := fill(t, handler, http.MethodGet, "/api/v1/pages/x", a.gates[classRead].size()-a.gates[classHeavy].size(), h.in)
	if !refusedWithin(handler, http.MethodGet, "/api/v1/pages/y", time.Second) {
		t.Fatal("search and reading together went past reading's share")
	}
	// The held requests end when the deferred release runs.
	_ = fill(t, handler, http.MethodPost, "/api/v1/pages", 8-a.gates[classRead].size(), h.in)
	_, _ = heavy, readers
}

func TestProbesAndMetricsNeverWait(t *testing.T) {
	a := newAdmission(4)
	for _, g := range a.gates {
		for range g.size() {
			g.slots <- struct{}{}
		}
		g.maxWaiting = 0
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, path := range []string{"/livez", "/readyz", "/metrics"} {
		if rec := answer(a.middleware(ok), http.MethodGet, path); rec.Code != http.StatusOK {
			t.Errorf("%s answered %d", path, rec.Code)
		}
	}
}

func TestEachRequestHasTheRightClass(t *testing.T) {
	cases := []struct {
		method, path string
		want         workClass
	}{
		{http.MethodPost, "/api/v1/pages", classSave},
		{http.MethodDelete, "/api/v1/pages/x", classSave},
		{http.MethodPut, "/api/v1/pages/x/watch", classSave},
		{http.MethodGet, "/api/v1/pages/x", classRead},
		{http.MethodGet, "/api/v1/me/notifications/count", classRead},
		{http.MethodGet, "/api/v1/search?q=a", classHeavy},
		{http.MethodGet, "/api/v1/search/quick?q=a", classHeavy},
		{http.MethodGet, "/api/v1/spaces/x/export/markdown", classHeavy},
		{http.MethodGet, "/api/v1/admin/insights", classHeavy},
		{http.MethodGet, "/api/v1/admin/audit", classHeavy},
		{http.MethodPost, "/api/v1/spaces/x/export/pdf", classSave},
	}
	for _, c := range cases {
		if got := classOf(httptest.NewRequest(c.method, c.path, nil)); got != c.want {
			t.Errorf("%s %s: class %d, want %d", c.method, c.path, got, c.want)
		}
	}
}

func TestOnlyAReadReusesPermissionFacts(t *testing.T) {
	var memoized bool
	h := withPermissionMemo(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		memoized = permissions.Memoized(r.Context())
	}))
	for method, want := range map[string]bool{http.MethodGet: true, http.MethodPost: false, http.MethodPut: false, http.MethodDelete: false} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, "/api/v1/pages/x", nil))
		if memoized != want {
			t.Errorf("%s: memo %v, want %v", method, memoized, want)
		}
	}
}

func TestReadinessSaysBusyWhenAQueueIsLong(t *testing.T) {
	a := newAdmission(8)
	if a.overloaded() {
		t.Fatal("an idle node is overloaded")
	}
	g := a.gates[classSave]
	g.waiting.Store(g.maxWaiting/2 + 1)
	if !a.overloaded() {
		t.Fatal("a long queue of saves is not reported")
	}
}
