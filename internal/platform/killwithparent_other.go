//go:build !linux

package platform

import "os/exec"

// KillWithParent: on Windows the link is made after Start, with
// BindChildToUs (Job Object); other systems have no simple equivalent.
func KillWithParent(cmd *exec.Cmd) {}
