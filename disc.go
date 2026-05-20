package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"portforge/models"
)

// discKey returns a stable identifier for a drive that works across platforms.
// On macOS the device node changes per-disc; using the mount point is stable.
// On Linux and Windows the device path is stable.
func discKey(d models.OpticalDrive) string {
	if d.MountPoint != "" {
		return d.MountPoint
	}
	return d.Path
}

// probeAndEmit waits for the disc to mount (on Linux), then identifies it and
// emits "disc:identified".
func (a *App) probeAndEmit(drive models.OpticalDrive) {
	if drive.MountPoint == "" {
		// Give the automount daemon time to mount the disc.
		time.Sleep(3 * time.Second)
		if drives, err := listOpticalDrives(); err == nil {
			for _, d := range drives {
				if d.Path == drive.Path {
					drive = d
					break
				}
			}
		}
	}
	info := probeDisc(drive)
	wailsruntime.EventsEmit(a.ctx, "disc:identified", info)
}

// GetOpticalDrives returns the current list of optical drives with disc status.
func (a *App) GetOpticalDrives() ([]models.OpticalDrive, error) {
	return listOpticalDrives()
}

// ScanDiscs refreshes the drive list and, for every drive that has a disc,
// starts probing in the background — emitting disc:identified when done.
// This is the primary entry point on platforms without automatic monitoring
// (macOS, Windows) and is also useful as a manual refresh on Linux.
func (a *App) ScanDiscs() ([]models.OpticalDrive, error) {
	drives, err := listOpticalDrives()
	if err != nil {
		return nil, err
	}
	for _, d := range drives {
		if d.HasDisc {
			go a.probeAndEmit(d)
		}
	}
	return drives, nil
}

// probeDisc identifies the system and serial from a mounted disc.
func probeDisc(drive models.OpticalDrive) models.DiscInfo {
	info := models.DiscInfo{Drive: drive.Path, MountPoint: drive.MountPoint}

	if drive.MountPoint != "" {
		if system, serial, ok := readSystemCNF(drive.MountPoint); ok {
			info.System = system
			info.Serial = serial
			info.DiscType = "cd"
			info.Volume = readISOVolumeLabel(drive.RawPath)
			return info
		}

		if _, err := os.Stat(filepath.Join(drive.MountPoint, "default.xbe")); err == nil {
			info.System = "xbox"
			info.DiscType = "dvd"
			return info
		}

		if _, err := os.Stat(filepath.Join(drive.MountPoint, "default.xex")); err == nil {
			info.System = "xbox360"
			info.DiscType = "dvd"
			return info
		}
	}

	// Raw byte probe for GameCube/Wii — these use proprietary filesystems and
	// typically do not auto-mount on PC operating systems.
	if sys := probeRawDevice(drive.RawPath); sys != "" {
		info.System = sys
		info.DiscType = "dvd"
		return info
	}

	if drive.MountPoint != "" {
		if entries, err := os.ReadDir(drive.MountPoint); err == nil && len(entries) > 0 {
			info.DiscType = "data"
			info.Volume = readISOVolumeLabel(drive.RawPath)
		}
	}

	return info
}

// readSystemCNF parses a PS1/PS2 SYSTEM.CNF file to identify the system and
// extract the game serial.
func readSystemCNF(mountPoint string) (system, serial string, ok bool) {
	for _, name := range []string{"SYSTEM.CNF", "system.cnf"} {
		data, err := os.ReadFile(filepath.Join(mountPoint, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			kv := strings.SplitN(strings.TrimSpace(line), "=", 2)
			if len(kv) != 2 {
				continue
			}
			key := strings.TrimSpace(kv[0])
			val := strings.TrimSpace(kv[1])
			if key == "BOOT2" {
				return "ps2", parseDiscSerial(val), true
			}
			if key == "BOOT" {
				return "psx", parseDiscSerial(val), true
			}
		}
	}
	return "", "", false
}

// parseDiscSerial normalises the serial from a BOOT/BOOT2 path value.
// "cdrom0:\SLUS_20734.01;1" → "SLUS-20734"
// "cdrom:\SLUS_005.94;1"    → "SLUS-00594"
func parseDiscSerial(val string) string {
	// Extract the filename component.
	base := val
	if idx := strings.LastIndexAny(base, `/\\`); idx >= 0 {
		base = base[idx+1:]
	}
	// Strip the ;N version suffix.
	if idx := strings.Index(base, ";"); idx >= 0 {
		base = base[:idx]
	}
	// Strip the .NN revision suffix (e.g. .01 on PS2 serials).
	if idx := strings.LastIndex(base, "."); idx > 3 {
		base = base[:idx]
	}
	return strings.ToUpper(strings.ReplaceAll(base, "_", "-"))
}

// readISOVolumeLabel reads the volume label from an ISO 9660 Primary Volume
// Descriptor on the given raw device path. Returns "" on any error.
func readISOVolumeLabel(rawDevice string) string {
	if rawDevice == "" {
		return ""
	}
	f, err := os.Open(rawDevice)
	if err != nil {
		return ""
	}
	defer f.Close()

	// PVD is at sector 16, sectors are 2048 bytes.
	// Volume identifier spans bytes 40–71 of the PVD.
	buf := make([]byte, 2048)
	if _, err := f.ReadAt(buf, 16*2048); err != nil {
		return ""
	}
	if buf[0] != 1 || string(buf[1:6]) != "CD001" {
		return ""
	}
	return strings.TrimRight(string(buf[40:72]), " ")
}

// probeRawDevice reads the first 32 bytes of a disc device and checks for
// GameCube (0xC2339F3D) and Wii (0x5D1C9EA3) magic values at offset 28.
func probeRawDevice(rawDevice string) string {
	if rawDevice == "" {
		return ""
	}
	f, err := os.Open(rawDevice)
	if err != nil {
		return ""
	}
	defer f.Close()

	var buf [32]byte
	if _, err := f.ReadAt(buf[:], 0); err != nil {
		return ""
	}
	magic := uint32(buf[28])<<24 | uint32(buf[29])<<16 | uint32(buf[30])<<8 | uint32(buf[31])
	switch magic {
	case 0xC2339F3D:
		return "gamecube"
	case 0x5D1C9EA3:
		return "wii"
	}
	return ""
}
