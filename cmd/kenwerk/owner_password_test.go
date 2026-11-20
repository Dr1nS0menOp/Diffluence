// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// The owner's password reaches psql through its environment, never its command line, which
// every process of the container can read (spec 072).
func TestTheOwnersPasswordIsTakenOutOfTheAddress(t *testing.T) {
	cases := []struct{ in, url, password string }{
		{"postgres://owner:s3cret@db:5432/kenwerk?sslmode=disable", "postgres://owner@db:5432/kenwerk?sslmode=disable", "s3cret"},
		{"postgresql://owner:p%40ss%2Fw%25rd@db/kenwerk", "postgresql://owner@db/kenwerk", "p@ss/w%rd"},
		{"postgres://owner@db/kenwerk", "postgres://owner@db/kenwerk", ""},
		{"host=db user=owner dbname=kenwerk", "host=db user=owner dbname=kenwerk", ""},
	}
	for _, c := range cases {
		url, password := passwordApart(c.in)
		if url != c.url || password != c.password {
			t.Errorf("%q: got %q and %q, want %q and %q", c.in, url, password, c.url, c.password)
		}
	}
}
