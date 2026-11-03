//go:build !unix

package platform

import (
	"os"
	"os/exec"
)

// SudoUser only exists on Unix systems; on Windows InvokingUser is always nil
// (apps are started unelevated with StartUnelevated there).
type SudoUser struct{ Home string }

func InvokingUser() *SudoUser { return nil }

func (u *SudoUser) MkdirAllOwned(dir string, perm os.FileMode) error { return os.MkdirAll(dir, perm) }
func (u *SudoUser) Chown(path string) error                          { return nil }
func (u *SudoUser) Apply(cmd *exec.Cmd)                              {}
