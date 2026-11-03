//go:build !unix

package platform

import (
	"os"
	"os/exec"
)

// SudoUser só existe em sistemas Unix; no Windows InvokingUser é sempre nil
// (lá os apps são abertos sem elevação por StartUnelevated).
type SudoUser struct{ Home string }

func InvokingUser() *SudoUser { return nil }

func (u *SudoUser) MkdirAllOwned(dir string, perm os.FileMode) error { return os.MkdirAll(dir, perm) }
func (u *SudoUser) Chown(path string) error                          { return nil }
func (u *SudoUser) Apply(cmd *exec.Cmd)                              {}
