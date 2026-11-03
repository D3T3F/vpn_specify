//go:build windows

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"vpn_specify/internal/setup"
)

// Prepare extrai o vpn_specify-setup.exe do pacote. O Run inicia o
// instalador, que espera este processo terminar e então:
//   - /update: atualiza a instalação (mesma pasta, mantém os atalhos que existem);
//   - /extract=<pasta>: cópia portátil, só troca os arquivos da pasta.
//
// Com relaunch, o instalador abre o vpn_specify de novo no fim.
func Prepare(archive, ver string, relaunch bool) (*Plan, error) {
	setupExe := filepath.Join(filepath.Dir(archive), "vpn_specify-setup.exe")
	if err := extractZipFile(archive, "vpn_specify-setup.exe", setupExe); err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exeDir := filepath.Dir(exe)
	args := []string{"/wait=" + strconv.Itoa(os.Getpid())}
	desc := "atualizar a instalação em " + exeDir
	if inst := setup.InstalledDir(); inst != "" && strings.EqualFold(filepath.Clean(inst), filepath.Clean(exeDir)) {
		args = append(args, "/update")
	} else {
		args = append(args, "/extract="+exeDir)
		desc = "atualizar a cópia portátil em " + exeDir
	}
	if relaunch {
		args = append(args, "/run")
	}
	return &Plan{Description: desc, Run: func() error {
		return exec.Command(setupExe, args...).Start()
	}}, nil
}
