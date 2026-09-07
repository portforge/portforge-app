//go:build windows

package storageunits

import "golang.org/x/sys/windows"

// DiskSpace reports the capacity of the volume holding path and the space an
// unprivileged caller can actually write to it.
//
// GetDiskFreeSpaceEx takes a directory rather than a drive letter and works for
// UNC paths, so a unit on a network share measures like any other. freeToCaller
// honours per-user quotas; totalFree does not, so the caller's figure is the one
// that matches what they can write — the Windows counterpart of Bavail.
func DiskSpace(path string) (total, free uint64, err error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeToCaller, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeToCaller, &totalBytes, &totalFree); err != nil {
		return 0, 0, err
	}
	return totalBytes, freeToCaller, nil
}
