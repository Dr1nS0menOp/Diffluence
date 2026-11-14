// SPDX-License-Identifier: Apache-2.0

package publishing

import "time"

// Limits are the bounds of runs (chapter 12 of the study). They are a value of the service,
// so a test can lower them to reach a batch or a limit with a few pages.
type Limits struct {
	// OneTransaction is the largest run applied in one transaction: readers see the published
	// space as it was or as it will be, never half (PUB-450, PUB-1411).
	OneTransaction int
	// BatchSize is the number of pages of each batch of a larger run (PUB-451).
	BatchSize int
	// MaxPages is the largest run; a larger draft space is released by branches (PUB-1055).
	MaxPages int
	// PerPage and BytesPerSecond estimate how long a run takes. A run estimated past
	// TransactionBudget goes in batches even when it is small enough for one transaction, so
	// no transaction holds its locks and old rows for long (PUB-1106).
	PerPage           time.Duration
	BytesPerSecond    int64
	TransactionBudget time.Duration
	// RunsPerNode bounds the runs one worker carries out at once, so runs take at most two
	// connections of the pool (PUB-1105).
	RunsPerNode int
	// StaleAfter is how long a running run may go without a sign of life before it shows as
	// stopped, after its node died, so it can be resumed (PUB-1061, PUB-1032).
	StaleAfter time.Duration
	// Pause is how long a run waits before it asks again whether another run of its pair has
	// finished (PUB-452).
	Pause time.Duration
}

// DefaultLimits are those of the study: one transaction up to 1,000 pages and 30 seconds,
// batches of 250, at most 50,000 pages and two runs at once per node.
func DefaultLimits() Limits {
	// PerPage is what a page of a run took on the development machine of wave PB2, under load.
	return Limits{OneTransaction: 1000, BatchSize: 250, MaxPages: 50000, PerPage: 30 * time.Millisecond,
		BytesPerSecond: 25 << 20, TransactionBudget: 30 * time.Second, RunsPerNode: 2, StaleAfter: 2 * time.Minute, Pause: time.Second}
}

// estimate is how long a run of pages and bytes is expected to take.
func (l Limits) estimate(pages int, bytes int64) time.Duration {
	out := time.Duration(pages) * l.PerPage
	if l.BytesPerSecond > 0 {
		out += time.Duration(bytes * int64(time.Second) / l.BytesPerSecond)
	}
	return out
}

// batchesFor is the number of batches of a run, or 0 for one transaction (PUB-451, PUB-1106).
func (l Limits) batchesFor(pages int, bytes int64) int {
	if pages <= l.OneTransaction && l.estimate(pages, bytes) <= l.TransactionBudget {
		return 0
	}
	return (pages + l.BatchSize - 1) / l.BatchSize
}
