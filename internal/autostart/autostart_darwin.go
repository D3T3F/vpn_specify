//go:build darwin

package autostart

import (
	"os"
	"path/filepath"
)

const launchLabel = "io.github.d3t3f.vpn_specify"

// File is the LaunchAgent that runs vpn_specify when the user signs in.
func File() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchLabel+".plist")
}

// Enable creates (or updates) the LaunchAgent that runs exe at sign-in. launchd
// reads it at the next sign-in, so no launchctl call is needed.
func Enable(exe string) error {
	p := File()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(launchAgentPlist(launchLabel, []string{exe, Arg})), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Disable removes the LaunchAgent (not an error if it does not exist).
func Disable() error {
	if err := os.Remove(File()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func Enabled() bool {
	_, err := os.Stat(File())
	return err == nil
}
