// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/permissions"
)

// workClass is what a request asks of the database. Under too much load the classes are
// served in this order: saving first, then reading, then search and reports (ARC-233).
type workClass int

const (
	classSave workClass = iota
	classRead
	classHeavy
)

// busyMessage is what a refused request says. Text being written stays in the browser,
// so the person loses nothing by trying again.
const busyMessage = "Diffluence is very busy right now. Try again in a moment."

// heavyPrefixes are reads that cost far more than a page view. People can try them again.
var heavyPrefixes = []string{
	"/api/v1/search",
	"/api/v1/admin/insights",
	"/api/v1/admin/analytics",
	"/api/v1/admin/care",
	"/api/v1/admin/audit",
}

// classOf sorts a request into its class.
func classOf(r *http.Request) workClass {
	if !isRead(r) {
		return classSave
	}
	if strings.Contains(r.URL.Path, "/export") || strings.Contains(r.URL.Path, "/insights") || strings.Contains(r.URL.Path, "/analytics") {
		return classHeavy
	}
	for _, prefix := range heavyPrefixes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return classHeavy
		}
	}
	return classRead
}

// gate is the share of one class. Nearly every handler holds one connection at a time,
// so the requests running at once stand in for the connections they hold.
type gate struct {
	slots      chan struct{}
	waiting    atomic.Int64
	maxWaiting int64
	patience   time.Duration
	retryAfter string
}

func newGate(size, maxWaiting int, patience time.Duration, retryAfter string) *gate {
	return &gate{slots: make(chan struct{}, size), maxWaiting: int64(maxWaiting), patience: patience, retryAfter: retryAfter}
}

func (g *gate) size() int { return cap(g.slots) }

// enter takes a place, waiting at most the patience of the class. Past the waiting
// limit it refuses at once, because a queue without end helps nobody (ARC-234).
func (g *gate) enter(r *http.Request) bool {
	select {
	case g.slots <- struct{}{}:
		return true
	default:
	}
	if g.waiting.Add(1) > g.maxWaiting {
		g.waiting.Add(-1)
		return false
	}
	defer g.waiting.Add(-1)
	timer := time.NewTimer(g.patience)
	defer timer.Stop()
	select {
	case g.slots <- struct{}{}:
		return true
	case <-timer.C:
		return false
	case <-r.Context().Done():
		return false
	}
}

func (g *gate) leave() { <-g.slots }

// admission keeps the order of work for one node.
type admission struct {
	gates [3]*gate
}

// newAdmission divides a pool of the given size. Saving may use it all; reading leaves a
// quarter for saving; search and reports use a quarter, counted inside reading as well.
func newAdmission(poolSize int) *admission {
	quarter := max(poolSize/4, 1)
	read := max(poolSize-quarter, 1)
	return &admission{gates: [3]*gate{
		classSave:  newGate(max(poolSize, 1), 8*max(poolSize, 1), 15*time.Second, "5"),
		classRead:  newGate(read, 4*max(poolSize, 1), 3*time.Second, "2"),
		classHeavy: newGate(min(quarter, read), 2*quarter, time.Second, "2"),
	}}
}

// unqueued are probes and metrics, because a probe that waits on a full node reports too
// late, and the editing socket, which holds no connection and stays open as long as its
// editor: in a queue it would keep a place of reading for hours (it has its own cap).
var unqueued = []string{"/livez", "/readyz", "/metrics", collabSocketPath}

func (a *admission) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, path := range unqueued {
			if r.URL.Path == path {
				next.ServeHTTP(w, r)
				return
			}
		}
		class := classOf(r)
		// A heavy read also holds a place among the reads, so together they never reach
		// the reserve of saving.
		taken := []*gate{a.gates[class]}
		if class == classHeavy {
			taken = append(taken, a.gates[classRead])
		}
		for i, g := range taken {
			if !g.enter(r) {
				for _, held := range taken[:i] {
					held.leave()
				}
				w.Header().Set("Retry-After", g.retryAfter)
				writeError(w, http.StatusServiceUnavailable, "busy", busyMessage)
				return
			}
		}
		defer func() {
			for _, g := range taken {
				g.leave()
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// acquireLimit is the longest a request waits for a database connection. A request
// that got through admission normally finds one at once; one that waits this long
// meets a pool held by work that is stuck, and is better told "busy" (spec 050).
const acquireLimit = 10 * time.Second

// withAcquireLimit bounds every wait of a request for a database connection.
func withAcquireLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(db.WithAcquireLimit(r.Context(), acquireLimit)))
	})
}

// queryLimit is the longest one query of a read may run. A read normally needs
// milliseconds; one that runs this long holds a connection and a core of the database
// that everyone else needs, and is better told "busy" (spec 061). Writes keep running,
// so a change is never cut off halfway by this limit.
const queryLimit = 15 * time.Second

// withQueryLimit bounds every query of a read.
func withQueryLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isRead(r) {
			r = r.WithContext(db.WithQueryLimit(r.Context(), queryLimit))
		}
		next.ServeHTTP(w, r)
	})
}

// withPermissionMemo lets a read load the facts of a person and a space once (ARC-197).
// A write gets none: a grant it makes must count for the checks after it.
func withPermissionMemo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isRead(r) {
			r = r.WithContext(permissions.WithMemo(r.Context()))
		}
		next.ServeHTTP(w, r)
	})
}

// lagLists are reads that list what others change all the time, where a moment's lag is
// fine. Notifications are left out: a person who marks one read must see it read.
var lagLists = []string{"/api/v1/feed", "/api/v1/people", "/api/v1/me/recent"}

// withReadFreshness marks the reads that may come from a copy that lags (ARC-228):
// search, reports, analytics, exports and the lists above. Permission facts and
// sessions mark themselves as current inside it, so a lagging copy can never decide
// access. Until a standby exists, this only counts.
func withReadFreshness(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mayLag(r) {
			r = r.WithContext(db.WithMayLag(r.Context()))
		}
		next.ServeHTTP(w, r)
	})
}

func mayLag(r *http.Request) bool {
	if !isRead(r) {
		return false
	}
	if classOf(r) == classHeavy {
		return true
	}
	// Exact paths only: a profile below /people may be the one a person just saved.
	return slices.Contains(lagLists, r.URL.Path)
}

// overloaded says whether saving or reading has more than half its limit waiting, so
// /readyz can tell a load balancer to send people elsewhere (ARC-235).
func (a *admission) overloaded() bool {
	for _, class := range []workClass{classSave, classRead} {
		g := a.gates[class]
		if g.waiting.Load() > g.maxWaiting/2 {
			return true
		}
	}
	return false
}
