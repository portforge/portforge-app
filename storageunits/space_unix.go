//go:build !windows

package storageunits

import "golang.org/x/sys/unix"

// DiskSpace reports the capacity of the filesystem holding path and the space an
// unprivileged caller can actually write to it.
//
// Bavail rather than Bfree: the difference is the reserve only root may use, so
// Bfree would promise space a rip or a build cannot have.
func DiskSpace(path string) (total, free uint64, err error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := uint64(st.Bsize)
	return st.Blocks * bsize, st.Bavail * bsize, nil
}
