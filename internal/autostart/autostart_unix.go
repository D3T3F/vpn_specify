//go:build !windows

package autostart

import (
	"os"
	"path/filepath"
	"strings"
)

// File is the startup .desktop file (XDG autostart).
func File() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", "vpn_specify.desktop")
}

// quote escapes an argument for the Exec key of a .desktop file.
func quote(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\n\"'\\><~|&;$*?#()`%") {
		return a
	}
	r := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", `$`, `\\$`, `%`, `%%`)
	return `"` + r.Replace(a) + `"`
}

// Enable creates (or updates) the entry to run exe when the session starts.
func Enable(exe string) error {
	content := "[Desktop Entry]\nType=Application\nVersion=1.0\n" +
		"Name=vpn_specify (apps in VPN)\n" +
		"Comment=Opens in the VPN the apps marked to start with the system\n" +
		"Exec=" + quote(exe) + " " + Arg + "\n" +
		"Terminal=false\nNoDisplay=true\n" +
		"X-GNOME-Autostart-enabled=true\nX-GNOME-Autostart-Delay=10\n"
	p := File()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Disable removes the entry (not an error if it does not exist).
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
