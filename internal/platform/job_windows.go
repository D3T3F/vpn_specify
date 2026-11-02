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

// NewStopEvent cria um evento nomeado para o "--service <evento> 0" do
// OpenVPN: quando sinalizado, o openvpn encerra de forma limpa.
func NewStopEvent() (string, func(), error) {
	name := fmt.Sprintf("vpn_specify_stop_%d_%d", os.Getpid(), rand.Uint32())
	p, _ := windows.UTF16PtrFromString(name)
	h, err := windows.CreateEvent(nil, 1, 0, p)
	if err != nil {
		return "", nil, err
	}
	return name, func() { _ = windows.SetEvent(h) }, nil
}

// HideConsole impede que o processo filho abra uma janela de console.
func HideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

// BindChildToUs coloca o processo filho num Job Object com
// KILL_ON_JOB_CLOSE: se o vpn_specify morrer (fechar a janela, Gerenciador
// de Tarefas), o Windows encerra o openvpn junto, sem deixar a VPN órfã.
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
	// O handle do job fica aberto de propósito até o fim do processo.
	return nil
}
