//go:build !linux

package main

// startDiscWatcher is a no-op on non-Linux platforms. Disc detection is
// triggered manually via ScanDiscs instead of running a background watcher.
func (a *App) startDiscWatcher() {}
