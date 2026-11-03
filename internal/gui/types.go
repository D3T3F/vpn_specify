package gui

import (
	_ "embed"
	"strings"

	"vpn_specify/internal/brand"
	"vpn_specify/internal/engine"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/store"
)

//go:embed web/index.html
var uiHTML string

// Types and helpers used by the window on Windows (WebView2) and on Linux
// (locally served page).

type uiState struct {
	Status   engine.Status  `json:"status"`
	Settings store.Settings `json:"settings"`
	Notice   string         `json:"notice"`
	DataDir  string         `json:"dataDir"`
	Watching []string       `json:"watching"`
	Update   *updateInfo    `json:"update"`
}

type pickedApp struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Args    string   `json:"args"`
	WorkDir string   `json:"workDir"`
	Icon    string   `json:"icon"`
	Capture []string `json:"capture"`
}

// pageHTML builds the page: logo, platform ("windows" or "linux", which
// adjusts texts and options), the translation runtime (internal/i18n) and,
// on Linux, the script that binds the vs* functions to the local server (on
// Windows they come from WebView2).
func pageHTML(platform, bridge string) string {
	return strings.NewReplacer(
		"{{LOGO}}", brand.LogoDataURI(),
		"{{PLATFORM}}", platform,
		"{{I18N_SCRIPT}}", i18n.PageScript(),
		"<!--{{BRIDGE}}-->", bridge,
	).Replace(uiHTML)
}
