package main

// GetDuckStationPath returns the configured DuckStation executable path, or "" if not set.
func (a *App) GetDuckStationPath() string {
	return a.duckstationPath
}

// SetDuckStationPath saves the DuckStation executable path to settings.
func (a *App) SetDuckStationPath(path string) error {
	a.duckstationPath = path
	return a.persistSettings()
}

// duckStationLaunchArgs returns the command-line arguments that boot romPath
// directly and close DuckStation when the game exits.
func duckStationLaunchArgs(romPath string) []string {
	return []string{"-batch", romPath}
}
