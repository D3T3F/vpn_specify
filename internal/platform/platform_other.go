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

// MessageBox mostra um aviso fora da janela principal: zenity (GNOME),
// kdialog (KDE) ou notify-send; sem nenhum deles, só no terminal.
// flags segue o MessageBox do Windows: 0x10 = erro.
func MessageBox(title, text string, flags uintptr) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
	kind := "--info"
	if flags&0x10 != 0 {
		kind = "--error"
	}
	if p, err := exec.LookPath("zenity"); err == nil {
		_ = exec.Command(p, kind, "--title", title, "--no-markup", "--text", text).Run()
	} else if p, err := exec.LookPath("kdialog"); err == nil {
		if kind == "--info" {
			kind = "--msgbox"
		}
		_ = exec.Command(p, "--title", title, kind, text).Run()
	} else if p, err := exec.LookPath("notify-send"); err == nil {
		_ = exec.Command(p, "-a", title, title, text).Run()
	}
}
