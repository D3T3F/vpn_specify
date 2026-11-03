package engine

import (
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"vpn_specify/internal/capture"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/proxy"
	"vpn_specify/internal/tunnel"
)

// Default addresses of the local proxies.
const (
	DefaultSocksAddr = "127.0.0.1:10800"
	DefaultHTTPAddr  = "127.0.0.1:10801"
)

// Connection states (Status.State; also used by the web page).
const (
	StateDisconnected = "disconnected"
	StateConnecting   = "connecting"
	StateConnected    = "connected"
	StateError        = "error"
)

type Status struct {
	State    string   `json:"state"` // one of the State* constants
	Config   string   `json:"config"`
	Error    string   `json:"error"`
	Since    int64    `json:"since"`
	Captured []string `json:"captured"`
	Socks    string   `json:"socks"`
	HTTP     string   `json:"http"`
}

// Engine keeps a VPN connection with the local proxies and (optionally)
// per-process capture. Used by `up` (CLI) and by the graphical interface.
type Engine struct {
	SocksAddr, HTTPAddr string
	Verbose             bool

	mu      sync.Mutex
	status  Status
	tun     tunnel.Tunnel
	closers []io.Closer
	capture capture.Controller
	gen     int // changes on each connection, to ignore events of older ones
}

// ErrCanceled: Disconnect was called while connecting.
var ErrCanceled = i18n.Sentinel("connection canceled")

func New() *Engine {
	return &Engine{SocksAddr: DefaultSocksAddr, HTTPAddr: DefaultHTTPAddr, status: Status{State: StateDisconnected}}
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

// Connect connects (blocks until connected or failed).
func (e *Engine) Connect(name, path string, o tunnel.Options, forwards []proxy.Forward) error {
	e.mu.Lock()
	if e.status.State == StateConnecting || e.status.State == StateConnected {
		cur := e.status.Config
		e.mu.Unlock()
		if cur == name {
			return nil
		}
		return i18n.Error("the VPN {0} is already active; disconnect it first", strconv.Quote(cur))
	}
	e.gen++
	gen := e.gen
	e.status = Status{State: StateConnecting, Config: name, Since: time.Now().Unix()}
	e.mu.Unlock()

	fail := func(err error) error {
		e.mu.Lock()
		if e.gen == gen {
			for _, c := range e.closers {
				c.Close()
			}
			e.closers = nil
			e.status = Status{State: StateError, Config: name, Error: err.Error()}
		}
		e.mu.Unlock()
		log.Printf("error: %v", err)
		return err
	}

	var closers []io.Closer
	socksLn, err := net.Listen("tcp", e.SocksAddr)
	if err != nil {
		return fail(i18n.Wrap(err, "could not open the SOCKS proxy on {0} (is another vpn_specify running?)", e.SocksAddr))
	}
	closers = append(closers, socksLn)
	httpLn, err := net.Listen("tcp", e.HTTPAddr)
	if err != nil {
		socksLn.Close()
		return fail(i18n.Wrap(err, "could not open the HTTP proxy on {0}", e.HTTPAddr))
	}
	closers = append(closers, httpLn)
	fwdConns := make([]io.Closer, len(forwards))
	for i, f := range forwards {
		c, err := proxy.ListenForward(f)
		if err != nil {
			for _, c := range closers {
				c.Close()
			}
			return fail(i18n.Wrap(err, "port forward {0}", f))
		}
		fwdConns[i] = c
		closers = append(closers, c)
	}
	e.mu.Lock()
	e.closers = closers
	e.mu.Unlock()

	log.Printf("connecting to %q...", name)
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
	if e.gen != gen { // Disconnect called while connecting
		e.mu.Unlock()
		tun.Close()
		return ErrCanceled
	}
	e.tun = tun
	e.status = Status{State: StateConnected, Config: name, Since: time.Now().Unix()}
	e.mu.Unlock()
	log.Printf("VPN %q connected. The rest of the computer stays outside the VPN.", name)

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

// EnsureCapture starts the capture (if not running yet) and adds the names.
func (e *Engine) EnsureCapture(names []string) error {
	if len(names) == 0 {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.tun == nil {
		return i18n.Error("VPN not connected")
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
	wasActive := e.status.State == StateConnected || e.status.State == StateConnecting
	cfg := e.status.Config
	if errMsg != "" {
		e.status = Status{State: StateError, Config: cfg, Error: errMsg}
	} else {
		e.status = Status{State: StateDisconnected}
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
			log.Printf("disconnected: %s", errMsg)
		} else {
			log.Printf("VPN %q disconnected.", cfg)
		}
	}
}
