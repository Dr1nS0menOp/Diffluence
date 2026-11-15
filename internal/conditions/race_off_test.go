// SPDX-License-Identifier: Apache-2.0

//go:build !race

package conditions

// raceEnabled says the race detector runs, which slows everything and so makes a measurement
// of time meaningless.
const raceEnabled = false
