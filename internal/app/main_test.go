// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"os"
	"testing"

	"github.com/kenwerk/kenwerk/internal/testutil"
)

// TestMain migrates one database and copies it for every test, since migrating anew took a
// large part of this package's time.
func TestMain(m *testing.M) {
	os.Exit(testutil.MainWithTemplate(m))
}
