//go:build windows

package main

import (
	"portforge/models"
	"golang.org/x/sys/windows"
)

// listOpticalDrives checks all 26 possible drive letters for CD-ROM drives and
// reports whether each has a disc present.
func listOpticalDrives() ([]models.OpticalDrive, error) {
	var drives []models.OpticalDrive

	for _, letter := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		root := string(letter) + `:\`
		rootPtr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		if windows.GetDriveType(rootPtr) != windows.DRIVE_CDROM {
			continue
		}

		hasDisc, label := readDiscLabel(root)
		drives = append(drives, models.OpticalDrive{
			Path:       root,
			RawPath:    `\\.\` + string(letter) + `:`,
			Label:      label,
			HasDisc:    hasDisc,
			MountPoint: root,
		})
	}
	return drives, nil
}

// unmountDrive is a no-op on Windows — raw access to a CD-ROM device
// (\\.\D:) does not conflict with a mounted volume the way it does on
// Linux/macOS, so there is nothing to release before dumping.
func unmountDrive(devicePath, mountPoint string) error {
	return nil
}

// readDiscLabel attempts to read the volume label from a CD-ROM drive.
// Returns (false, "X:") if no disc is present.
func readDiscLabel(root string) (hasDisc bool, label string) {
	fallback := root[:2]

	rootPtr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return false, fallback
	}

	volName := make([]uint16, 256)
	fsName := make([]uint16, 256)
	var serial, maxLen, flags uint32

	err = windows.GetVolumeInformation(
		rootPtr,
		&volName[0], uint32(len(volName)),
		&serial, &maxLen, &flags,
		&fsName[0], uint32(len(fsName)),
	)
	if err != nil {
		return false, fallback
	}

	name := windows.UTF16ToString(volName)
	if name == "" {
		name = fallback
	}
	return true, name
}
