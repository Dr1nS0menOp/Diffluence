// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package app

// freeSpace cannot tell on this system. Diffluence ships for Linux only, so this only
// lets the code build and its tests run elsewhere.
func freeSpace(string) (bytes uint64, known bool, err error) { return 0, false, nil }
