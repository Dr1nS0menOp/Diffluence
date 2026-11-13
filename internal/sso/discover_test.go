// SPDX-License-Identifier: Apache-2.0

package sso

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// silentProvider accepts every connection and never answers, until the test ends.
func silentProvider(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-stop:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(stop)
		srv.Close()
	})
	return srv, &hits
}

func TestAProviderThatNeverAnswersDoesNotHoldLoginsForEver(t *testing.T) {
	srv, _ := silentProvider(t)
	p := New(Config{Issuer: srv.URL, ClientID: "c"}, nil, nil)
	p.discoveryTimeout = 200 * time.Millisecond

	// The login's own context never ends here, as a person may wait: only the timeout may
	// end the wait, and the test stops looking after two seconds instead of hanging.
	done := make(chan error, 1)
	go func() {
		_, _, err := p.discover(context.Background())
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("discovery of a silent provider succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("discovery still waits for a provider that never answers")
	}
}

func TestOneSlowDiscoveryDoesNotQueueEveryLoginBehindIt(t *testing.T) {
	srv, hits := silentProvider(t)
	p := New(Config{Issuer: srv.URL, ClientID: "c"}, nil, nil)
	p.discoveryTimeout = 300 * time.Millisecond

	// Ten logins at once ask the provider once, and all are answered within one timeout.
	var wg sync.WaitGroup
	start := time.Now()
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = p.discover(context.Background())
		}()
	}
	wg.Wait()
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("ten logins took %s together: they waited one after the other", took)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("ten logins asked the provider %d times", n)
	}
}

func TestAFailedDiscoveryIsRememberedForAWhile(t *testing.T) {
	srv, hits := silentProvider(t)
	p := New(Config{Issuer: srv.URL, ClientID: "c"}, nil, nil)
	p.discoveryTimeout = 100 * time.Millisecond
	now := time.Now()
	p.now = func() time.Time { return now }

	if _, _, err := p.discover(context.Background()); err == nil {
		t.Fatal("discovery of a silent provider succeeded")
	}
	start := time.Now()
	if _, _, err := p.discover(context.Background()); err == nil {
		t.Fatal("a remembered failure succeeded")
	}
	if took := time.Since(start); took > 50*time.Millisecond {
		t.Fatalf("a remembered failure still waited %s", took)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("the provider was asked %d times within the pause", n)
	}
	now = now.Add(failurePause + time.Second)
	_, _, _ = p.discover(context.Background())
	if n := hits.Load(); n != 2 {
		t.Fatalf("after the pause the provider was asked %d times, want 2", n)
	}
}
