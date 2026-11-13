// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/metrics"
)

// An editing socket stays open as long as its editor; it must not hold a place of reading.
func TestOpenEditorSocketsLeaveReadingFree(t *testing.T) {
	a := quickAdmission(20)
	h := newHold()
	route := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == collabSocketPath {
			h.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := a.middleware(route)
	var sockets sync.WaitGroup
	defer func() {
		close(h.release)
		sockets.Wait()
	}()
	for i := range 50 {
		sockets.Add(1)
		go func() {
			defer sockets.Done()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, collabSocketPath, nil))
		}()
		select {
		case <-h.in:
		case <-time.After(2 * time.Second):
			t.Fatalf("editing socket %d was held back by admission", i+1)
		}
	}

	done := make(chan int, 1)
	go func() { done <- answer(handler, http.MethodGet, "/api/v1/pages/x").Code }()
	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Fatalf("a page read with 50 editors open answered %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a page read waited behind open editors")
	}
}

// A socket is not a request that took hours: it must not reach the request counters.
func TestEditorSocketsAreNotTimedAsRequests(t *testing.T) {
	requests := metrics.NewRequests()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	withMetrics(requests)(ok).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, collabSocketPath, nil))
	var out bytes.Buffer
	if err := requests.Write(&out, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), `method="GET"`) {
		t.Fatalf("an editing socket was counted as a request:\n%s", out.String())
	}
	withMetrics(requests)(ok).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/pages/x", nil))
	out.Reset()
	if err := requests.Write(&out, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `method="GET"`) {
		t.Fatal("an ordinary read was not counted")
	}
}

func TestOnePersonCannotTakeEverySocketOfANode(t *testing.T) {
	c := newSocketCap(5, 2)
	// Each acquire takes a place, so a key that repeats asks for one more each time.
	for _, key := range []string{"a", "a"} {
		if !c.acquire(key) {
			t.Fatal("a person could not open two editors")
		}
	}
	if c.acquire("a") {
		t.Fatal("a person went past the sockets of one person")
	}
	for _, key := range []string{"b", "b", "c"} {
		if !c.acquire(key) {
			t.Fatalf("%s was refused while the node had room", key)
		}
	}
	if c.acquire("d") {
		t.Fatal("the node went past its sockets")
	}
	c.release("a")
	if !c.acquire("d") {
		t.Fatal("a closed socket did not free its place")
	}
	c.release("b")
	c.release("b")
	if len(c.perKey) != 3 {
		t.Fatalf("closed sockets left %d keys behind", len(c.perKey))
	}
}
