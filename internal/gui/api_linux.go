//go:build linux

package gui

import (
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	vpnspecify "vpn_specify"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/netcheck"
	"vpn_specify/internal/store"
	"vpn_specify/internal/version"
)

// api: the vs* functions the page calls (the same as on Windows, in
// window_windows.go), plus vsListApps and vsImportConfigData, Linux only.
func (a *agent) api() map[string]any {
	m := a.baseAPI()
	for k, f := range a.upd.api() {
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
		// On Linux the page uses the browser's own file picker and sends the
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
		// vsPickApp: choose a program or a .desktop file in the file picker.
		"vsPickApp": func() (*pickedApp, error) {
			p, ok, err := pickFile(i18n.T("Choose the program or shortcut"), applicationsDir(), i18n.T("Programs and shortcuts"), nil)
			if err != nil || !ok {
				return nil, err
			}
			if strings.HasSuffix(p, ".desktop") {
				return appFromDesktopFile(p)
			}
			if fi, err := os.Stat(p); err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
				return nil, i18n.Error("{0} is not an executable program", filepath.Base(p))
			}
			return &pickedApp{Name: filepath.Base(p), Path: p, Capture: []string{filepath.Base(p)}}, nil
		},
		"vsSaveApp": func(app store.AppEntry) (store.AppEntry, error) {
			app.Mode, app.Capture = "proxy", nil // no full capture on Linux
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
				_ = os.Remove(app.Shortcut)
				_ = os.Remove(filepath.Join(desktopDir(), filepath.Base(app.Shortcut)))
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
			app, ok := a.store.App(id)
			if !ok {
				return "", store.ErrAppNotFound
			}
			name = store.SafeName(name, "")
			if name == "" {
				return "", i18n.Error("empty shortcut name")
			}
			exe, err := os.Executable()
			if err != nil {
				return "", err
			}
			file := "vpn_specify-" + strings.ReplaceAll(name, " ", "_") + ".desktop"
			lnk := filepath.Join(applicationsDir(), file)
			desc := i18n.T("Opens {0} through the VPN {1} (vpn_specify)", app.Name, app.Config)
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
			log.Printf("shortcut created: %s", lnk)
			return lnk, nil
		},
		"vsCreateMainShortcut": func() (string, error) {
			exe, err := os.Executable()
			if err != nil {
				return "", err
			}
			lnk := filepath.Join(applicationsDir(), "vpn_specify.desktop")
			return lnk, writeDesktopFile(lnk, "vpn_specify", i18n.T("VPN only for the apps you choose"), desktopQuote(exe), logoIconPath())
		},
		"vsSetAutoDisconnect": func(v bool) error { return a.store.SetAutoDisconnect(v) },
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
