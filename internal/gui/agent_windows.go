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
	"strings"
	"sync"
	"time"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"vpn_specify/internal/control"
	"vpn_specify/internal/engine"
	"vpn_specify/internal/launcher"
	"vpn_specify/internal/platform"
	"vpn_specify/internal/store"
	"vpn_specify/internal/update"
)

// ---------- agente ----------

type showReq struct{ autoClose bool }

type agent struct {
	store *store.Store
	eng   *engine.Engine
	logs  *logRing

	mu         sync.Mutex
	w          webview2.WebView
	autoClose  bool
	watch      map[string]bool // executáveis abertos pelo vpn_specify
	lastLaunch time.Time
	notice     string // mensagem para a tela (erros de abrir app etc.)
	showCh     chan showReq

	upd    *updater
	quitCh chan struct{} // atualização pronta: fechar o agente
	plan   *update.Plan  // aplicada depois de desconectar (veja runAgent)
}

func runAgent(show bool, launchID string, autostartNow bool) error {
	runtime.LockOSThread() // a janela WebView2 precisa ficar sempre na mesma thread

	st, err := store.Open()
	if err != nil {
		platform.MessageBox("vpn_specify", err.Error(), 0x10)
		return err
	}
	a := &agent{store: st, eng: engine.New(), logs: &logRing{}, watch: map[string]bool{}, showCh: make(chan showReq, 4), quitCh: make(chan struct{}, 1)}
	// registrado primeiro, roda por último: depois de desconectar e liberar as portas
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
		// corrida: outra instância acabou de abrir
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

	log.Printf("vpn_specify iniciado (dados em %s)", st.Dir)
	if show {
		a.showCh <- showReq{}
	}
	if launchID != "" {
		go a.launchFromShortcut(launchID)
	}
	go a.upd.autoCheck()
	go func() { // a entrada do sistema acompanha o executável atual
		if err := syncAutostart(st); err != nil {
			log.Printf("inicialização com o sistema: %v", err)
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

// quitForUpdate fecha a janela e o agente; runAgent desconecta e então
// aplica a atualização (o instalador espera este processo terminar).
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
		log.Printf("não consegui iniciar a atualização: %v", err)
		platform.MessageBox("vpn_specify", "Não consegui iniciar a atualização: "+err.Error(), 0x10)
	}
}

// autostart: "vpn_specify autostart" (tarefa agendada no logon) abre os
// apps marcados para iniciar com o sistema, sem mostrar a janela.
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

// shouldExit roda a cada 3s sem janela aberta: decide se a instância termina.
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
	case "conectando":
		return false
	case "conectado":
		if len(watch) == 0 {
			log.Printf("janela fechada e nenhum app aberto pela VPN: desconectando")
			a.eng.Disconnect()
			return true
		}
		if !a.store.Snapshot().AutoDisconnect {
			return false
		}
		if len(platform.RunningPIDs(watch)) == 0 {
			log.Printf("os apps da VPN foram fechados: desconectando")
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
		platform.MessageBox("vpn_specify", "Este atalho aponta para um app que não existe mais no vpn_specify.", 0x10)
		return
	}
	a.mu.Lock()
	a.lastLaunch = time.Now()
	windowOpen := a.w != nil
	a.mu.Unlock()
	needConnect := a.eng.Status().State != "conectado"
	if needConnect && !windowOpen {
		a.requestShow(true) // mostra o progresso enquanto conecta; fecha sozinha depois
	}
	if err := a.launchApp(app); err != nil {
		log.Printf("erro ao abrir %s: %v", app.Name, err)
		a.setNotice(fmt.Sprintf("Não consegui abrir %s: %v", app.Name, err))
		a.mu.Lock()
		a.autoClose = false
		hasWindow := a.w != nil
		a.mu.Unlock()
		if !hasWindow {
			platform.MessageBox("vpn_specify", fmt.Sprintf("Não consegui abrir %s:\n\n%v", app.Name, err), 0x10)
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

// launchApp conecta (se preciso), liga a captura e abre o app sem elevação.
func (a *agent) launchApp(app store.AppEntry) error {
	a.mu.Lock()
	a.lastLaunch = time.Now()
	a.mu.Unlock()
	st := a.eng.Status()
	if (st.State == "conectado" || st.State == "conectando") && st.Config != app.Config {
		return fmt.Errorf("a VPN %q está ativa e %s usa %q; desconecte antes", st.Config, app.Name, app.Config)
	}
	if st.State != "conectado" {
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
	log.Printf("%s aberto na VPN (pid %d, modo %s)", app.Name, pid, app.Mode)
	return nil
}

func (a *agent) waitConnected(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		switch st := a.eng.Status(); st.State {
		case "conectado":
			return nil
		case "erro":
			return errors.New(st.Error)
		case "desconectado":
			return errors.New("conexão cancelada")
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("tempo esgotado conectando")
}
