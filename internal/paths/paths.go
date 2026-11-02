// Package paths decide onde ficam os dados do vpn_specify.
package paths

import (
	"os"
	"path/filepath"
)

// DataDir devolve a pasta de dados:
//   - modo portátil: <pasta do exe>\data, se essa pasta já existir (ou houver
//     um arquivo "portable" ao lado do exe);
//   - instalado: %LOCALAPPDATA%\vpn_specify (Arquivos de Programas não é
//     gravável sem Administrador).
func DataDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "data"
	}
	dir := filepath.Dir(exe)
	portable := filepath.Join(dir, "data")
	if fi, err := os.Stat(portable); err == nil && fi.IsDir() {
		return portable
	}
	if _, err := os.Stat(filepath.Join(dir, "portable")); err == nil {
		return portable
	}
	if base, err := os.UserCacheDir(); err == nil { // %LOCALAPPDATA%
		return filepath.Join(base, "vpn_specify")
	}
	return portable
}
