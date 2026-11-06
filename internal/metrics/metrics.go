// SPDX-License-Identifier: Apache-2.0

// Package metrics counts requests and writes them, with gauges read at scrape
// time, in the Prometheus text format. It is a small package of our own, because
// counters and one histogram do not justify the client library (spec 021 D1).
package metrics

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strconv"
	"sync"
	"time"
)

// ContentType is the media type of the text format.
const ContentType = "text/plain; version=0.0.4; charset=utf-8"

// durationBuckets are the upper bounds of the request histogram, in seconds.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// knownMethods bounds the method label, since a client can send any word.
var knownMethods = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}

// Gauge is one number read at scrape time.
type Gauge struct {
	Name  string
	Help  string
	Value float64
}

type requestKey struct {
	method      string
	statusClass string
}

// Requests counts HTTP requests. It is safe for concurrent use.
type Requests struct {
	mu           sync.Mutex
	counts       map[requestKey]uint64
	bucketCounts []uint64
	seconds      float64
	total        uint64
}

// NewRequests returns empty counters.
func NewRequests() *Requests {
	return &Requests{counts: map[requestKey]uint64{}, bucketCounts: make([]uint64, len(durationBuckets))}
}

// Observe records one finished request.
func (r *Requests) Observe(method string, status int, took time.Duration) {
	key := requestKey{method: methodLabel(method), statusClass: statusClassLabel(status)}
	seconds := took.Seconds()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[key]++
	r.seconds += seconds
	r.total++
	for i, bound := range durationBuckets {
		if seconds <= bound {
			r.bucketCounts[i]++
		}
	}
}

func methodLabel(method string) string {
	if slices.Contains(knownMethods, method) {
		return method
	}
	return "OTHER"
}

func statusClassLabel(status int) string {
	if status < 100 || status > 599 {
		return "other"
	}
	return strconv.Itoa(status/100) + "xx"
}

// Write writes the counters and the gauges in the text format.
func (r *Requests) Write(w io.Writer, gauges []Gauge) error {
	r.mu.Lock()
	text := r.requestText()
	r.mu.Unlock()
	if _, err := io.WriteString(w, text); err != nil {
		return err
	}
	for _, g := range gauges {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %s\n", g.Name, g.Help, g.Name, g.Name, number(g.Value)); err != nil {
			return err
		}
	}
	return nil
}

// requestText needs the lock held.
func (r *Requests) requestText() string {
	keys := make([]requestKey, 0, len(r.counts))
	for k := range r.counts {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b requestKey) int {
		return cmp.Or(cmp.Compare(a.method, b.method), cmp.Compare(a.statusClass, b.statusClass))
	})
	text := "# HELP diffluence_http_requests_total HTTP requests, by method and class of status.\n" +
		"# TYPE diffluence_http_requests_total counter\n"
	for _, k := range keys {
		text += fmt.Sprintf("diffluence_http_requests_total{method=%q,status=%q} %d\n", k.method, k.statusClass, r.counts[k])
	}
	text += "# HELP diffluence_http_request_duration_seconds How long requests took.\n" +
		"# TYPE diffluence_http_request_duration_seconds histogram\n"
	for i, bound := range durationBuckets {
		text += fmt.Sprintf("diffluence_http_request_duration_seconds_bucket{le=%q} %d\n", number(bound), r.bucketCounts[i])
	}
	text += fmt.Sprintf("diffluence_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", r.total)
	text += fmt.Sprintf("diffluence_http_request_duration_seconds_sum %s\n", number(r.seconds))
	text += fmt.Sprintf("diffluence_http_request_duration_seconds_count %d\n", r.total)
	return text
}

func number(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
