//go:build linux

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"portforge/models"
)

const (
	cdromDriveStatus uintptr = 0x5326
	cdslCurrent      uintptr = 0xFFFF
	cdsDiscOK        uintptr = 4
)

// listOpticalDrives enumerates optical drives by reading /proc/sys/dev/cdrom/info
// and checks each drive for disc presence via the CDROM_DRIVE_STATUS ioctl.
func listOpticalDrives() ([]models.OpticalDrive, error) {
	names, err := readCDROMNames()
	if err != nil || len(names) == 0 {
		return nil, nil
	}

	drives := make([]models.OpticalDrive, 0, len(names))
	for _, name := range names {
		dev := "/dev/" + name
		hasDisc := isDriveReady(dev)
		mount := ""
		if hasDisc {
			mount = getMountPoint(dev)
		}
		drives = append(drives, models.OpticalDrive{
			Path:       dev,
			RawPath:    dev, // on Linux, block device and raw device are the same
			Label:      name,
			HasDisc:    hasDisc,
			MountPoint: mount,
		})
	}
	return drives, nil
}

// readCDROMNames parses /proc/sys/dev/cdrom/info and returns the drive names
// (e.g. ["sr0", "sr1"]). Returns nil if the file is missing (no CD-ROM subsystem).
func readCDROMNames() ([]string, error) {
	f, err := os.Open("/proc/sys/dev/cdrom/info")
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "drive name:") {
			return strings.Fields(strings.TrimPrefix(line, "drive name:")), nil
		}
	}
	return nil, nil
}

// isDriveReady uses the CDROM_DRIVE_STATUS ioctl to check whether a disc is
// present in the drive. Returns false on any error.
//
// O_NONBLOCK is required: opening a CD-ROM device without it sets FMODE_NDELAY
// to false, which causes the kernel's CDO_AUTO_CLOSE logic to close the tray
// automatically — exactly what we must not do while polling every 2 seconds.
func isDriveReady(device string) bool {
	fd, err := syscall.Open(device, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	defer syscall.Close(fd)
	r1, _, _ := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), cdromDriveStatus, cdslCurrent)
	return r1 == cdsDiscOK
}

// unmountDrive unmounts the disc's filesystem so redumper can open the device
// for exclusive raw SCSI access. udisksctl is used instead of umount(8)
// because it talks to the udisks2 daemon over D-Bus, which (via polkit) lets
// the active session user unmount removable media without root.
func unmountDrive(devicePath, mountPoint string) error {
	out, err := exec.Command("udisksctl", "unmount", "-b", devicePath).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}

// getMountPoint returns the current mount point for a device by reading
// /proc/mounts. Returns "" if the device is not mounted.
func getMountPoint(device string) string {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == device {
			return fields[1]
		}
	}
	return ""
}
