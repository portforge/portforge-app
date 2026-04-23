//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func newCommand(path string) *exec.Cmd {
	cmd := exec.Command(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
	return cmd
}
