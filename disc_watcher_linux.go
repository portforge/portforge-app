//go:build linux

package main

import (
	"path/filepath"

	"github.com/pilebones/go-udev/netlink"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"portforge/models"
)

// startDiscWatcher subscribes to kernel uevents via go-udev and fires
// disc:inserted / disc:ejected events immediately on state changes for any
// CD-ROM block device (MAJOR=11).
//
// If the netlink socket cannot be opened (e.g. inside a Flatpak sandbox),
// disc detection is unavailable and the user must refresh manually.
func (a *App) startDiscWatcher() {
	conn := new(netlink.UEventConn)
	if err := conn.Connect(netlink.KernelEvent); err != nil {
		return
	}

	go func() {
		defer conn.Close()

		queue := make(chan netlink.UEvent)
		errs := make(chan error)
		quit := conn.Monitor(queue, errs, &netlink.RuleDefinitions{
			Rules: []netlink.RuleDefinition{
				{Env: map[string]string{
					"SUBSYSTEM": "block",
					"MAJOR":     "11",
				}},
			},
		})

		// Seed known state so we don't fire spurious inserts on startup.
		known := map[string]bool{}
		if drives, err := listOpticalDrives(); err == nil {
			for _, d := range drives {
				known[d.Path] = d.HasDisc
			}
		}

		for {
			select {
			case <-a.ctx.Done():
				quit <- struct{}{}
				return
			case <-errs:
				// Non-fatal; the monitor keeps running.
			case ev := <-queue:
				devName := ueventDevName(ev.Env)
				if devName == "" {
					continue
				}
				switch ev.Action {
				case netlink.ADD:
					hasDisc := isDriveReady(devName)
					known[devName] = hasDisc
					if hasDisc {
						d := buildDrive(devName)
						wailsruntime.EventsEmit(a.ctx, "disc:inserted", d)
						go a.probeAndEmit(d)
					}
				case netlink.REMOVE:
					if known[devName] {
						wailsruntime.EventsEmit(a.ctx, "disc:ejected",
							map[string]string{"drive": devName})
					}
					delete(known, devName)
				case netlink.CHANGE:
					hasDisc := isDriveReady(devName)
					if hasDisc == known[devName] {
						continue
					}
					known[devName] = hasDisc
					if hasDisc {
						d := buildDrive(devName)
						wailsruntime.EventsEmit(a.ctx, "disc:inserted", d)
						go a.probeAndEmit(d)
					} else {
						wailsruntime.EventsEmit(a.ctx, "disc:ejected",
							map[string]string{"drive": devName})
					}
				}
			}
		}
	}()
}

// ueventDevName derives /dev/srX from a uevent environment map.
// Kernel uevents carry DEVNAME without the /dev/ prefix.
func ueventDevName(env map[string]string) string {
	if name := env["DEVNAME"]; name != "" {
		if name[0] != '/' {
			return "/dev/" + name
		}
		return name
	}
	if path := env["DEVPATH"]; path != "" {
		return "/dev/" + filepath.Base(path)
	}
	return ""
}

// buildDrive constructs an OpticalDrive for a device that is known to have a
// disc present.
func buildDrive(devName string) models.OpticalDrive {
	return models.OpticalDrive{
		Path:       devName,
		RawPath:    devName,
		Label:      filepath.Base(devName),
		HasDisc:    true,
		MountPoint: getMountPoint(devName),
	}
}
