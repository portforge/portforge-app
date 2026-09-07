//go:build windows

package main

import "fmt"

func openInFileManager(path string) error {
	// explorer.exe returns a non-zero exit code even on success, so the result is
	// deliberately not waited on or checked beyond the spawn itself.
	cmd := newCommand("explorer", path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not open Explorer: %w", err)
	}
	go cmd.Wait()
	return nil
}
