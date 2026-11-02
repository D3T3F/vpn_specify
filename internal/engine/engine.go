package engine

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"vpn_specify/internal/capture"
	"vpn_specify/internal/proxy"
	"vpn_specify/internal/tunnel"
)

// Endereços padrão dos proxies locais.
const (
	DefaultSocksAddr = "127.0.0.1:10800"
	DefaultHTTPAddr  = "127.0.0.1:10801"
)

type Status struct {
	State    string   `json:"state"` // desconectado | conectando | conectado | erro
	Config   string   `json:"config"`
	Error    string   `json:"error"`
	Since    int64    `json:"since"`
	Captured []string `json:"captured"`
	Socks    string   `json:"socks"`
	HTTP     string   `json:"http"`
}

// Engine mantém uma conexão VPN com os proxies locais e (opcionalmente) a
// captura por processo. Usado pelo `up` (CLI) e pela interface gráfica.
type Engine struct {
	SocksAddr, HTTPAddr string
	Verbose             bool

	mu      sync.Mutex
	status  Status
	tun     tunnel.Tunnel
	closers []io.Closer
	capture capture.Controller
	gen     int // muda a cada conexão, para ignorar eventos de conexões antigas
}

func New() *Engine {
	return &Engine{SocksAddr: DefaultSocksAddr, HTTPAddr: DefaultHTTPAddr, status: Status{State: "desconectado"}}
}

func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.status
	s.Socks, s.HTTP = e.SocksAddr, e.HTTPAddr
	if e.capture != nil {
		s.Captured = e.capture.Names()
	}
	return s
}

func (e *Engine) Tunnel() tunnel.Tunnel {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tun
}

// Connect conecta (bloqueia até conectar ou falhar).
func (e *Engine) Connect(name, path string, o tunnel.Options, forwards []proxy.Forward) error {
	e.mu.Lock()
	if e.status.State == "conectando" || e.status.State == "conectado" {
		cur := e.status.Config
		e.mu.Unlock()
		if cur == name {
			return nil
		}
		return fmt.Errorf("a VPN %q já está ativa; desconecte antes", cur)
	}
	e.gen++
	gen := e.gen
	e.status = Status{State: "conectando", Config: name, Since: time.Now().Unix()}
	e.mu.Unlock()

	fail := func(err error) error {
		e.mu.Lock()
		if e.gen == gen {
			for _, c := range e.closers {
				c.Close()
			}
			e.closers = nil
			e.status = Status{State: "erro", Config: name, Error: err.Error()}
		}
		e.mu.Unlock()
		log.Printf("erro: %v", err)
		return err
	}

	var closers []io.Closer
	socksLn, err := net.Listen("tcp", e.SocksAddr)
	if err != nil {
		return fail(fmt.Errorf("não consegui abrir o proxy SOCKS em %s (outro vpn_specify rodando?): %w", e.SocksAddr, err))
	}
	closers = append(closers, socksLn)
	httpLn, err := net.Listen("tcp", e.HTTPAddr)
	if err != nil {
		socksLn.Close()
		return fail(fmt.Errorf("não consegui abrir o proxy HTTP em %s: %w", e.HTTPAddr, err))
	}
	closers = append(closers, httpLn)
	fwdConns := make([]io.Closer, len(forwards))
	for i, f := range forwards {
		c, err := proxy.ListenForward(f)
		if err != nil {
			for _, c := range closers {
				c.Close()
			}
			return fail(fmt.Errorf("encaminhamento %s: %w", f, err))
		}
		fwdConns[i] = c
		closers = append(closers, c)
	}
	e.mu.Lock()
	e.closers = closers
	e.mu.Unlock()

	log.Printf("conectando em %q...", name)
	tun, err := tunnel.Open(path, o)
	if err != nil {
		return fail(err)
	}
	go proxy.ServeSOCKS(socksLn, tun, e.Verbose)
	go proxy.ServeHTTP(httpLn, tun, e.Verbose)
	for i, f := range forwards {
		proxy.ServeForward(fwdConns[i], f, tun, e.Verbose)
	}

	e.mu.Lock()
	if e.gen != gen { // Disconnect chamado enquanto conectava
		e.mu.Unlock()
		tun.Close()
		return errors.New("conexão cancelada")
	}
	e.tun = tun
	e.status = Status{State: "conectado", Config: name, Since: time.Now().Unix()}
	e.mu.Unlock()
	log.Printf("VPN %q conectada. O resto do computador continua fora da VPN.", name)

	go func() {
		err, ok := <-tun.Done()
		if !ok && err == nil {
			return
		}
		e.mu.Lock()
		if e.gen != gen || e.tun != tun {
			e.mu.Unlock()
			return
		}
		e.mu.Unlock()
		e.disconnect(fmt.Sprintf("a VPN caiu: %v", err))
	}()
	return nil
}

// EnsureCapture liga a captura (se ainda não estiver) e adiciona os nomes.
func (e *Engine) EnsureCapture(names []string) error {
	if len(names) == 0 {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.tun == nil {
		return errors.New("VPN não conectada")
	}
	if e.capture != nil {
		e.capture.AddNames(names)
		return nil
	}
	c, err := capture.Start(e.tun, names, e.Verbose)
	if err != nil {
		return err
	}
	e.capture = c
	return nil
}

func (e *Engine) Disconnect() { e.disconnect("") }

func (e *Engine) disconnect(errMsg string) {
	e.mu.Lock()
	e.gen++
	capt, tun, closers := e.capture, e.tun, e.closers
	e.capture, e.tun, e.closers = nil, nil, nil
	wasActive := e.status.State == "conectado" || e.status.State == "conectando"
	cfg := e.status.Config
	if errMsg != "" {
		e.status = Status{State: "erro", Config: cfg, Error: errMsg}
	} else {
		e.status = Status{State: "desconectado"}
	}
	e.mu.Unlock()

	if capt != nil {
		capt.Stop()
	}
	for _, c := range closers {
		c.Close()
	}
	if tun != nil {
		tun.Close()
	}
	if wasActive {
		if errMsg != "" {
			log.Printf("desconectado: %s", errMsg)
		} else {
			log.Printf("VPN %q desconectada.", cfg)
		}
	}
}
