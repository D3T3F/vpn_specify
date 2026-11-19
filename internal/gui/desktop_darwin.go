//go:build darwin

package gui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/store"
)

// macOS desktop integration: the apps are application bundles (.app) chosen
// in the system file picker, there is no applications menu and no shortcuts
// (the page hides that feature), and URLs open with "open".

var errNoShortcuts = i18n.Sentinel("shortcuts are not available on macOS")

func hasDisplay() bool { return true }

// listInstalledApps: there is no applications menu to list; the apps are
// chosen with "Choose file…".
func listInstalledApps() []installedApp { return nil }

// pickProgram: "Choose…" in the app dialog. An application bundle runs the
// program named in its Info.plist; a program is taken as it is.
func pickProgram() (*pickedApp, error) {
	p, ok, err := chooseFile(i18n.T("Choose the program or shortcut"))
	if err != nil || !ok {
		return nil, err
	}
	if isAppBundle(p) {
		return appFromBundle(strings.TrimRight(p, "/"))
	}
	if fi, err := os.Stat(p); err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
		return nil, i18n.Error("{0} is not an executable program", filepath.Base(p))
	}
	return &pickedApp{Name: filepath.Base(p), Path: p, Capture: []string{filepath.Base(p)}}, nil
}

// appFromBundle: the program of an application bundle is the executable named
// by CFBundleExecutable in Contents/Info.plist.
func appFromBundle(bundle string) (*pickedApp, error) {
	out, err := exec.Command("/usr/bin/defaults", "read", filepath.Join(bundle, "Contents", "Info"), "CFBundleExecutable").Output()
	exe := strings.TrimSpace(string(out))
	if err != nil || exe == "" {
		return nil, i18n.Error("{0} is not an executable program", filepath.Base(bundle))
	}
	p := filepath.Join(bundle, "Contents", "MacOS", exe)
	return &pickedApp{Name: strings.TrimSuffix(filepath.Base(bundle), ".app"), Path: p, Capture: []string{filepath.Base(p)}}, nil
}

// chooseFile opens the system file picker (osascript). ok=false if canceled.
func chooseFile(title string) (string, bool, error) {
	const script = "on run argv\nreturn POSIX path of (choose file with prompt (item 1 of argv))\nend run"
	out, err := exec.Command("osascript", "-e", script, title).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return "", false, nil // canceled (the user pressed Cancel)
		}
		return "", false, err
	}
	p := strings.TrimRight(string(out), "\r\n")
	return p, p != "", nil
}

// Shortcuts are not offered on macOS; the page hides them, and these return
// the error if they are asked for anyway.
func createShortcut(*store.Store, string, string, bool) (string, error) {
	return "", errNoShortcuts
}

func createMainShortcut() (string, error) { return "", errNoShortcuts }

func removeShortcut(store.AppEntry) {}

// anyRunning reports whether some process is still alive in the pgids groups
// or has one of the executable names in names (read from ps).
func anyRunning(pgids map[int]bool, names map[string]bool) bool {
	out, err := exec.Command("ps", "-A", "-o", "pgid=,stat=,comm=").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line) // pgid, state, then the path (it may contain spaces)
		if len(f) < 3 {
			continue
		}
		pg, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		if pgids[pg] && !strings.HasPrefix(f[1], "Z") {
			return true
		}
		if names[filepath.Base(strings.Join(f[2:], " "))] {
			return true
		}
	}
	return false
}

// openWithDefault opens a folder or URL in the default program (open). The
// target is an argument, never run through a shell.
func openWithDefault(target string) error {
	if err := checkOpenTarget(target); err != nil {
		return err
	}
	_, err := startDetached("/usr/bin/open", []string{target}, "", nil)
	return err
}
