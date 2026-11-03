//go:build windows

package setup

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"vpn_specify/internal/platform"
	"vpn_specify/internal/version"
)

// RunningInstances lista os PIDs do vpn_specify em execução a partir da
// pasta dir (exceto este processo). Cópias em outras pastas não atrapalham.
func RunningInstances(dir string) []uint32 {
	var out []uint32
	for pid := range platform.RunningPIDs(map[string]bool{MainExe: true, CLIExe: true}) {
		if int(pid) == os.Getpid() {
			continue
		}
		p := platform.ProcessPath(pid)
		if p == "" || strings.EqualFold(filepath.Dir(p), filepath.Clean(dir)) {
			out = append(out, pid) // sem acesso ao caminho: melhor assumir que é desta pasta
		}
	}
	return out
}

var errRunning = errors.New("o vpn_specify está aberto. Abra a janela dele, clique em Desconectar e feche-a " +
	"(os apps abertos pela VPN também precisam estar fechados); depois tente de novo")

// Install copia os arquivos de payload (raiz = conteúdo da pasta de
// instalação), cria os atalhos e registra a desinstalação. progress recebe
// mensagens curtas para a tela.
func Install(payload fs.FS, o Options, progress func(string)) (string, error) {
	dir, err := NormalizeDir(o.Dir)
	if err != nil {
		return "", err
	}
	if len(RunningInstances(dir)) > 0 {
		return "", errRunning
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("não consegui criar %s: %w", dir, err)
	}

	var total int64
	err = fs.WalkDir(payload, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		progress("Copiando " + p + "…")
		n, err := copyFile(payload, p, filepath.Join(dir, filepath.FromSlash(p)))
		total += n
		return err
	})
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, MainExe)); err != nil {
		return "", errors.New("o instalador não contém o vpn_specify.exe (pacote incompleto)")
	}

	exe := filepath.Join(dir, MainExe)
	desc := "vpn_specify: VPN só para os apps escolhidos"
	if o.StartMenu {
		progress("Criando atalho no Menu Iniciar…")
		if err := platform.CreateShortcut(filepath.Join(platform.StartMenuDir(), shortcutName), exe, "", dir, exe+",0", desc); err != nil {
			return "", fmt.Errorf("atalho do Menu Iniciar: %w", err)
		}
	}
	if o.Desktop {
		progress("Criando atalho na Área de Trabalho…")
		if err := platform.CreateShortcut(filepath.Join(platform.DesktopDir(), shortcutName), exe, "", dir, exe+",0", desc); err != nil {
			return "", fmt.Errorf("atalho da Área de Trabalho: %w", err)
		}
	}

	progress("Registrando em Aplicativos instalados…")
	if err := register(dir, total); err != nil {
		return "", fmt.Errorf("registro da desinstalação: %w", err)
	}
	progress("Pronto.")
	return dir, nil
}

// copyFile grava num arquivo temporário e troca no fim, para não deixar um
// .exe pela metade se algo falhar.
func copyFile(src fs.FS, name, dst string) (int64, error) {
	in, err := src.Open(name)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	tmp := dst + ".novo"
	out, err := os.Create(tmp)
	if err != nil {
		return 0, fmt.Errorf("não consegui gravar %s: %w", dst, err)
	}
	n, err := io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return 0, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return 0, fmt.Errorf("não consegui substituir %s (está em uso?): %w", dst, err)
	}
	return n, nil
}

func register(dir string, size int64) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, uninstallKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	exe := filepath.Join(dir, MainExe)
	for name, v := range map[string]string{
		"DisplayName":     AppName,
		"DisplayVersion":  version.Version,
		"Publisher":       Publisher,
		"InstallLocation": dir,
		"DisplayIcon":     exe + ",0",
		"UninstallString": `"` + exe + `" uninstall`,
	} {
		if err := k.SetStringValue(name, v); err != nil {
			return err
		}
	}
	for name, v := range map[string]uint32{"NoModify": 1, "NoRepair": 1, "EstimatedSize": uint32(size / 1024)} {
		if err := k.SetDWordValue(name, v); err != nil {
			return err
		}
	}
	return nil
}

// InstalledDir devolve a pasta registrada por uma instalação anterior.
func InstalledDir() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstallKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("InstallLocation")
	return v
}

// UpdateOptions: as opções da instalação atual, para atualizar sem mudar
// nada (mesma pasta; só recria os atalhos que ainda existem).
func UpdateOptions() Options {
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	return Options{
		Dir:       InstalledDir(),
		Desktop:   exists(filepath.Join(platform.DesktopDir(), shortcutName)),
		StartMenu: exists(filepath.Join(platform.StartMenuDir(), shortcutName)),
	}
}

// Extract só copia os arquivos para dir (atualização de cópia portátil:
// sem registro nem atalhos).
func Extract(payload fs.FS, dir string, progress func(string)) error {
	if len(RunningInstances(dir)) > 0 {
		return errRunning
	}
	return fs.WalkDir(payload, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		progress("Copiando " + p + "…")
		_, err = copyFile(payload, p, filepath.Join(dir, filepath.FromSlash(p)))
		return err
	})
}

// WaitProcessExit espera o processo pid terminar (usado na atualização:
// o vpn_specify abre o instalador e fecha em seguida).
func WaitProcessExit(pid uint32, timeout time.Duration) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return // já terminou
	}
	defer windows.CloseHandle(h)
	_, _ = windows.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
}
