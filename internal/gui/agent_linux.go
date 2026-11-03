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
	"sync"
	"time"

	"vpn_specify/internal/control"
	"vpn_specify/internal/engine"
	"vpn_specify/internal/launcher"
	"vpn_specify/internal/platform"
	"vpn_specify/internal/store"
)

// O agente no Linux faz o mesmo que no Windows (agent_windows.go): mantém a
// VPN, abre os apps e se encerra sozinho quando eles fecham. As diferenças:
//   - roda como usuário comum; só o OpenVPN pede a senha de administrador
//     (pkexec), na hora de conectar;
//   - a janela é uma página local (server_linux.go);
//   - não há captura completa: todo app usa o modo proxy.

type agent struct {
	store *store.Store
	eng   *engine.Engine
	logs  *logRing
	ui    *uiServer

	mu         sync.Mutex
	autoClose  bool
	watch      map[string]bool // executáveis abertos pelo vpn_specify
	pgids      map[int]bool    // grupos de processos abertos pelo vpn_specify
	lastLaunch time.Time
	notice     string
	showCh     chan bool // pedido de janela (autoClose)
}

func runAgent(show bool, launchID string) error {
	st, err := store.Open()
	if err != nil {
		platform.MessageBox("vpn_specify", err.Error(), 0x10)
		return err
	}
	a := &agent{store: st, eng: engine.New(), logs: &logRing{}, watch: map[string]bool{}, pgids: map[int]bool{}, showCh: make(chan bool, 4)}
	if lf, err := os.OpenFile(filepath.Join(st.Dir, "vpn_specify.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		defer lf.Close()
		log.SetOutput(io.MultiWriter(lf, a.logs))
	} else {
		log.SetOutput(a.logs)
	}

	stopControl, err := control.Start(map[string]func(url.Values) error{
		"show":   func(url.Values) error { a.requestShow(false); return nil },
		"launch": func(q url.Values) error { go a.launchFromShortcut(q.Get("id")); return nil },
	})
	if err != nil {
		// corrida: outra instância acabou de abrir
		if launchID != "" {
			return control.Call("launch", url.Values{"id": {launchID}})
		}
		return control.Call("show", nil)
	}
	defer stopControl()
	defer a.eng.Disconnect()

	if a.ui, err = newUIServer(a.api()); err != nil {
		platform.MessageBox("vpn_specify", "Não consegui abrir a interface: "+err.Error(), 0x10)
		return err
	}
	log.Printf("vpn_specify iniciado (dados em %s)", st.Dir)
	if show {
		a.showCh <- false
	}
	if launchID != "" {
		go a.launchFromShortcut(launchID)
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
				fmt.Fprintf(os.Stderr, "vpn_specify: janela em %s\n", a.ui.pageURL())
			}
		case <-tick.C:
			if a.shouldExit() {
				return nil
			}
		}
	}
}

func (a *agent) requestShow(autoClose bool) {
	select {
	case a.showCh <- autoClose:
	default:
	}
}

// shouldExit roda a cada 3s: decide se a instância termina (sem janela,
// e a VPN desligada ou sem apps abertos por ela).
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
	case "conectando":
		return false
	case "conectado":
		if len(watch) == 0 && len(pgids) == 0 {
			log.Printf("janela fechada e nenhum app aberto pela VPN: desconectando")
			a.eng.Disconnect()
			return true
		}
		if !a.store.Snapshot().AutoDisconnect {
			return false
		}
		if !anyRunning(pgids, watch) {
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
	a.mu.Unlock()
	windowOpen := a.ui.isOpen()
	if a.eng.Status().State != "conectado" && !windowOpen {
		a.requestShow(true) // mostra o progresso enquanto conecta; fecha sozinha depois
	}
	if err := a.launchApp(app); err != nil {
		log.Printf("erro ao abrir %s: %v", app.Name, err)
		a.setNotice(fmt.Sprintf("Não consegui abrir %s: %v", app.Name, err))
		a.mu.Lock()
		a.autoClose = false
		a.mu.Unlock()
		if !a.ui.isOpen() {
			platform.MessageBox("vpn_specify", fmt.Sprintf("Não consegui abrir %s:\n\n%v", app.Name, err), 0x10)
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

// launchApp conecta (se preciso) e abre o app configurado para usar o proxy.
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
	if app.Mode == "capture" {
		log.Printf("%s: a captura completa só existe no Windows; abrindo no modo proxy", app.Name)
	}

	lo := launcher.Options{Socks: a.eng.SocksAddr, HTTP: a.eng.HTTPAddr, Profile: "app-" + app.ID, Chromium: true}
	pre, _, err := launcher.ProxyArgsFor(app.Path, lo)
	if err != nil {
		return err
	}
	// as flags de proxy vão depois dos argumentos do app: assim funcionam
	// também com "flatpak run ... com.app.Id", que repassa ao app o que vem
	// depois do id
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
	log.Printf("%s aberto na VPN (pid %d, modo proxy)", app.Name, pid)
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

// configInUse: a VPN está conectada (ou conectando) agora.
func (a *agent) configInUse(name string) bool {
	s := a.eng.Status()
	return s.Config == name && (s.State == "conectado" || s.State == "conectando")
}
