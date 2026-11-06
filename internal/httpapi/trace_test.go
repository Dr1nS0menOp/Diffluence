// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const validTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

func TestTraceIDOnlyComesFromAValidHeader(t *testing.T) {
	for header, want := range map[string]string{
		"00-" + validTraceID + "-00f067aa0ba902b7-01":       validTraceID,
		"01-" + validTraceID + "-00f067aa0ba902b7-01-extra": validTraceID,
		"":         "",
		"nonsense": "",
		"ff-" + validTraceID + "-00f067aa0ba902b7-01":                  "",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01":      "",
		"00-" + validTraceID + "-0000000000000000-01":                  "",
		"00-" + strings.ToUpper(validTraceID) + "-00f067aa0ba902b7-01": "",
		"00-" + validTraceID + "-00f067aa0ba902b7-01-extra":            "",
		"00-" + validTraceID[:31] + "\n-00f067aa0ba902b7-01":           "",
	} {
		if got := traceIDFrom(header); got != want {
			t.Errorf("%q: got %q", header, got)
		}
	}
}

func TestTheLogNamesTheTraceOfARequest(t *testing.T) {
	var logged bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logged, nil))
	handler := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), withRequestID, withLogAndRecover(log))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("traceparent", "00-"+validTraceID+"-00f067aa0ba902b7-01")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if !strings.Contains(logged.String(), `"trace_id":"`+validTraceID+`"`) {
		t.Fatal(logged.String())
	}
}
