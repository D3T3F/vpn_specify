// Package paths decides where vpn_specify keeps its data.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

// DataDir returns the data folder:
//   - portable mode: <exe folder>\data, if that folder already exists (or
//     there is a file named "portable" next to the exe);
//   - installed on Windows: %LOCALAPPDATA%\vpn_specify (Program Files is not
//     writable without Administrator);
//   - Linux/macOS: ~/.config/vpn_specify (or $XDG_CONFIG_HOME/vpn_specify).
func DataDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "data"
	}
	dir := filepath.Dir(exe)
	portable := filepath.Join(dir, "data")
	if fi, err := os.Stat(portable); err == nil && fi.IsDir() {
		return portable
	}
	if _, err := os.Stat(filepath.Join(dir, "portable")); err == nil {
		return portable
	}
	base, err := os.UserConfigDir()
	if runtime.GOOS == "windows" {
		base, err = os.UserCacheDir() // %LOCALAPPDATA%
	}
	if runtime.GOOS == "darwin" {
		base, err = macOSConfigBase() // os.UserConfigDir would use ~/Library/Application Support
	}
	if err == nil {
		return filepath.Join(base, "vpn_specify")
	}
	return portable
}

// macOSConfigBase is $XDG_CONFIG_HOME, or ~/.config: the same folder as on
// Linux, which the app uses on macOS too.
func macOSConfigBase() (string, error) {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}
