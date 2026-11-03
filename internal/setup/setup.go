package setup

import (
	"os"
	"path/filepath"
	"strings"

	"vpn_specify/internal/i18n"
)

const (
	AppName   = "vpn_specify"
	MainExe   = "vpn_specify.exe"
	CLIExe    = "vpn_specify-cli.exe"
	Publisher = "vpn_specify"
	// "Installed apps" key (Control Panel / Settings)
	uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\vpn_specify`
	shortcutName = "vpn_specify.lnk"
)

// Options are the choices made in the installer window.
type Options struct {
	Dir       string `json:"dir"`
	Desktop   bool   `json:"desktop"`
	StartMenu bool   `json:"startMenu"`
	Language  string `json:"language"` // interface language chosen in the installer
}

// DefaultDir is C:\Program Files\vpn_specify.
func DefaultDir() string {
	pf := os.Getenv("ProgramW6432") // 64-bit Program Files, even from a 32-bit process
	if pf == "" {
		pf = os.Getenv("ProgramFiles")
	}
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, AppName)
}

// NormalizeDir ensures an absolute path ending in \vpn_specify (choosing
// "D:\Apps" gives "D:\Apps\vpn_specify").
func NormalizeDir(dir string) (string, error) {
	dir = strings.TrimSpace(strings.Trim(strings.TrimSpace(dir), `"`))
	if dir == "" {
		return "", i18n.Error("choose the installation folder")
	}
	if !filepath.IsAbs(dir) || filepath.VolumeName(dir) == "" {
		return "", i18n.Error("use a full path, for example {0}", `C:\Program Files\vpn_specify`)
	}
	dir = filepath.Clean(dir)
	if !strings.EqualFold(filepath.Base(dir), AppName) {
		dir = filepath.Join(dir, AppName)
	}
	return dir, nil
}
