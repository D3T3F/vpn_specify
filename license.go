// Package vpnspecify expõe arquivos da raiz do projeto que vão embutidos nos
// executáveis (go:embed só alcança a pasta do próprio pacote).
package vpnspecify

import _ "embed"

// License é o texto do arquivo LICENSE (MIT).
//
//go:embed LICENSE
var License string
