//go:build linux

package gui

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	vpnspecify "vpn_specify"
	"vpn_specify/internal/netcheck"
	"vpn_specify/internal/store"
	"vpn_specify/internal/version"
)

// api: as funções vs* que a página chama (as mesmas do Windows, em
// window_windows.go), mais vsListApps e vsImportConfigData, só do Linux.
func (a *agent) api() map[string]any {
	return map[string]any{
		"vsState": func() uiState {
			a.mu.Lock()
			n := a.notice
			a.notice = ""
			var watching []string
			for k := range a.watch {
				watching = append(watching, k)
			}
			a.mu.Unlock()
			return uiState{Status: a.eng.Status(), Settings: a.store.Snapshot(), Notice: n, DataDir: a.store.Dir, Watching: watching}
		},
		"vsLog": func(since int) map[string]any {
			lines, next := a.logs.since(since)
			return map[string]any{"lines": lines, "next": next}
		},
		// No Linux a página usa o seletor de arquivos do próprio navegador e
		// manda o conteúdo (o navegador não revela o caminho do arquivo).
		"vsImportConfigData": func(fileName, text string) (*store.VPNConfig, error) {
			base := store.SafeName(filepath.Base(fileName), "_")
			if base == "" || base == "." {
				base = "vpn.conf"
			}
			dir, err := os.MkdirTemp("", "vpn_specify-import-")
			if err != nil {
				return nil, err
			}
			defer os.RemoveAll(dir)
			p := filepath.Join(dir, base)
			if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
				return nil, err
			}
			c, err := a.store.ImportConfig(p)
			if err != nil {
				return nil, err
			}
			log.Printf("configuração importada: %s (%s)", c.Name, c.Kind)
			return &c, nil
		},
		"vsRemoveConfig": func(name string) error {
			if a.configInUse(name) {
				return errors.New("desconecte esta VPN antes de removê-la")
			}
			return a.store.RemoveConfig(name)
		},
		"vsConfigText": func(name string) (string, error) { return a.store.ConfigText(name) },
		"vsUpdateConfig": func(name, newName, text string) (*store.VPNConfig, error) {
			if a.configInUse(name) {
				return nil, errors.New("desconecte esta VPN antes de editá-la")
			}
			c, err := a.store.UpdateConfig(name, newName, text)
			if err != nil {
				return nil, err
			}
			if c.Name != name {
				log.Printf("VPN renomeada: %s -> %s", name, c.Name)
			} else {
				log.Printf("VPN %s alterada", name)
			}
			return &c, nil
		},
		"vsSetCredentials": func(name, user, pass string) error { return a.store.SetCredentials(name, user, pass) },
		"vsConnect": func(name string) error {
			path, opts, err := a.store.ConfigFor(name)
			if err != nil {
				return err
			}
			go a.eng.Connect(name, path, opts, nil)
			return nil
		},
		"vsDisconnect": func() {
			go func() {
				a.eng.Disconnect()
				a.mu.Lock()
				a.watch, a.pgids = map[string]bool{}, map[int]bool{}
				a.mu.Unlock()
			}()
		},
		// vsListApps: os apps do menu de aplicativos (arquivos .desktop).
		"vsListApps": func() []installedApp {
			apps := listInstalledApps()
			for _, x := range apps {
				a.ui.allowIcon(x.IconURL)
			}
			return apps
		},
		// vsPickApp: escolher um programa ou um .desktop no seletor de arquivos.
		"vsPickApp": func() (*pickedApp, error) {
			p, ok, err := pickFile("Escolha o programa ou atalho", applicationsDir(), "Programas e atalhos", nil)
			if err != nil || !ok {
				return nil, err
			}
			if strings.HasSuffix(p, ".desktop") {
				return appFromDesktopFile(p)
			}
			if fi, err := os.Stat(p); err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
				return nil, fmt.Errorf("%s não é um programa executável", filepath.Base(p))
			}
			return &pickedApp{Name: filepath.Base(p), Path: p, Capture: []string{filepath.Base(p)}}, nil
		},
		"vsSaveApp": func(app store.AppEntry) (store.AppEntry, error) {
			app.Mode, app.Capture = "proxy", nil // sem captura completa no Linux
			if app.Path != "" && !filepath.IsAbs(app.Path) {
				return app, errors.New("informe o caminho completo do programa (ex.: /usr/bin/firefox)")
			}
			return a.store.SaveApp(app)
		},
		"vsRemoveApp": func(id string) error {
			app, err := a.store.RemoveApp(id)
			if err == nil && app.Shortcut != "" {
				_ = os.Remove(app.Shortcut)
				_ = os.Remove(filepath.Join(desktopDir(), filepath.Base(app.Shortcut)))
			}
			return err
		},
		"vsLaunchApp": func(id string) error {
			app, ok := a.store.App(id)
			if !ok {
				return errors.New("app não encontrado")
			}
			go func() {
				if err := a.launchApp(app); err != nil {
					log.Printf("erro ao abrir %s: %v", app.Name, err)
					a.setNotice(fmt.Sprintf("Não consegui abrir %s: %v", app.Name, err))
				}
			}()
			return nil
		},
		"vsCreateShortcut": func(id, name string, desktop bool) (string, error) {
			app, ok := a.store.App(id)
			if !ok {
				return "", errors.New("app não encontrado")
			}
			name = store.SafeName(name, "")
			if name == "" {
				return "", errors.New("nome do atalho vazio")
			}
			exe, err := os.Executable()
			if err != nil {
				return "", err
			}
			file := "vpn_specify-" + strings.ReplaceAll(name, " ", "_") + ".desktop"
			lnk := filepath.Join(applicationsDir(), file)
			desc := fmt.Sprintf("Abre %s pela VPN %s (vpn_specify)", app.Name, app.Config)
			execLine := desktopQuote(exe) + " launch " + desktopQuote(app.ID)
			if err := writeDesktopFile(lnk, name, desc, execLine, app.Icon); err != nil {
				return "", err
			}
			if desktop {
				if err := writeDesktopFile(filepath.Join(desktopDir(), file), name, desc, execLine, app.Icon); err != nil {
					return "", err
				}
			}
			if app.Shortcut != "" && app.Shortcut != lnk {
				_ = os.Remove(app.Shortcut)
				_ = os.Remove(filepath.Join(desktopDir(), filepath.Base(app.Shortcut)))
			}
			_ = a.store.SetShortcut(app.ID, lnk)
			log.Printf("atalho criado: %s", lnk)
			return lnk, nil
		},
		"vsCreateMainShortcut": func() (string, error) {
			exe, err := os.Executable()
			if err != nil {
				return "", err
			}
			lnk := filepath.Join(applicationsDir(), "vpn_specify.desktop")
			return lnk, writeDesktopFile(lnk, "vpn_specify", "VPN só para os apps escolhidos", desktopQuote(exe), logoIconPath())
		},
		"vsSetAutoDisconnect": func(v bool) error { return a.store.SetAutoDisconnect(v) },
		"vsAbout": func() map[string]string {
			return map[string]string{"version": version.Version, "license": vpnspecify.License}
		},
		"vsOpenDataDir": func() error { return openWithDefault(a.store.Dir) },
		"vsCheckIP": func() map[string]string {
			out := map[string]string{}
			d, derr := netcheck.FetchIP("https://api.ipify.org", nil)
			out["direct"] = netcheck.Describe(d, derr)
			v, verr := netcheck.FetchIP("https://api.ipify.org", &url.URL{Scheme: "http", Host: a.eng.HTTPAddr})
			out["vpn"] = netcheck.Describe(v, verr)
			u, uerr := netcheck.UDPViaSocks(a.eng.SocksAddr)
			out["vpnUdp"] = netcheck.Describe(u, uerr)
			return out
		},
	}
}
