// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package storage

import (
	"context"
	"errors"
)

// Space is not known on this system, so the console stays silent about the disk.
func (d *Disk) Space(_ context.Context) (free, total uint64, err error) {
	return 0, 0, errors.New("storage: disk space is not known on this system")
}
