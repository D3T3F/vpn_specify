//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---------- Elevation ----------

// IsElevated reports whether the process runs as Administrator.
func IsElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

// ElevateSelf reopens this executable as Administrator (UAC prompt).
func ElevateSelf(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	var quoted []string
	for _, a := range args {
		quoted = append(quoted, windows.EscapeArg(a))
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(exe))
	return windows.ShellExecute(0, verb, file, params, dir, windows.SW_SHOWNORMAL)
}

// ---------- Starting apps as the normal user (not Administrator) ----------

// StartUnelevated starts the program with Explorer's token, that is, as the
// normal user. Otherwise Discord would open as Administrator.
func StartUnelevated(path, rawArgs, dir string, env []string) (uint32, error) {
	cmdline := windows.EscapeArg(path)
	if strings.TrimSpace(rawArgs) != "" {
		cmdline += " " + rawArgs
	}
	if dir == "" {
		dir = filepath.Dir(path)
	}
	if !IsElevated() {
		return StartNormal(path, cmdline, dir, env)
	}
	hwnd, _, _ := procGetShellWindow.Call()
	if hwnd == 0 {
		return StartNormal(path, cmdline, dir, env)
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(windows.HWND(hwnd), &pid); err != nil {
		return 0, err
	}
	ph, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, fmt.Errorf("opening explorer: %w", err)
	}
	defer windows.CloseHandle(ph)
	var tok windows.Token
	if err := windows.OpenProcessToken(ph, windows.TOKEN_DUPLICATE, &tok); err != nil {
		return 0, fmt.Errorf("explorer token: %w", err)
	}
	defer tok.Close()
	var dup windows.Token
	access := uint32(windows.TOKEN_QUERY | windows.TOKEN_DUPLICATE | windows.TOKEN_ASSIGN_PRIMARY | windows.TOKEN_ADJUST_DEFAULT | windows.TOKEN_ADJUST_SESSIONID)
	if err := windows.DuplicateTokenEx(tok, access, nil, windows.SecurityImpersonation, windows.TokenPrimary, &dup); err != nil {
		return 0, err
	}
	defer dup.Close()

	app, _ := windows.UTF16PtrFromString(path)
	cl, _ := windows.UTF16FromString(cmdline)
	d, _ := windows.UTF16PtrFromString(dir)
	var envPtr *uint16
	flags := uint32(0)
	if env != nil {
		block := envBlock(env)
		envPtr = &block[0]
		flags |= windows.CREATE_UNICODE_ENVIRONMENT
	}
	si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var pi windows.ProcessInformation
	r, _, callErr := procCreateProcessWithTokenW.Call(uintptr(dup), 0,
		uintptr(unsafe.Pointer(app)), uintptr(unsafe.Pointer(&cl[0])), uintptr(flags),
		uintptr(unsafe.Pointer(envPtr)), uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi)))
	if r == 0 {
		return 0, fmt.Errorf("CreateProcessWithTokenW: %w", callErr)
	}
	windows.CloseHandle(pi.Thread)
	windows.CloseHandle(pi.Process)
	return pi.ProcessId, nil
}

func StartNormal(path, cmdline, dir string, env []string) (uint32, error) {
	cmd := exec.Command(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: cmdline}
	cmd.Dir = dir
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := uint32(cmd.Process.Pid)
	_ = cmd.Process.Release()
	return pid, nil
}

func envBlock(env []string) []uint16 {
	var b []uint16
	for _, kv := range env {
		b = append(b, utf16.Encode([]rune(kv))...)
		b = append(b, 0)
	}
	return append(b, 0)
}
