//go:build windows

package gui

import (
	_ "embed"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	vpnspecify "vpn_specify"
	"vpn_specify/internal/engine"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/version"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"vpn_specify/internal/netcheck"
	"vpn_specify/internal/platform"
	"vpn_specify/internal/store"
)

// ---------- window ----------

var processStartRe = regexp.MustCompile(`(?i)--processStart\s+"?([^"\s]+)"?`)

func (a *agent) runWindow(autoClose bool) {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  filepath.Join(a.store.Dir, "webview2"),
		WindowOptions: webview2.WindowOptions{
			Title: "vpn_specify", Width: 1000, Height: 760, Center: true,
			IconId: 1, // embedded icon (assets\resource.rc.tmpl)
		},
	})
	if w == nil {
		platform.MessageBox("vpn_specify", i18n.T("Could not open the window: the Microsoft Edge WebView2 Runtime is not installed.\nDownload it from {0}", "https://developer.microsoft.com/microsoft-edge/webview2/"), 0x10)
		return
	}
	hwnd := uintptr(w.Window())
	platform.SetWindowIcons(hwnd)
	w.SetSize(820, 600, webview2.HintMin)

	must := func(err error) {
		if err != nil {
			log.Printf("bind: %v", err)
		}
	}
	must(w.Bind("vsState", func() uiState {
		a.mu.Lock()
		n := a.notice
		a.notice = ""
		var watching []string
		for k := range a.watch {
			watching = append(watching, k)
		}
		a.mu.Unlock()
		return uiState{Status: a.eng.Status(), Settings: a.store.Snapshot(), Notice: n, DataDir: a.store.Dir, Watching: watching, Update: a.upd.snapshot()}
	}))
	for name, f := range a.upd.api() {
		must(w.Bind(name, f))
	}
	for name, f := range newGateFinder(a.store).api() {
		must(w.Bind(name, f))
	}
	// vsSetLanguage switches the interface language and saves the choice.
	must(w.Bind("vsSetLanguage", func(code string) (i18n.PageState, error) {
		if err := i18n.Save(code); err != nil {
			return i18n.PageState{}, err
		}
		return i18n.State(), nil
	}))
	must(w.Bind("vsLog", func(since int) map[string]any {
		lines, next := a.logs.since(since)
		return map[string]any{"lines": lines, "next": next}
	}))
	must(w.Bind("vsImportConfig", func() (*store.VPNConfig, error) {
		p, ok := platform.OpenFileDialog(hwnd, i18n.T("Choose the VPN file"), filepath.Join(os.Getenv("USERPROFILE"), "Downloads"), false,
			i18n.T("VPN configuration (*.ovpn, *.conf)"), "*.ovpn;*.conf", i18n.T("All files"), "*.*")
		if !ok {
			return nil, nil
		}
		c, err := a.store.ImportConfig(p)
		if err != nil {
			return nil, err
		}
		log.Printf("configuration imported: %s (%s)", c.Name, c.Kind)
		return &c, nil
	}))
	must(w.Bind("vsRemoveConfig", func(name string) error {
		if a.configInUse(name) {
			return i18n.Error("disconnect this VPN before removing it")
		}
		return a.store.RemoveConfig(name)
	}))
	must(w.Bind("vsConfigText", func(name string) (string, error) { return a.store.ConfigText(name) }))
	must(w.Bind("vsUpdateConfig", func(name, newName, text string) (*store.VPNConfig, error) {
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
	}))
	must(w.Bind("vsSetCredentials", func(name, user, pass string) error { return a.store.SetCredentials(name, user, pass) }))
	must(w.Bind("vsConnect", func(name string) error {
		path, opts, err := a.store.ConfigFor(name)
		if err != nil {
			return err
		}
		go a.eng.Connect(name, path, opts, nil)
		return nil
	}))
	must(w.Bind("vsDisconnect", func() {
		go func() {
			a.eng.Disconnect()
			a.mu.Lock()
			a.watch = map[string]bool{}
			a.mu.Unlock()
		}()
	}))
	must(w.Bind("vsPickApp", func() (*pickedApp, error) {
		p, ok := platform.OpenFileDialog(hwnd, i18n.T("Choose the program or shortcut"), platform.StartMenuDir(), true,
			i18n.T("Programs and shortcuts (*.exe, *.lnk)"), "*.exe;*.lnk", i18n.T("All files"), "*.*")
		if !ok {
			return nil, nil
		}
		pa := &pickedApp{Path: p, Name: strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))}
		if strings.EqualFold(filepath.Ext(p), ".lnk") {
			si, err := platform.ResolveShortcut(p)
			if err != nil {
				return nil, err
			}
			pa.Path, pa.Args, pa.WorkDir, pa.Icon = si.Target, si.Args, si.WorkDir, si.Icon
		}
		// Apps with an updater (Discord, Slack, old Teams...): "Update.exe --processStart Discord.exe"
		exe := strings.ToLower(filepath.Base(pa.Path))
		if m := processStartRe.FindStringSubmatch(pa.Args); m != nil {
			exe = strings.ToLower(m[1])
		}
		pa.Capture = []string{exe}
		if pa.Icon == "" || strings.HasPrefix(pa.Icon, ",") {
			pa.Icon = pa.Path + ",0"
		}
		return pa, nil
	}))
	must(w.Bind("vsSaveApp", func(app store.AppEntry) (store.AppEntry, error) {
		saved, err := a.store.SaveApp(app)
		if err != nil {
			return saved, err
		}
		return saved, syncAutostartAfterSave(a.store)
	}))
	must(w.Bind("vsRemoveApp", func(id string) error {
		app, err := a.store.RemoveApp(id)
		if err == nil && app.Shortcut != "" {
			_ = os.Remove(app.Shortcut)
			_ = os.Remove(filepath.Join(platform.DesktopDir(), filepath.Base(app.Shortcut)))
		}
		if err == nil && app.Autostart {
			return syncAutostartAfterSave(a.store)
		}
		return err
	}))
	must(w.Bind("vsLaunchApp", func(id string) error {
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
	}))
	must(w.Bind("vsCreateShortcut", func(id, name string, desktop bool) (string, error) {
		app, ok := a.store.App(id)
		if !ok {
			return "", store.ErrAppNotFound
		}
		name = store.SafeName(name, "")
		if name == "" {
			return "", i18n.Error("empty shortcut name")
		}
		exe, _ := os.Executable()
		lnk := filepath.Join(platform.StartMenuDir(), name+".lnk")
		desc := i18n.T("Opens {0} through the VPN {1} (vpn_specify)", app.Name, app.Config)
		if err := platform.CreateShortcut(lnk, exe, "launch "+app.ID, filepath.Dir(exe), app.Icon, desc); err != nil {
			return "", err
		}
		if desktop {
			if err := platform.CreateShortcut(filepath.Join(platform.DesktopDir(), name+".lnk"), exe, "launch "+app.ID, filepath.Dir(exe), app.Icon, desc); err != nil {
				return "", err
			}
		}
		if app.Shortcut != "" && !strings.EqualFold(app.Shortcut, lnk) {
			_ = os.Remove(app.Shortcut)
		}
		_ = a.store.SetShortcut(app.ID, lnk)
		log.Printf("shortcut created: %s", lnk)
		return lnk, nil
	}))
	must(w.Bind("vsCreateMainShortcut", func() (string, error) {
		exe, _ := os.Executable()
		lnk := filepath.Join(platform.StartMenuDir(), "vpn_specify.lnk")
		return lnk, platform.CreateShortcut(lnk, exe, "", filepath.Dir(exe), exe+",0", i18n.T("vpn_specify: VPN only for the apps you choose"))
	}))
	must(w.Bind("vsSetAutoDisconnect", func(v bool) error { return a.store.SetAutoDisconnect(v) }))
	must(w.Bind("vsAbout", func() map[string]string {
		return map[string]string{"version": version.Version, "license": vpnspecify.License}
	}))
	must(w.Bind("vsOpenDataDir", func() {
		_, _ = platform.StartUnelevated(filepath.Join(os.Getenv("WINDIR"), "explorer.exe"), windows.EscapeArg(a.store.Dir), "", nil)
	}))
	must(w.Bind("vsCheckIP", func() map[string]string {
		out := map[string]string{}
		d, derr := netcheck.FetchIP("https://api.ipify.org", nil)
		out["direct"] = netcheck.Describe(d, derr)
		v, verr := netcheck.FetchIP("https://api.ipify.org", &url.URL{Scheme: "http", Host: a.eng.HTTPAddr})
		out["vpn"] = netcheck.Describe(v, verr)
		u, uerr := netcheck.UDPViaSocks(a.eng.SocksAddr)
		out["vpnUdp"] = netcheck.Describe(u, uerr)
		return out
	}))

	w.SetHtml(pageHTML("windows", ""))
	a.mu.Lock()
	a.w = w
	a.autoClose = autoClose
	a.mu.Unlock()
	platform.BringToFront(hwnd)
	w.Run()
	a.mu.Lock()
	a.w = nil
	a.mu.Unlock()
}

// configInUse: the VPN is connected (or connecting) right now.
func (a *agent) configInUse(name string) bool {
	s := a.eng.Status()
	return s.Config == name && (s.State == engine.StateConnected || s.State == engine.StateConnecting)
}
