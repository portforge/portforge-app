//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func newCommand(path string, args ...string) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
	return cmd
}
