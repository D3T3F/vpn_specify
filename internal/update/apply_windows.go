//go:build windows

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/setup"
)

// Prepare extracts vpn_specify-setup.exe from the package. Run starts the
// installer, which waits for this process to end and then:
//   - /update: updates the installation (same folder, keeps existing shortcuts);
//   - /extract=<folder>: portable copy, only replaces the folder's files.
//
// With relaunch, the installer opens vpn_specify again at the end.
func Prepare(archive, ver string, relaunch bool) (*Plan, error) {
	setupExe := filepath.Join(filepath.Dir(archive), "vpn_specify-setup.exe")
	if err := extractZipFile(archive, "vpn_specify-setup.exe", setupExe); err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exeDir := filepath.Dir(exe)
	args := []string{"/wait=" + strconv.Itoa(os.Getpid())}
	desc := i18n.T("update the installation in {0}", exeDir)
	if inst := setup.InstalledDir(); inst != "" && strings.EqualFold(filepath.Clean(inst), filepath.Clean(exeDir)) {
		args = append(args, "/update")
	} else {
		args = append(args, "/extract="+exeDir)
		desc = i18n.T("update the portable copy in {0}", exeDir)
	}
	if relaunch {
		args = append(args, "/run")
	}
	return &Plan{Description: desc, Run: func() error {
		return exec.Command(setupExe, args...).Start()
	}}, nil
}
