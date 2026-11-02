//go:build !windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

var errWindowsOnly = errors.New("só no Windows")

func IsElevated() bool { return os.Geteuid() == 0 }

func BindChildToUs(p *os.Process) error { return nil }

func NewStopEvent() (string, func(), error) { return "", nil, errWindowsOnly }

func HideConsole(cmd *exec.Cmd) {}

func ElevateSelf(args []string) error { return errWindowsOnly }

func MessageBox(title, text string, flags uintptr) { fmt.Fprintf(os.Stderr, "%s: %s\n", title, text) }

func Protect(s string) (string, error)     { return "", errWindowsOnly }
func Unprotect(enc string) (string, error) { return "", errWindowsOnly }
