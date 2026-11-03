//go:build linux

package update

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"vpn_specify/internal/i18n"
)

// Prepare replaces the executable with the package's right away (Linux allows
// replacing a running program) and checks that the new one runs on this
// system. If the folder needs root (e.g. /usr/local/bin), it asks for the
// password with pkexec. Run reopens vpn_specify (relaunch) in place of this
// process.
func Prepare(archive, ver string, relaunch bool) (*Plan, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return prepareAt(archive, ver, relaunch, exe)
}

func prepareAt(archive, ver string, relaunch bool, exe string) (*Plan, error) {
	fresh := filepath.Join(filepath.Dir(archive), "vpn_specify")
	if err := extractTarFile(archive, "vpn_specify", fresh); err != nil {
		return nil, err
	}
	out, err := exec.Command(fresh, "version").Output()
	if got := strings.TrimSpace(string(out)); err != nil || got != ver {
		return nil, i18n.Error("the downloaded executable did not run on this system (version {0}, error {1})", strconv.Quote(got), err)
	}
	if err := replaceFile(fresh, exe); err != nil {
		return nil, err
	}
	return &Plan{Description: "atualizado em " + exe, Run: func() error {
		if !relaunch {
			return nil
		}
		return syscall.Exec(exe, []string{exe}, os.Environ())
	}}, nil
}

// replaceFile atomically replaces dst with src (copy alongside + rename);
// without write permission on the folder, it uses "pkexec install".
func replaceFile(src, dst string) error {
	staged := filepath.Join(filepath.Dir(dst), ".vpn_specify.new")
	if err := copyFile(src, staged); err == nil {
		if err := os.Rename(staged, dst); err == nil {
			return nil
		}
		os.Remove(staged)
	} else if !errors.Is(err, os.ErrPermission) {
		return err
	}
	pk, err := exec.LookPath("pkexec")
	if err != nil {
		return i18n.Error("no permission to write in {0}; update with: sudo vpn_specify update", filepath.Dir(dst))
	}
	inst, err := exec.LookPath("install")
	if err != nil {
		return err
	}
	if out, err := exec.Command(pk, inst, "-m", "0755", src, dst).CombinedOutput(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && (ee.ExitCode() == 126 || ee.ExitCode() == 127) {
			return i18n.Error("administrator permission denied; the update was not installed")
		}
		return i18n.Error("could not replace {0}: {1} {2}", dst, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}
