//go:build !windows

package main

import "os/exec"

func newCommand(path string, args ...string) *exec.Cmd {
	return exec.Command(path, args...)
}
