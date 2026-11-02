// Package brand guarda o logotipo do vpn_specify, usado nas páginas da
// interface e do instalador.
package brand

import (
	_ "embed"
	"encoding/base64"
)

//go:embed logo.png
var logoPNG []byte

// LogoDataURI devolve o logotipo como data URI (para <img src> e favicon).
func LogoDataURI() string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(logoPNG)
}
