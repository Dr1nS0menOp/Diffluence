// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/diffluence/diffluence/internal/metrics"
)

// sameSecret compares in constant time. Hashing first hides the length as well.
func sameSecret(given, want string) bool {
	givenSum, wantSum := sha256.Sum256([]byte(given)), sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(givenSum[:], wantSum[:]) == 1
}

// handleMetrics shows the numbers of this node to a scraper with the token. Without
// a configured token the route does not exist (spec 021 D2).
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	configured := s.deps.Config.MetricsToken
	if configured == "" {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found || !sameSecret(token, configured) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthenticated", "This needs the metrics token.")
		return
	}
	w.Header().Set("Content-Type", metrics.ContentType)
	w.Header().Set("Cache-Control", "no-store")
	if err := s.requests.Write(w, s.deps.Gauges(r.Context())); err != nil {
		s.deps.Log.Warn("metrics: writing the answer failed", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
}
