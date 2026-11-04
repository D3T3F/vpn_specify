package tunnel

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// hideOpenVPN makes findOpenVPN look only at an empty folder. Skipped where a
// standard Linux path has a real openvpn, which the test must not touch.
func hideOpenVPN(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("ProgramFiles", dir)
	t.Setenv("ProgramFiles(x86)", dir)
	if _, err := findOpenVPN(""); !errors.Is(err, ErrOpenVPNMissing) {
		t.Skipf("openvpn is installed at a fixed path: %v", err)
	}
	return dir
}

func TestLocateOpenVPNInstallFlow(t *testing.T) {
	dir := hideOpenVPN(t)

	// no hook: the plain error, as before
	if _, err := locateOpenVPN(Options{}); !errors.Is(err, ErrOpenVPNMissing) {
		t.Fatalf("no hook: %v", err)
	}

	// declined: nothing else happens, and the message is the plain one
	_, err := locateOpenVPN(Options{InstallOpenVPN: func(func(string)) error { return ErrOpenVPNMissing }})
	if !errors.Is(err, ErrOpenVPNMissing) || strings.Contains(err.Error(), "automatically") {
		t.Fatalf("declined: %v", err)
	}

	// failed: counts as missing (autostart does not retry) and says why
	_, err = locateOpenVPN(Options{InstallOpenVPN: func(func(string)) error { return errors.New("boom") }})
	if !errors.Is(err, ErrOpenVPNMissing) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("failed: %v", err)
	}

	// "succeeded" without producing the program: still an error
	_, err = locateOpenVPN(Options{InstallOpenVPN: func(func(string)) error { return nil }})
	if !errors.Is(err, ErrOpenVPNMissing) {
		t.Fatalf("not produced: %v", err)
	}

	// installed: found on the second look, and the progress is cleared at the end
	name := "openvpn"
	if runtime.GOOS == "windows" {
		name = "openvpn.exe"
	}
	var steps []string
	exe, err := locateOpenVPN(Options{
		Progress: func(s string) { steps = append(steps, s) },
		InstallOpenVPN: func(progress func(string)) error {
			progress("working")
			return os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755)
		},
	})
	if err != nil || !strings.EqualFold(filepath.Dir(exe), dir) {
		t.Fatalf("installed: %q, %v", exe, err)
	}
	if len(steps) != 2 || steps[0] != "working" || steps[1] != "" {
		t.Fatalf("progress: %q", steps)
	}

	// already there: the hook is not called
	if _, err := locateOpenVPN(Options{InstallOpenVPN: func(func(string)) error {
		t.Error("asked to install although openvpn exists")
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitPathNeverInstalls(t *testing.T) {
	hideOpenVPN(t)
	exe, err := locateOpenVPN(Options{OpenVPNPath: "custom-openvpn", InstallOpenVPN: func(func(string)) error {
		t.Error("asked to install although a path was given")
		return nil
	}})
	if err != nil || exe != "custom-openvpn" {
		t.Fatalf("%q, %v", exe, err)
	}
}
