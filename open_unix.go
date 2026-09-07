//go:build !windows && !darwin

package main

import "fmt"

// openInFileManager shows a folder in the desktop's file manager.
//
// xdg-open is the portable entry point on Linux; it hands off to whichever file
// manager the session actually uses rather than assuming one.
func openInFileManager(path string) error {
	cmd := newCommand("xdg-open", path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not open the file manager: %w", err)
	}
	go cmd.Wait() // reap the child; the file manager outlives this call
	return nil
}
