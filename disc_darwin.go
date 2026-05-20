//go:build darwin

package main

import (
	"os"
	"os/exec"
	"strings"

	"portforge/models"
)

// listOpticalDrives finds mounted optical discs by enumerating /Volumes and
// confirming each is optical via "diskutil info".
//
// On macOS, optical discs are auto-mounted to /Volumes/{label} on insertion,
// so the presence of an optical volume IS the presence of a disc.
func listOpticalDrives() ([]models.OpticalDrive, error) {
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return nil, err
	}

	var drives []models.OpticalDrive
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		volPath := "/Volumes/" + e.Name()
		devPath, isOptical := getOpticalDeviceInfo(volPath)
		if !isOptical {
			continue
		}
		// /dev/diskN → /dev/rdiskN for raw sector access.
		rawPath := strings.Replace(devPath, "/dev/disk", "/dev/rdisk", 1)
		drives = append(drives, models.OpticalDrive{
			Path:       devPath,
			RawPath:    rawPath,
			Label:      e.Name(),
			HasDisc:    true,
			MountPoint: volPath,
		})
	}
	return drives, nil
}

// getOpticalDeviceInfo runs "diskutil info <volPath>" and checks whether the
// volume is on optical media. Returns the device node and true if it is.
func getOpticalDeviceInfo(volPath string) (devPath string, isOptical bool) {
	out, err := exec.Command("diskutil", "info", volPath).Output()
	if err != nil {
		return "", false
	}
	info := string(out)
	if !strings.Contains(info, "Optical Media Type") {
		return "", false
	}
	for _, line := range strings.Split(info, "\n") {
		if strings.Contains(line, "Device Node:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return parts[len(parts)-1], true
			}
		}
	}
	return "", true
}
