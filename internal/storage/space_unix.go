// SPDX-License-Identifier: Apache-2.0

//go:build unix

package storage

import (
	"context"
	"fmt"
	"syscall"
)

// Space says how much room the disk under the root has, in bytes.
func (d *Disk) Space(_ context.Context) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(d.root, &st); err != nil {
		return 0, 0, fmt.Errorf("storage: disk space: %w", err)
	}
	// Bsize is int64 on Linux and uint32 elsewhere, so it is converted once.
	block := uint64(st.Bsize)
	return st.Bavail * block, st.Blocks * block, nil
}
