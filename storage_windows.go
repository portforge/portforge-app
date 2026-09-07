//go:build windows

package main

import "golang.org/x/sys/windows"

// diskSpace reports the capacity and free space of the volume holding path.
//
// GetDiskFreeSpaceEx takes a directory rather than a drive letter and works for
// UNC paths, so a library on a network share measures like any other.
func diskSpace(path string) (total, free uint64, err error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	// freeToCaller honours per-user quotas; totalFree does not. The caller's
	// figure is the one that matches what they can write.
	var freeToCaller, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeToCaller, &totalBytes, &totalFree); err != nil {
		return 0, 0, err
	}
	return totalBytes, freeToCaller, nil
}
