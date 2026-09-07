//go:build !windows

package storageunits

import (
	"os"

	"golang.org/x/sys/unix"
)

// lockFile takes an exclusive advisory lock on an already-open file and returns
// the release function. The lock is held for a whole read-modify-write, not just
// the write: every mutation reads the current list first, and two programs whose
// reads interleave would each write a list missing the other's change.
func lockFile(f *os.File) (func(), error) {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return nil, err
	}
	return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }, nil
}
