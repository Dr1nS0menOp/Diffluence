// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// The owner's password reaches psql through its environment, never its command line, which
// every process of the container can read (spec 072).
func TestTheOwnersPasswordIsTakenOutOfTheAddress(t *testing.T) {
	cases := []struct{ in, url, password string }{
		{"postgres://owner:s3cret@db:5432/diffluence?sslmode=disable", "postgres://owner@db:5432/diffluence?sslmode=disable", "s3cret"},
		{"postgresql://owner:p%40ss%2Fw%25rd@db/diffluence", "postgresql://owner@db/diffluence", "p@ss/w%rd"},
		{"postgres://owner@db/diffluence", "postgres://owner@db/diffluence", ""},
		{"host=db user=owner dbname=diffluence", "host=db user=owner dbname=diffluence", ""},
	}
	for _, c := range cases {
		url, password := passwordApart(c.in)
		if url != c.url || password != c.password {
			t.Errorf("%q: got %q and %q, want %q and %q", c.in, url, password, c.url, c.password)
		}
	}
}
