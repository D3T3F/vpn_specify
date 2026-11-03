//go:build linux

package update

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Prepare troca o executável pelo do pacote já agora (o Linux permite trocar
// um programa em execução) e confere que o novo roda neste sistema. Se a
// pasta exigir root (ex.: /usr/local/bin), pede a senha com pkexec. O Run
// reabre o vpn_specify (relaunch) no lugar deste processo.
func Prepare(archive, ver string, relaunch bool) (*Plan, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return prepareAt(archive, ver, relaunch, exe)
}

func prepareAt(archive, ver string, relaunch bool, exe string) (*Plan, error) {
	fresh := filepath.Join(filepath.Dir(archive), "vpn_specify")
	if err := extractTarFile(archive, "vpn_specify", fresh); err != nil {
		return nil, err
	}
	out, err := exec.Command(fresh, "version").Output()
	if got := strings.TrimSpace(string(out)); err != nil || got != ver {
		return nil, fmt.Errorf("o executável baixado não rodou neste sistema (versão %q, erro %v)", got, err)
	}
	if err := replaceFile(fresh, exe); err != nil {
		return nil, err
	}
	return &Plan{Description: "atualizado em " + exe, Run: func() error {
		if !relaunch {
			return nil
		}
		return syscall.Exec(exe, []string{exe}, os.Environ())
	}}, nil
}

// replaceFile troca dst por src de forma atômica (cópia ao lado + rename);
// sem permissão de escrita na pasta, usa "pkexec install".
func replaceFile(src, dst string) error {
	staged := filepath.Join(filepath.Dir(dst), ".vpn_specify.novo")
	if err := copyFile(src, staged); err == nil {
		if err := os.Rename(staged, dst); err == nil {
			return nil
		}
		os.Remove(staged)
	} else if !errors.Is(err, os.ErrPermission) {
		return err
	}
	pk, err := exec.LookPath("pkexec")
	if err != nil {
		return fmt.Errorf("sem permissão para gravar em %s; atualize com: sudo vpn_specify update", filepath.Dir(dst))
	}
	inst, err := exec.LookPath("install")
	if err != nil {
		return err
	}
	if out, err := exec.Command(pk, inst, "-m", "0755", src, dst).CombinedOutput(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && (ee.ExitCode() == 126 || ee.ExitCode() == 127) {
			return errors.New("permissão de administrador negada; a atualização não foi instalada")
		}
		return fmt.Errorf("não consegui substituir %s: %v %s", dst, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}
