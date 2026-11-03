//go:build windows

package setup

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/platform"
	"vpn_specify/internal/version"
)

// RunningInstances lists the PIDs of vpn_specify running from the dir folder
// (except this process). Copies in other folders do not get in the way.
func RunningInstances(dir string) []uint32 {
	var out []uint32
	for pid := range platform.RunningPIDs(map[string]bool{MainExe: true, CLIExe: true}) {
		if int(pid) == os.Getpid() {
			continue
		}
		p := platform.ProcessPath(pid)
		if p == "" || strings.EqualFold(filepath.Dir(p), filepath.Clean(dir)) {
			out = append(out, pid) // path not accessible: better assume it is from this folder
		}
	}
	return out
}

var errRunning = i18n.Sentinel("vpn_specify is open. Open its window, click Disconnect and close it (the apps opened through the VPN must be closed too); then try again")

// Install copies the payload files (root = content of the installation
// folder), creates the shortcuts and registers the uninstaller. progress
// receives short messages for the window.
func Install(payload fs.FS, o Options, progress func(string)) (string, error) {
	dir, err := NormalizeDir(o.Dir)
	if err != nil {
		return "", err
	}
	if len(RunningInstances(dir)) > 0 {
		return "", errRunning
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", i18n.Wrap(err, "could not create {0}", dir)
	}

	var total int64
	err = fs.WalkDir(payload, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		progress(i18n.T("Copying {0}…", p))
		n, err := copyFile(payload, p, filepath.Join(dir, filepath.FromSlash(p)))
		total += n
		return err
	})
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, MainExe)); err != nil {
		return "", i18n.Error("the installer does not contain vpn_specify.exe (incomplete package)")
	}

	exe := filepath.Join(dir, MainExe)
	desc := i18n.T("vpn_specify: VPN only for the apps you choose")
	if o.StartMenu {
		progress(i18n.T("Creating the Start menu shortcut…"))
		if err := platform.CreateShortcut(filepath.Join(platform.StartMenuDir(), shortcutName), exe, "", dir, exe+",0", desc); err != nil {
			return "", i18n.Wrap(err, "Start menu shortcut")
		}
	}
	if o.Desktop {
		progress(i18n.T("Creating the desktop shortcut…"))
		if err := platform.CreateShortcut(filepath.Join(platform.DesktopDir(), shortcutName), exe, "", dir, exe+",0", desc); err != nil {
			return "", i18n.Wrap(err, "desktop shortcut")
		}
	}

	progress(i18n.T("Registering in Installed apps…"))
	if err := register(dir, total); err != nil {
		return "", i18n.Wrap(err, "uninstall registration")
	}
	progress(i18n.T("Done."))
	return dir, nil
}

// copyFile writes to a temporary file and swaps it in at the end, so a
// half-written .exe is never left behind if something fails.
func copyFile(src fs.FS, name, dst string) (int64, error) {
	in, err := src.Open(name)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return 0, i18n.Wrap(err, "could not write {0}", dst)
	}
	n, err := io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return 0, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return 0, i18n.Wrap(err, "could not replace {0} (is it in use?)", dst)
	}
	return n, nil
}

func register(dir string, size int64) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, uninstallKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	exe := filepath.Join(dir, MainExe)
	for name, v := range map[string]string{
		"DisplayName":     AppName,
		"DisplayVersion":  version.Version,
		"Publisher":       Publisher,
		"InstallLocation": dir,
		"DisplayIcon":     exe + ",0",
		"UninstallString": `"` + exe + `" uninstall`,
	} {
		if err := k.SetStringValue(name, v); err != nil {
			return err
		}
	}
	for name, v := range map[string]uint32{"NoModify": 1, "NoRepair": 1, "EstimatedSize": uint32(size / 1024)} {
		if err := k.SetDWordValue(name, v); err != nil {
			return err
		}
	}
	return nil
}

// InstalledDir returns the folder registered by a previous installation.
func InstalledDir() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstallKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("InstallLocation")
	return v
}

// UpdateOptions: the options of the current installation, to update
// without changing anything (same folder; only recreates shortcuts that
// still exist).
func UpdateOptions() Options {
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	return Options{
		Dir:       InstalledDir(),
		Desktop:   exists(filepath.Join(platform.DesktopDir(), shortcutName)),
		StartMenu: exists(filepath.Join(platform.StartMenuDir(), shortcutName)),
	}
}

// Extract only copies the files into dir (portable copy update: no registry
// and no shortcuts).
func Extract(payload fs.FS, dir string, progress func(string)) error {
	if len(RunningInstances(dir)) > 0 {
		return errRunning
	}
	return fs.WalkDir(payload, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		progress(i18n.T("Copying {0}…", p))
		_, err = copyFile(payload, p, filepath.Join(dir, filepath.FromSlash(p)))
		return err
	})
}

// WaitProcessExit waits for process pid to end (used when updating:
// vpn_specify starts the installer and exits right after).
func WaitProcessExit(pid uint32, timeout time.Duration) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return // already ended
	}
	defer windows.CloseHandle(h)
	_, _ = windows.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
}
