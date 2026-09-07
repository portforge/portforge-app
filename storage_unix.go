//go:build !windows

package main

import "golang.org/x/sys/unix"

// diskSpace reports the capacity and unprivileged free space of the filesystem
// holding path.
//
// Bavail rather than Bfree: the difference is the reserve only root may use, and
// the number shown to the user should be the space they can actually fill.
func diskSpace(path string) (total, free uint64, err error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := uint64(st.Bsize)
	return st.Blocks * bsize, st.Bavail * bsize, nil
}
