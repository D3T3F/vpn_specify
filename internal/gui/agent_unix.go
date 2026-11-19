//go:build linux

package gui

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"vpn_specify/internal/control"
	"vpn_specify/internal/engine"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/launcher"
	"vpn_specify/internal/platform"
	"vpn_specify/internal/store"
	"vpn_specify/internal/update"
)

// The Linux agent does the same as on Windows (agent_windows.go): keeps the
// VPN, opens the apps and exits by itself when they close. The differences:
//   - it runs as a normal user; only OpenVPN asks for the administrator
//     password (pkexec), when connecting;
//   - the window is a local page (server_linux.go);
//   - there is no full capture: every app uses proxy mode.

type agent struct {
	store *store.Store
	eng   *engine.Engine
	logs  *logRing
	ui    *uiServer

	mu         sync.Mutex
	autoClose  bool
	watch      map[string]bool // executables opened by vpn_specify
	pgids      map[int]bool    // process groups opened by vpn_specify
	lastLaunch time.Time
	notice     string
	showCh     chan bool // window request (autoClose)

	upd    *updater
	quitCh chan struct{} // update ready: exit the agent
	plan   *update.Plan  // aplicada depois de desconectar (veja runAgent)
}

func runAgent(show bool, launchID string, autostartNow bool) error {
	st, err := store.Open()
	if err != nil {
		platform.MessageBox("vpn_specify", err.Error(), 0x10)
		return err
	}
	a := &agent{store: st, eng: engine.New(), logs: &logRing{}, watch: map[string]bool{}, pgids: map[int]bool{}, showCh: make(chan bool, 4), quitCh: make(chan struct{}, 1)}
	// registered first, runs last: after disconnecting and releasing the
	// ports (on Linux, Run replaces this process with the new version)
	defer a.runPlan()
	a.upd = newUpdater(st, a.quitForUpdate, openWithDefault)
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

	if a.ui, err = newUIServer(a.api()); err != nil {
		platform.MessageBox("vpn_specify", i18n.T("Could not open the interface: {0}", err), 0x10)
		return err
	}
	log.Printf("vpn_specify started (data in %s)", st.Dir)
	if show {
		a.showCh <- false
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
		case ac := <-a.showCh:
			a.mu.Lock()
			a.autoClose = ac
			a.mu.Unlock()
			if err := a.ui.openWindow(); err != nil {
				log.Printf("%v", err)
				fmt.Fprintln(os.Stderr, err)
				platform.MessageBox("vpn_specify", err.Error(), 0x10)
			} else {
				fmt.Fprintf(os.Stderr, "vpn_specify: window at %s\n", a.ui.pageURL())
			}
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
// then applies the update.
func (a *agent) quitForUpdate(plan *update.Plan) {
	a.mu.Lock()
	a.plan = plan
	a.mu.Unlock()
	a.ui.broadcast("close")
	time.Sleep(300 * time.Millisecond) // the event arrives before the server goes away
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
		log.Printf("could not reopen after updating: %v", err)
		platform.MessageBox("vpn_specify", i18n.T("The update was installed, but vpn_specify could not be reopened: {0}", err), 0x10)
	}
}

// autostart: "vpn_specify autostart" (XDG autostart, when the session
// starts) opens the apps marked to start with the system, without showing the
// window.
func (a *agent) autostart() {
	a.mu.Lock()
	a.lastLaunch = time.Now()
	a.mu.Unlock()
	runAutostart(a.store, a.launchApp, func(msg string) {
		a.setNotice(msg)
		if !a.ui.isOpen() {
			platform.MessageBox("vpn_specify", msg, 0x10)
		}
	})
}

func (a *agent) requestShow(autoClose bool) {
	select {
	case a.showCh <- autoClose:
	default:
	}
}

// shouldExit runs every 3s: decides whether the instance exits (no window,
// and the VPN off or without apps opened through it).
func (a *agent) shouldExit() bool {
	a.mu.Lock()
	recent := time.Since(a.lastLaunch) < 30*time.Second
	watch := map[string]bool{}
	for k := range a.watch {
		watch[k] = true
	}
	pgids := map[int]bool{}
	for k := range a.pgids {
		pgids[k] = true
	}
	a.mu.Unlock()
	if a.ui.isOpen() || recent || len(a.showCh) > 0 {
		return false
	}
	switch a.eng.Status().State {
	case engine.StateConnecting:
		return false
	case engine.StateConnected:
		if len(watch) == 0 && len(pgids) == 0 {
			log.Printf("window closed and no app opened through the VPN: disconnecting")
			a.eng.Disconnect()
			return true
		}
		if !a.store.Snapshot().AutoDisconnect {
			return false
		}
		if !anyRunning(pgids, watch) {
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
	a.mu.Unlock()
	windowOpen := a.ui.isOpen()
	if a.eng.Status().State != engine.StateConnected && !windowOpen {
		a.requestShow(true) // shows the progress while connecting; closes by itself afterwards
	}
	if err := a.launchApp(app); err != nil {
		log.Printf("error opening %s: %v", app.Name, err)
		a.setNotice(i18n.T("Could not open {0}: {1}", app.Name, err))
		a.mu.Lock()
		a.autoClose = false
		a.mu.Unlock()
		if !a.ui.isOpen() {
			platform.MessageBox("vpn_specify", i18n.T("Could not open {0}:", app.Name)+"\n\n"+err.Error(), 0x10)
		}
		return
	}
	a.mu.Lock()
	ac := a.autoClose
	a.autoClose = false
	a.mu.Unlock()
	if ac {
		time.Sleep(1500 * time.Millisecond)
		a.ui.broadcast("close")
	}
}

// launchApp connects (if needed) and opens the app configured to use the proxy.
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
	if app.Mode == "capture" {
		log.Printf("%s: full capture only exists on Windows; opening in proxy mode", app.Name)
	}

	lo := launcher.Options{Socks: a.eng.SocksAddr, HTTP: a.eng.HTTPAddr, Profile: "app-" + app.ID, Chromium: true}
	pre, _, err := launcher.ProxyArgsFor(app.Path, lo)
	if err != nil {
		return err
	}
	// the proxy flags go after the app's arguments: that way they also work
	// with "flatpak run ... com.app.Id", which passes to the app whatever
	// comes after the id
	args := append(splitArgs(app.Args), pre...)
	env := append(os.Environ(), launcher.ProxyEnv(lo)...)
	pid, err := startDetached(app.Path, args, app.WorkDir, env)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.watch[filepath.Base(app.Path)] = true
	a.pgids[pid] = true
	a.lastLaunch = time.Now()
	a.mu.Unlock()
	log.Printf("%s opened in the VPN (pid %d, proxy mode)", app.Name, pid)
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

// configInUse: the VPN is connected (or connecting) right now.
func (a *agent) configInUse(name string) bool {
	s := a.eng.Status()
	return s.Config == name && (s.State == engine.StateConnected || s.State == engine.StateConnecting)
}
