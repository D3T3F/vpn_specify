//go:build windows

package gui

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"vpn_specify/internal/control"
	"vpn_specify/internal/engine"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/launcher"
	"vpn_specify/internal/platform"
	"vpn_specify/internal/store"
	"vpn_specify/internal/update"
)

// ---------- agent ----------

type showReq struct{ autoClose bool }

type agent struct {
	store *store.Store
	eng   *engine.Engine
	logs  *logRing

	mu         sync.Mutex
	w          webview2.WebView
	autoClose  bool
	watch      map[string]bool // executables opened by vpn_specify
	lastLaunch time.Time
	notice     string // message for the window (errors opening apps etc.)
	showCh     chan showReq

	upd    *updater
	quitCh chan struct{} // update ready: exit the agent
	plan   *update.Plan  // aplicada depois de desconectar (veja runAgent)
}

func runAgent(show bool, launchID string, autostartNow bool) error {
	runtime.LockOSThread() // the WebView2 window must always stay on the same thread

	st, err := store.Open()
	if err != nil {
		platform.MessageBox("vpn_specify", err.Error(), 0x10)
		return err
	}
	a := &agent{store: st, eng: engine.New(), logs: &logRing{}, watch: map[string]bool{}, showCh: make(chan showReq, 4), quitCh: make(chan struct{}, 1)}
	// registered first, runs last: after disconnecting and releasing the ports
	defer a.runPlan()
	a.upd = newUpdater(st, a.quitForUpdate, func(u string) error {
		_, err := platform.StartUnelevated(filepath.Join(os.Getenv("WINDIR"), "explorer.exe"), windows.EscapeArg(u), "", nil)
		return err
	})
	if lf, err := os.OpenFile(filepath.Join(st.Dir, "vpn_specify.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		defer lf.Close()
		log.SetOutput(io.MultiWriter(lf, a.logs))
	} else {
		log.SetOutput(a.logs)
	}

	stopControl, err := control.Start(map[string]func(url.Values) error{
		"show":      func(url.Values) error { a.requestShow(false); return nil },
		"launch":    func(q url.Values) error { go a.launchFromShortcut(q.Get("id")); return nil },
		"autostart": func(url.Values) error { go a.autostart(); return nil },
	})
	if err != nil {
		// race: another instance just started
		if autostartNow {
			return control.Call("autostart", nil)
		}
		if launchID != "" {
			return control.Call("launch", url.Values{"id": {launchID}})
		}
		return control.Call("show", nil)
	}
	defer stopControl()
	defer a.eng.Disconnect()

	log.Printf("vpn_specify started (data in %s)", st.Dir)
	if show {
		a.showCh <- showReq{}
	}
	if launchID != "" {
		go a.launchFromShortcut(launchID)
	}
	go a.upd.autoCheck()
	go func() { // the system entry follows the current executable
		if err := syncAutostart(st); err != nil {
			log.Printf("start with the system: %v", err)
		}
	}()
	if autostartNow {
		go a.autostart()
	}

	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case r := <-a.showCh:
			a.runWindow(r.autoClose)
		case <-a.quitCh:
			return nil
		case <-tick.C:
			if a.shouldExit() {
				return nil
			}
		}
	}
}

// quitForUpdate closes the window and the agent; runAgent disconnects and
// then applies the update (the installer waits for this process to end).
func (a *agent) quitForUpdate(plan *update.Plan) {
	a.mu.Lock()
	a.plan = plan
	w := a.w
	a.mu.Unlock()
	if w != nil {
		w.Dispatch(func() { w.Destroy() })
	}
	select {
	case a.quitCh <- struct{}{}:
	default:
	}
}

func (a *agent) runPlan() {
	a.mu.Lock()
	plan := a.plan
	a.mu.Unlock()
	if plan == nil {
		return
	}
	if err := plan.Run(); err != nil {
		log.Printf("could not start the update: %v", err)
		platform.MessageBox("vpn_specify", i18n.T("Could not start the update: {0}", err), 0x10)
	}
}

// autostart: "vpn_specify autostart" (scheduled task at logon) opens the
// apps marked to start with the system, without showing the window.
func (a *agent) autostart() {
	a.mu.Lock()
	a.lastLaunch = time.Now()
	a.mu.Unlock()
	runAutostart(a.store, a.launchApp, func(msg string) {
		a.setNotice(msg)
		a.mu.Lock()
		hasWindow := a.w != nil
		a.mu.Unlock()
		if !hasWindow {
			platform.MessageBox("vpn_specify", msg, 0x10)
		}
	})
}

func (a *agent) requestShow(autoClose bool) {
	a.mu.Lock()
	w := a.w
	a.mu.Unlock()
	if w != nil {
		w.Dispatch(func() { platform.BringToFront(uintptr(w.Window())) })
		return
	}
	select {
	case a.showCh <- showReq{autoClose: autoClose}:
	default:
	}
}

