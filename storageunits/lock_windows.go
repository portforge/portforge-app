//go:build windows

package storageunits

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive lock on an already-open file and returns the
// release function. See the Unix implementation for why the lock spans the whole
// read-modify-write.
func lockFile(f *os.File) (func(), error) {
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	const exclusive = windows.LOCKFILE_EXCLUSIVE_LOCK
	if err := windows.LockFileEx(h, exclusive, 0, ^uint32(0), ^uint32(0), ol); err != nil {
		return nil, err
	}
	return func() { _ = windows.UnlockFileEx(h, 0, ^uint32(0), ^uint32(0), ol) }, nil
}
