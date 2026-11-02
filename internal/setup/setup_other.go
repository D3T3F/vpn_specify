//go:build !windows

package setup

import (
	"errors"
	"io/fs"
)

var errWindowsOnly = errors.New("o instalador só existe no Windows")

func RunInstaller(payload fs.FS) error { return errWindowsOnly }
func Uninstall(args []string) error    { return errWindowsOnly }

func Install(payload fs.FS, o Options, progress func(string)) (string, error) {
	return "", errWindowsOnly
}

func InstalledDir() string { return "" }
