//go:build linux || darwin

package gui

import (
	"log"
	"net/url"
	"os"
	"path/filepath"

	vpnspecify "vpn_specify"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/netcheck"
	"vpn_specify/internal/store"
	"vpn_specify/internal/version"
)

// api: the vs* functions the page calls (the same as on Windows, in
// window_windows.go), plus vsListApps and vsImportConfigData, Unix only (Linux
// and macOS). The per-system parts are in desktop_linux.go and desktop_darwin.go.
func (a *agent) api() map[string]any {
	m := a.baseAPI()
	for k, f := range a.upd.api() {
		m[k] = f
	}
	for k, f := range newGateFinder(a.store).api() {
		m[k] = f
	}
	for k, f := range newRiseupFinder(a.store).api() {
		m[k] = f
	}
	return m
}

func (a *agent) baseAPI() map[string]any {
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
			return uiState{Status: a.eng.Status(), Settings: a.store.Snapshot(), Notice: n, DataDir: a.store.Dir, Watching: watching, Update: a.upd.snapshot()}
		},
		"vsLog": func(since int) map[string]any {
			lines, next := a.logs.since(since)
			return map[string]any{"lines": lines, "next": next}
		},
		// On Unix the page uses the browser's own file picker and sends the
		// content (the browser does not reveal the file path).
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
			log.Printf("configuration imported: %s (%s)", c.Name, c.Kind)
			return &c, nil
		},
		"vsRemoveConfig": func(name string) error {
			if a.configInUse(name) {
				return i18n.Error("disconnect this VPN before removing it")
			}
			return a.store.RemoveConfig(name)
		},
		"vsConfigText": func(name string) (string, error) { return a.store.ConfigText(name) },
		"vsUpdateConfig": func(name, newName, text string) (*store.VPNConfig, error) {
			if a.configInUse(name) {
				return nil, i18n.Error("disconnect this VPN before editing it")
			}
			c, err := a.store.UpdateConfig(name, newName, text)
			if err != nil {
				return nil, err
			}
			if c.Name != name {
				log.Printf("VPN renamed: %s -> %s", name, c.Name)
			} else {
				log.Printf("VPN %s changed", name)
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
		// vsListApps: the apps of the applications menu (.desktop files).
		"vsListApps": func() []installedApp {
			apps := listInstalledApps()
			for _, x := range apps {
				a.ui.allowIcon(x.IconURL)
			}
			return apps
		},
		// vsPickApp: choose a program in the file picker (see pickProgram).
		"vsPickApp": func() (*pickedApp, error) { return pickProgram() },
		"vsSaveApp": func(app store.AppEntry) (store.AppEntry, error) {
			app.Mode, app.Capture = "proxy", nil // no full capture outside Windows
			if app.Path != "" && !filepath.IsAbs(app.Path) {
				return app, i18n.Error("enter the full path of the program (e.g. /usr/bin/firefox)")
			}
			saved, err := a.store.SaveApp(app)
			if err != nil {
				return saved, err
			}
			return saved, syncAutostartAfterSave(a.store)
		},
		"vsRemoveApp": func(id string) error {
			app, err := a.store.RemoveApp(id)
			if err == nil && app.Shortcut != "" {
				removeShortcut(app)
			}
			if err == nil && app.Autostart {
				return syncAutostartAfterSave(a.store)
			}
			return err
		},
		"vsLaunchApp": func(id string) error {
			app, ok := a.store.App(id)
			if !ok {
				return store.ErrAppNotFound
			}
			go func() {
				if err := a.launchApp(app); err != nil {
					log.Printf("error opening %s: %v", app.Name, err)
					a.setNotice(i18n.T("Could not open {0}: {1}", app.Name, err))
				}
			}()
			return nil
		},
		"vsCreateShortcut": func(id, name string, desktop bool) (string, error) {
			return createShortcut(a.store, id, name, desktop)
		},
		"vsCreateMainShortcut": func() (string, error) { return createMainShortcut() },
		"vsSetAutoDisconnect":  func(v bool) error { return a.store.SetAutoDisconnect(v) },
		// vsSetLanguage switches the interface language and saves the choice.
		"vsSetLanguage": func(code string) (i18n.PageState, error) {
			if err := i18n.Save(code); err != nil {
				return i18n.PageState{}, err
			}
			return i18n.State(), nil
		},
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
