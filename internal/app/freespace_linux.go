// SPDX-License-Identifier: Apache-2.0

package app

import "syscall"

// freeSpace is the room an unprivileged process has left on the file system of dir.
func freeSpace(dir string) (bytes uint64, known bool, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, false, err
	}
	// Bsize is signed on some architectures, but a block size is never negative.
	return st.Bavail * uint64(st.Bsize), true, nil
}
