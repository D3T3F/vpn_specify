//go:build windows

package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"vpn_specify/internal/autostart"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/paths"
	"vpn_specify/internal/platform"
)

// Uninstall removes the vpn_specify installed in this executable's folder
// ("vpn_specify.exe uninstall", called from Installed apps).
//
// Options: /silent (no questions or messages; deletes the data unless
// /keepdata is given).
func Uninstall(args []string) error {
	silent, keepData := false, false
	for _, a := range args {
		switch strings.ToLower(a) {
		case "/silent", "-silent":
			silent = true
		case "/keepdata", "-keepdata":
			keepData = true
		}
	}
	if !platform.IsElevated() {
		return platform.ElevateSelf(append([]string{"uninstall"}, args...))
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	dataDir := paths.DataDir()
	portableData := strings.EqualFold(dataDir, filepath.Join(dir, "data"))
	notify := func(text string, flags uintptr) {
		if silent {
			fmt.Fprintln(os.Stderr, text)
		} else {
			platform.MessageBox(i18n.T("Uninstall vpn_specify"), text, flags)
		}
	}

	removeData := !keepData
	if !silent {
		answer := platform.Ask(i18n.T("Uninstall vpn_specify"),
			i18n.T("Uninstall vpn_specify from {0}?\n\nYes: removes the program and also its data (imported VPNs, apps and saved passwords).\nNo: removes only the program and keeps the data for a reinstall.\nCancel: does not uninstall.", dir), true)
		if answer == platform.AnswerCancel {
			return nil
		}
		removeData = answer == platform.AnswerYes
	}

	if len(RunningInstances(dir)) > 0 {
		notify(i18n.T("Could not uninstall: {0}.", errRunning), 0x30)
		return errRunning
	}

	var problems []string
	// shortcuts: the installer's and the ones created by the app ("DiscordVPN" etc.), which would point to a deleted exe
	for _, lnk := range append([]string{
		filepath.Join(platform.StartMenuDir(), shortcutName),
		filepath.Join(platform.DesktopDir(), shortcutName),
	}, appShortcuts(dataDir)...) {
		if err := os.Remove(lnk); err != nil && !errors.Is(err, os.ErrNotExist) {
			problems = append(problems, lnk)
		}
	}
	// scheduled task of the apps that start with the system
	if err := autostart.Disable(); err != nil {
		problems = append(problems, i18n.T("task {0}", autostart.TaskName))
	}
	if err := registry.DeleteKey(registry.LOCAL_MACHINE, uninstallKey); err != nil && !errors.Is(err, registry.ErrNotExist) {
		problems = append(problems, i18n.T("uninstall registration"))
	}
	if removeData {
		if err := os.RemoveAll(dataDir); err != nil {
			problems = append(problems, dataDir)
		}
	}

	// only the files the installer put there; the folder goes away only if empty
	for _, f := range []string{CLIExe, `windivert\WinDivert.dll`, `windivert\WinDivert64.sys`, `windivert\LICENSE`, "LEIA-ME.txt", "README.txt", "LICENSE.txt"} {
		removeOrDelay(filepath.Join(dir, f))
	}
	_ = os.Remove(filepath.Join(dir, "windivert"))
	if removeData && portableData {
		_ = os.RemoveAll(filepath.Join(dir, "data"))
	}
	deleteSelfLater(exe, dir)

	msg := i18n.T("vpn_specify was uninstalled.")
	if !removeData {
		msg += "\n\n" + i18n.T("Your data was kept in {0}.", dataDir)
	}
	if len(problems) > 0 {
		msg += "\n\n" + i18n.T("Could not remove:") + "\n" + strings.Join(problems, "\n")
	}
	notify(msg, 0x40)
	if len(problems) > 0 {
		return i18n.Error("incomplete uninstall: {0}", strings.Join(problems, "; "))
	}
	return nil
}

// appShortcuts reads from the settings the shortcuts created by the app (and
// the same-named copies on the desktop).
func appShortcuts(dataDir string) []string {
	b, err := os.ReadFile(filepath.Join(dataDir, "settings.json"))
	if err != nil {
		return nil
	}
	var s struct { // only what matters from settings.json
		Apps []struct {
			Shortcut string `json:"shortcut"`
		} `json:"apps"`
	}
	if json.Unmarshal(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}), &s) != nil {
		return nil
	}
	var out []string
	for _, a := range s.Apps {
		if a.Shortcut != "" {
			out = append(out, a.Shortcut, filepath.Join(platform.DesktopDir(), filepath.Base(a.Shortcut)))
		}
	}
	return out
}

// removeOrDelay deletes the file or, if it is in use (e.g. the WinDivert
// driver is loaded), schedules the removal for the next restart.
func removeOrDelay(path string) {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return
	}
	p, _ := windows.UTF16PtrFromString(path)
	_ = windows.MoveFileEx(p, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
}

// deleteSelfLater deletes this .exe (and the folder, if empty) after the
// process ends: an executable cannot delete itself.
func deleteSelfLater(exe, dir string) {
	script := fmt.Sprintf(`ping -n 3 127.0.0.1 >nul & del /f /q "%s" & rmdir "%s"`, exe, dir)
	cmd := exec.Command(filepath.Join(os.Getenv("WINDIR"), "System32", "cmd.exe"))
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /c "` + script + `"`, HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	cmd.Dir = os.TempDir()
	if err := cmd.Start(); err != nil {
		removeOrDelay(exe)
		return
	}
	_ = cmd.Process.Release()
}
