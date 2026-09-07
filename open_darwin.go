//go:build darwin

package main

import "fmt"

func openInFileManager(path string) error {
	cmd := newCommand("open", path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not open Finder: %w", err)
	}
	go cmd.Wait()
	return nil
}
