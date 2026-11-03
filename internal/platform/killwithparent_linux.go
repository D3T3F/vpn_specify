//go:build linux

package platform

import (
	"os/exec"
	"syscall"
)

// KillWithParent faz o processo filho receber SIGTERM se o vpn_specify
// morrer (equivalente ao Job Object do Windows). Chame antes de cmd.Start.
func KillWithParent(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM
}
