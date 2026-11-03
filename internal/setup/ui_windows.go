//go:build windows

package setup

import (
	_ "embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jchv/go-webview2"

	vpnspecify "vpn_specify"
	"vpn_specify/internal/brand"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/platform"
	"vpn_specify/internal/version"
)

//go:embed web/setup.html
var setupHTML string

type progressState struct {
	Message string `json:"message"`
	Done    bool   `json:"done"`
	Error   string `json:"error"`
	Dir     string `json:"dir"`
}

// RunInstaller shows the installer window. payload has the files that go into
// the installation folder (vpn_specify.exe, windivert\..., etc.).
func RunInstaller(payload fs.FS) error {
	if !platform.IsElevated() {
		// Program Files and the uninstall registry need Administrator.
		return platform.ElevateSelf(os.Args[1:])
	}
	dataPath := filepath.Join(os.TempDir(), "vpn_specify-setup-webview")
	defer os.RemoveAll(dataPath)

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  dataPath,
		WindowOptions: webview2.WindowOptions{
			Title: i18n.T("Install vpn_specify") + " " + version.Version, Width: 640, Height: 540, Center: true,
			IconId: 1,
		},
	})
	if w == nil {
		platform.MessageBox(i18n.T("Install vpn_specify"), i18n.T("Could not open the installer: the Microsoft Edge WebView2 Runtime is not installed.\nDownload it from {0}", "https://developer.microsoft.com/microsoft-edge/webview2/"), 0x10)
		return nil
	}
	defer w.Destroy()
	hwnd := uintptr(w.Window())
	platform.SetWindowIcons(hwnd)
	w.SetSize(560, 480, webview2.HintMin)

	var (
		mu      sync.Mutex
		st      progressState
		running bool
	)
	must := func(err error) {
		if err != nil {
			log.Printf("bind: %v", err)
		}
	}
	must(w.Bind("vsInit", func() map[string]any {
		dir, upgrade := InstalledDir(), true
		if dir == "" {
			dir, upgrade = DefaultDir(), false
		}
		return map[string]any{"dir": dir, "upgrade": upgrade, "version": version.Version, "license": vpnspecify.License}
	}))
	// vsSetLanguage changes the installer language right away; the choice is
	// saved for the app when the installation succeeds.
	must(w.Bind("vsSetLanguage", func(code string) i18n.PageState {
		i18n.Set(code)
		return i18n.State()
	}))
	must(w.Bind("vsBrowse", func(current string) (string, error) {
		start := current
		for start != "" {
			if fi, err := os.Stat(start); err == nil && fi.IsDir() {
				break
			}
			parent := filepath.Dir(start)
			if parent == start {
				start = ""
				break
			}
			start = parent
		}
		p, ok := platform.BrowseFolder(hwnd, i18n.T("Choose where to install vpn_specify"), start)
		if !ok {
			return "", nil
		}
		return NormalizeDir(p)
	}))
	must(w.Bind("vsInstall", func(o Options) error {
		mu.Lock()
		defer mu.Unlock()
		if running {
			return nil
		}
		running, st = true, progressState{Message: i18n.T("Preparing…")}
		go func() {
			dir, err := Install(payload, o, func(msg string) {
				mu.Lock()
				st.Message = msg
				mu.Unlock()
			})
			if err == nil && o.Language != "" {
				if lerr := i18n.Save(o.Language); lerr != nil {
					log.Printf("saving the language: %v", lerr)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			running = false
			st.Done, st.Dir = true, dir
			if err != nil {
				st.Error = err.Error()
			}
		}()
		return nil
	}))
	must(w.Bind("vsProgress", func() progressState {
		mu.Lock()
		defer mu.Unlock()
		return st
	}))
	must(w.Bind("vsFinish", func(launch bool) {
		mu.Lock()
		dir := st.Dir
		mu.Unlock()
		if launch && dir != "" {
			exe := filepath.Join(dir, MainExe)
			if _, err := platform.StartNormal(exe, `"`+exe+`"`, dir, nil); err != nil {
				platform.MessageBox("vpn_specify", i18n.T("Could not open vpn_specify: {0}", err), 0x30)
			}
		}
		w.Terminate()
	}))
	must(w.Bind("vsClose", func() {
		mu.Lock()
		busy := running
		mu.Unlock()
		if !busy {
			w.Terminate()
		}
	}))

	w.SetHtml(strings.NewReplacer("{{LOGO}}", brand.LogoDataURI(), "{{I18N_SCRIPT}}", i18n.PageScript()).Replace(setupHTML))
	platform.BringToFront(hwnd)
	w.Run()
	return nil
}
