//go:build dev

package main

import (
	"os"
	"path/filepath"
)

func init() {
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	devMetadataOverride = filepath.Join(wd, "mediaitems")
}
