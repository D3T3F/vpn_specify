package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	AppName   = "vpn_specify"
	MainExe   = "vpn_specify.exe"
	CLIExe    = "vpn_specify-cli.exe"
	Publisher = "vpn_specify"
	// chave de "Aplicativos instalados" (Painel de Controle / Configurações)
	uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\vpn_specify`
	shortcutName = "vpn_specify.lnk"
)

// Options são as escolhas feitas na tela do instalador.
type Options struct {
	Dir       string `json:"dir"`
	Desktop   bool   `json:"desktop"`
	StartMenu bool   `json:"startMenu"`
}

// DefaultDir é C:\Program Files\vpn_specify.
func DefaultDir() string {
	pf := os.Getenv("ProgramW6432") // Program Files de 64 bits, mesmo num processo de 32
	if pf == "" {
		pf = os.Getenv("ProgramFiles")
	}
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, AppName)
}

// NormalizeDir garante um caminho absoluto terminado em \vpn_specify (quem
// escolhe "D:\Apps" recebe "D:\Apps\vpn_specify").
func NormalizeDir(dir string) (string, error) {
	dir = strings.TrimSpace(strings.Trim(strings.TrimSpace(dir), `"`))
	if dir == "" {
		return "", errors.New("escolha a pasta de instalação")
	}
	if !filepath.IsAbs(dir) || filepath.VolumeName(dir) == "" {
		return "", errors.New(`use um caminho completo, por exemplo C:\Program Files\vpn_specify`)
	}
	dir = filepath.Clean(dir)
	if !strings.EqualFold(filepath.Base(dir), AppName) {
		dir = filepath.Join(dir, AppName)
	}
	return dir, nil
}
