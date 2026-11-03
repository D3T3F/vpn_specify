//go:build linux

package platform

import (
	"os/exec"
	"syscall"
)

// KillWithParent makes the child process get SIGTERM if vpn_specify dies
// (the equivalent of the Windows Job Object). Call it before cmd.Start.
func KillWithParent(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM
}