// shouldExit runs every 3s with no window open: decides whether the instance exits.
func (a *agent) shouldExit() bool {
	a.mu.Lock()
	windowOpen := a.w != nil
	recent := time.Since(a.lastLaunch) < 30*time.Second
	watch := map[string]bool{}
	for k := range a.watch {
		watch[k] = true
	}
	a.mu.Unlock()
	if windowOpen || recent || len(a.showCh) > 0 {
		return false
	}
	st := a.eng.Status()
	switch st.State {
	case engine.StateConnecting:
		return false
	case engine.StateConnected:
		if len(watch) == 0 {
			log.Printf("window closed and no app opened through the VPN: disconnecting")
			a.eng.Disconnect()
			return true
		}
		if !a.store.Snapshot().AutoDisconnect {
			return false
		}
		if len(platform.RunningPIDs(watch)) == 0 {
			log.Printf("the VPN apps were closed: disconnecting")
			a.eng.Disconnect()
			return true
		}
		return false
	default:
		return true
	}
}

func (a *agent) setNotice(s string) {
	a.mu.Lock()
	a.notice = s
	a.mu.Unlock()
}

func (a *agent) launchFromShortcut(id string) {
	app, ok := a.store.App(id)
	if !ok {
		platform.MessageBox("vpn_specify", i18n.T("This shortcut points to an app that no longer exists in vpn_specify."), 0x10)
		return
	}
	a.mu.Lock()
	a.lastLaunch = time.Now()
	windowOpen := a.w != nil
	a.mu.Unlock()
	needConnect := a.eng.Status().State != engine.StateConnected
	if needConnect && !windowOpen {
		a.requestShow(true) // shows the progress while connecting; closes by itself afterwards
	}
	if err := a.launchApp(app); err != nil {
		log.Printf("error opening %s: %v", app.Name, err)
		a.setNotice(i18n.T("Could not open {0}: {1}", app.Name, err))
		a.mu.Lock()
		a.autoClose = false
		hasWindow := a.w != nil
		a.mu.Unlock()
		if !hasWindow {
			platform.MessageBox("vpn_specify", i18n.T("Could not open {0}:", app.Name)+"\n\n"+err.Error(), 0x10)
		}
		return
	}
	a.mu.Lock()
	w, ac := a.w, a.autoClose
	a.autoClose = false
	a.mu.Unlock()
	if w != nil && ac {
		time.Sleep(1500 * time.Millisecond)
		w.Dispatch(func() { w.Destroy() })
	}
}

// launchApp connects (if needed), starts the capture and opens the app unelevated.
func (a *agent) launchApp(app store.AppEntry) error {
	a.mu.Lock()
	a.lastLaunch = time.Now()
	a.mu.Unlock()
	st := a.eng.Status()
	if (st.State == engine.StateConnected || st.State == engine.StateConnecting) && st.Config != app.Config {
		return i18n.Error("the VPN {0} is active and {1} uses {2}; disconnect first", strconv.Quote(st.Config), app.Name, strconv.Quote(app.Config))
	}
	if st.State != engine.StateConnected {
		path, opts, err := a.store.ConfigFor(app.Config)
		if err != nil {
			return err
		}
		if err := a.eng.Connect(app.Config, path, opts, nil); err != nil {
			return err
		}
		if err := a.waitConnected(5 * time.Minute); err != nil {
			return err
		}
	}

	var env []string
	args := app.Args
	watch := app.Capture
	if app.Mode == "capture" {
		if err := a.eng.EnsureCapture(app.Capture); err != nil {
			return fmt.Errorf("captura: %w", err)
		}
	} else {
		lo := launcher.Options{Socks: a.eng.SocksAddr, HTTP: a.eng.HTTPAddr, Profile: "app-" + app.ID, Chromium: true}
		pre, _, err := launcher.ProxyArgsFor(app.Path, lo)
		if err != nil {
			return err
		}
		var quoted []string
		for _, p := range pre {
			quoted = append(quoted, windows.EscapeArg(p))
		}
		args = strings.TrimSpace(strings.Join(quoted, " ") + " " + args)
		env = append(os.Environ(), launcher.ProxyEnv(lo)...)
		watch = []string{strings.ToLower(filepath.Base(app.Path))}
	}

	pid, err := platform.StartUnelevated(app.Path, args, app.WorkDir, env)
	if err != nil {
		return err
	}
	a.mu.Lock()
	for _, n := range watch {
		a.watch[n] = true
	}
	a.lastLaunch = time.Now()
	a.mu.Unlock()
	log.Printf("%s opened in the VPN (pid %d, %s mode)", app.Name, pid, app.Mode)
	return nil
}

func (a *agent) waitConnected(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		switch st := a.eng.Status(); st.State {
		case engine.StateConnected:
			return nil
		case engine.StateError:
			return errors.New(st.Error)
		case engine.StateDisconnected:
			return engine.ErrCanceled
		}
		time.Sleep(300 * time.Millisecond)
	}
	return i18n.Error("timed out connecting")
}
