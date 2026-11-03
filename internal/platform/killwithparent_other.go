//go:build !linux

package platform

import "os/exec"

// KillWithParent: no Windows o vínculo é feito depois do Start, com
// BindChildToUs (Job Object); nos outros sistemas não há equivalente simples.
func KillWithParent(cmd *exec.Cmd) {}
