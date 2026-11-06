// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

func TestThePoolHasALargerDefaultThatTheAddressCanChange(t *testing.T) {
	tdb := testutil.Database(t)
	joiner := "?"
	if strings.Contains(tdb.URL, "?") {
		joiner = "&"
	}
	for address, want := range map[string]int32{tdb.URL: 20, tdb.URL + joiner + "pool_max_conns=3": 3} {
		pool, err := db.Open(context.Background(), address)
		if err != nil {
			t.Fatal(err)
		}
		got := pool.Config().MaxConns
		pool.Close()
		if got != want {
			t.Errorf("got %d, want %d", got, want)
		}
	}
}
