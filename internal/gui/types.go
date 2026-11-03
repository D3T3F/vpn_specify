package gui

import (
	_ "embed"
	"strings"

	"vpn_specify/internal/brand"
	"vpn_specify/internal/engine"
	"vpn_specify/internal/store"
)

//go:embed web/index.html
var uiHTML string

// Tipos e utilitários usados pela janela no Windows (WebView2) e no Linux
// (página servida localmente).

type uiState struct {
	Status   engine.Status  `json:"status"`
	Settings store.Settings `json:"settings"`
	Notice   string         `json:"notice"`
	DataDir  string         `json:"dataDir"`
	Watching []string       `json:"watching"`
}

type pickedApp struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Args    string   `json:"args"`
	WorkDir string   `json:"workDir"`
	Icon    string   `json:"icon"`
	Capture []string `json:"capture"`
}

// pageHTML monta a página: logotipo, plataforma ("windows" ou "linux", que
// ajusta textos e opções) e, no Linux, o script que liga as funções vs* ao
// servidor local (no Windows elas vêm do WebView2).
func pageHTML(platform, bridge string) string {
	return strings.NewReplacer(
		"{{LOGO}}", brand.LogoDataURI(),
		"{{PLATFORM}}", platform,
		"<!--{{BRIDGE}}-->", bridge,
	).Replace(uiHTML)
}
