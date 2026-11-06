// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"strings"
	"testing"
	"time"
)

func text(t *testing.T, r *Requests, gauges ...Gauge) string {
	t.Helper()
	var b strings.Builder
	if err := r.Write(&b, gauges); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestBucketsAreCumulativeAndEndInInf(t *testing.T) {
	r := NewRequests()
	r.Observe("GET", 200, 3*time.Millisecond)
	r.Observe("GET", 200, 300*time.Millisecond)
	r.Observe("GET", 200, time.Minute)
	got := text(t, r)
	for _, line := range []string{
		`_bucket{le="0.005"} 1`, `_bucket{le="0.5"} 2`, `_bucket{le="10"} 2`, `_bucket{le="+Inf"} 3`,
		`diffluence_http_request_duration_seconds_count 3`,
	} {
		if !strings.Contains(got, line) {
			t.Errorf("missing %s in:\n%s", line, got)
		}
	}
}

func TestAMethodNobodyKnowsCannotGrowTheLabels(t *testing.T) {
	r := NewRequests()
	r.Observe("BREW", 405, time.Millisecond)
	r.Observe("KICK", 405, time.Millisecond)
	got := text(t, r)
	if !strings.Contains(got, `{method="OTHER",status="4xx"} 2`) || strings.Contains(got, "BREW") {
		t.Fatal(got)
	}
}

func TestAStatusOutsideTheRangeIsOther(t *testing.T) {
	r := NewRequests()
	r.Observe("GET", 0, time.Millisecond)
	if got := text(t, r); !strings.Contains(got, `status="other"`) {
		t.Fatal(got)
	}
}

func TestGaugesAreWrittenWithTheirType(t *testing.T) {
	got := text(t, NewRequests(), Gauge{Name: "diffluence_x", Help: "An x.", Value: 2.5})
	if !strings.Contains(got, "# TYPE diffluence_x gauge\ndiffluence_x 2.5\n") {
		t.Fatal(got)
	}
}
