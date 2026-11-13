// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
)

// The check from the console walks the whole chain in one query, so the limit on one query
// of a read, which suits a page view, would end it on a large log for ever. It gets the
// limit of a walk instead.
func TestTheConsoleCheckOfTheAuditLogIsNotCutAtTheLimitOfARead(t *testing.T) {
	var limit = db.QueryLimitOf(context.Background())
	s := &Server{deps: Deps{Log: slog.New(slog.DiscardHandler), CheckAudit: func(ctx context.Context) (audit.VerifyResult, error) {
		limit = db.QueryLimitOf(ctx)
		return audit.VerifyResult{}, errors.New("stop here")
	}}}
	handler := withQueryLimit(http.HandlerFunc(s.handleAuditVerify))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-events/verify", nil))
	if limit != auditWalkLimit || limit <= queryLimit {
		t.Fatalf("the check ran with a query limit of %s", limit)
	}
}
