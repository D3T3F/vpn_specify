//go:build windows

package platform

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NewStopEvent creates a named event for OpenVPN's "--service <event> 0":
// when signaled, openvpn exits cleanly.
func NewStopEvent() (string, func(), error) {
	name := fmt.Sprintf("vpn_specify_stop_%d_%d", os.Getpid(), rand.Uint32())
	p, _ := windows.UTF16PtrFromString(name)
	h, err := windows.CreateEvent(nil, 1, 0, p)
	if err != nil {
		return "", nil, err
	}
	return name, func() { _ = windows.SetEvent(h) }, nil
}

// HideConsole keeps the child process from opening a console window.
func HideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

// BindChildToUs puts the child process in a Job Object with
// KILL_ON_JOB_CLOSE: if vpn_specify dies (window closed, Task Manager),
// Windows ends openvpn too, so the VPN is never left orphaned.
func BindChildToUs(p *os.Process) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return err
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return err
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		windows.CloseHandle(job)
		return err
	}
	// The job handle stays open on purpose until the process ends.
	return nil
}
