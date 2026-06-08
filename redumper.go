package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"portforge/models"
)

var progressRe = regexp.MustCompile(`(\d+(?:\.\d+)?)%`)

var (
	dumpMu     sync.Mutex
	dumpCancel context.CancelFunc
)

// GetRedumperPath returns the configured redumper path, or "" if not set.
func (a *App) GetRedumperPath() string {
	return a.redumperPath
}

// SetRedumperPath saves the redumper executable path to settings.
func (a *App) SetRedumperPath(path string) error {
	a.redumperPath = path
	return a.persistSettings()
}

// StartDump launches redumper to dump the disc in drive. imageName is used as
// the output filename base (without extension). Dumps land in {dataPath}/Dumps/.
func (a *App) StartDump(drive, imageName string) error {
	redumperBin := a.GetRedumperPath()
	if redumperBin == "" {
		return fmt.Errorf("redumper not found — set the path in Settings")
	}

	// Validate drive against the live list of optical drives. This prevents a
	// crafted frontend call from passing an arbitrary string (e.g. a redumper
	// flag like "--server=evil") as the drive argument.
	known, err := listOpticalDrives()
	if err != nil {
		return fmt.Errorf("cannot enumerate drives: %w", err)
	}
	var mountPoint string
	valid := false
	for _, d := range known {
		if d.Path == drive {
			valid = true
			mountPoint = d.MountPoint
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown drive: %s", drive)
	}

	dumpMu.Lock()
	if dumpCancel != nil {
		dumpMu.Unlock()
		return fmt.Errorf("a dump is already in progress")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	dumpCancel = cancel
	dumpMu.Unlock()

	outDir := filepath.Join(a.dataPath, "Dumps")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		cancel()
		dumpMu.Lock()
		dumpCancel = nil
		dumpMu.Unlock()
		return fmt.Errorf("cannot create dump directory: %w", err)
	}

	go a.runRedumper(ctx, cancel, redumperBin, drive, mountPoint, outDir, imageName)
	return nil
}

// CancelDump cancels any in-progress dump.
func (a *App) CancelDump() {
	dumpMu.Lock()
	defer dumpMu.Unlock()
	if dumpCancel != nil {
		dumpCancel()
		dumpCancel = nil
	}
}

func (a *App) runRedumper(
	ctx context.Context,
	cancel context.CancelFunc,
	bin, drive, mountPoint, outDir, imageName string,
) {
	defer func() {
		cancel()
		dumpMu.Lock()
		dumpCancel = nil
		dumpMu.Unlock()
	}()

	emit := func(p models.DumpProgress) {
		wailsruntime.EventsEmit(a.ctx, "dump:progress", p)
	}

	// redumper needs exclusive raw access to the device. If the OS has
	// auto-mounted the disc's filesystem, opening it fails with "device or
	// resource busy" — so unmount it first.
	if mountPoint != "" {
		if err := unmountDrive(drive, mountPoint); err != nil {
			emit(models.DumpProgress{Drive: drive, Phase: "error",
				Error: fmt.Sprintf("could not unmount disc before dumping: %s", err)})
			return
		}
	}

	// Sanitise the image name for the filesystem.
	safe := sanitiseImageName(imageName)
	if safe == "" {
		safe = "DISC"
	}

	args := []string{
		"--drive=" + drive,
		"--image-path=" + outDir,
		"--image-name=" + safe,
	}
	cmd := exec.CommandContext(ctx, bin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		emit(models.DumpProgress{Drive: drive, Phase: "error", Error: err.Error()})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		emit(models.DumpProgress{Drive: drive, Phase: "error", Error: err.Error()})
		return
	}

	if err := cmd.Start(); err != nil {
		emit(models.DumpProgress{Drive: drive, Phase: "error", Error: err.Error()})
		return
	}

	combined := io.MultiReader(stdout, stderr)
	scanner := bufio.NewScanner(combined)
	var lastPct float64
	var recentLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if m := progressRe.FindStringSubmatch(line); m != nil {
			pct, _ := strconv.ParseFloat(m[1], 64)
			if pct != lastPct {
				lastPct = pct
				emit(models.DumpProgress{Drive: drive, Phase: "dumping", Percent: pct})
			}
			continue
		}
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			recentLines = append(recentLines, trimmed)
			if len(recentLines) > 5 {
				recentLines = recentLines[1:]
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return // user cancelled
		}
		msg := err.Error()
		if len(recentLines) > 0 {
			msg = strings.Join(recentLines, "\n")
		}
		emit(models.DumpProgress{Drive: drive, Phase: "error", Error: msg})
		return
	}

	emit(models.DumpProgress{Drive: drive, Phase: "done", Percent: 100})
}

// sanitiseImageName strips characters that are unsafe in filenames across all
// three supported platforms, and removes any leading dashes so the result can
// never be interpreted as a CLI flag by redumper's argument parser.
func sanitiseImageName(name string) string {
	replacer := strings.NewReplacer(
		`/`, "-", `\`, "-", `:`, "-", `*`, "-",
		`?`, "-", `"`, "-", `<`, "-", `>`, "-", `|`, "-",
	)
	name = strings.TrimSpace(replacer.Replace(name))
	name = strings.TrimLeft(name, "-")
	return name
}
