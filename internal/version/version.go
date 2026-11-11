// SPDX-License-Identifier: Apache-2.0

// Package version holds the build version and date, set at link time.
package version

import "time"

// Version is overwritten with -ldflags "-X .../internal/version.Version=v1.2.3".
var Version = "dev"

// Date is the date of the commit the binary was built from, as 2006-01-02, set with
// -ldflags "-X .../internal/version.Date=...". The commit date keeps builds reproducible;
// a developer build leaves it empty (spec 051, D6).
var Date = ""

// BuildDate is Date as a time, or zero when it is empty or not a date.
func BuildDate() time.Time {
	at, err := time.Parse(time.DateOnly, Date)
	if err != nil {
		return time.Time{}
	}
	return at
}
