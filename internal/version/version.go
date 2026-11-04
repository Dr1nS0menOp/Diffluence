// SPDX-License-Identifier: Apache-2.0

// Package version holds the build version, set at link time.
package version

// Version is overwritten with -ldflags "-X .../internal/version.Version=v1.2.3".
var Version = "dev"
