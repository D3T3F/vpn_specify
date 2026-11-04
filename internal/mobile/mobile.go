// Package mobile is the Go side of the Android app: one Call function that
// takes and returns JSON, reached from Kotlin through a single JNI function
// (cmd/libvpn_specify). Keeping JSON at the border means no type crosses the
// language boundary, and this package can be tested without a device.
package mobile

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/tunnel"
	"vpn_specify/internal/vpngate"
)

// Request is what Kotlin sends. Not every field applies to every method.
type Request struct {
	Method string `json:"method"`
	Config string `json:"config,omitempty"` // WireGuard configuration text
	FD     int    `json:"fd,omitempty"`     // tun file descriptor; Start takes ownership
	Text   string `json:"text,omitempty"`   // language code or locale name
	// VPN Gate: the app downloads (see VpnGate.kt) and Go reads the answers
	GateList string `json:"gateList,omitempty"`
	GateHome string `json:"gateHome,omitempty"`
}

var (
	mu  sync.Mutex
	dev *tunnel.WireGuardDevice

	gateMu   sync.Mutex
	gateList *vpngate.List
)

func init() {
	log.SetFlags(log.Ltime)
	log.SetOutput(ring)
}

// Call runs one request and returns {"result": ...} or {"error": "..."}.
func Call(req []byte) (reply []byte) {
	defer func() {
		if r := recover(); r != nil {
			reply = errorReply(fmt.Errorf("internal error: %v", r))
		}
	}()
	var r Request
	if err := json.Unmarshal(req, &r); err != nil {
		return errorReply(err)
	}
	res, err := dispatch(r)
	if err != nil {
		log.Printf("%s: %v", r.Method, err)
		return errorReply(err)
	}
	b, err := json.Marshal(map[string]any{"result": res})
	if err != nil {
		return errorReply(err)
	}
	return b
}

func errorReply(err error) []byte {
	b, _ := json.Marshal(map[string]string{"error": err.Error()})
	return b
}

func dispatch(r Request) (any, error) {
	switch r.Method {
	case "languages":
		return i18n.Languages, nil
	case "matchLanguage":
		return i18n.Match(r.Text), nil
	case "setLanguage":
		i18n.Set(r.Text)
		return i18n.Catalog(i18n.Current()), nil
	case "plan":
		return tunnel.PlanWireGuard([]byte(r.Config))
	case "planOpenVPN":
		return tunnel.PlanOpenVPN([]byte(r.Config))
	case "vpnGateList":
		l, err := vpngate.Build([]byte(r.GateList), []byte(r.GateHome), []string{r.Text})
		if err != nil {
			return nil, err
		}
		gateMu.Lock()
		gateList = l
		gateMu.Unlock()
		return l, nil
	case "vpnGateConfig":
		gateMu.Lock()
		l := gateList
		gateMu.Unlock()
		_, cfg, err := l.Config(r.Text)
		return string(cfg), err
	case "appendLog":
		log.Print(r.Text)
		return nil, nil
	case "resolve":
		return nil, tunnel.ResolveWireGuard([]byte(r.Config))
	case "start":
		return nil, start(r)
	case "stop":
		stop()
		return nil, nil
	case "rebind":
		mu.Lock()
		defer mu.Unlock()
		if dev == nil {
			return nil, nil
		}
		return nil, dev.Rebind()
	case "kick":
		mu.Lock()
		defer mu.Unlock()
		if dev != nil {
			dev.Kick()
		}
		return nil, nil
	case "stats":
		mu.Lock()
		defer mu.Unlock()
		if dev == nil {
			return tunnel.DeviceStats{}, nil
		}
		return dev.Stats()
	case "log":
		return strings.Join(ring.lines(), "\n"), nil
	}
	return nil, fmt.Errorf("unknown method %q", r.Method)
}

func start(r Request) error {
	stop()
	tdev, err := openTun(r.FD)
	if err != nil {
		return err
	}
	d, err := tunnel.StartWireGuardDevice([]byte(r.Config), tdev)
	if err != nil {
		return err
	}
	mu.Lock()
	dev = d
	mu.Unlock()
	return nil
}

func stop() {
	mu.Lock()
	d := dev
	dev = nil
	mu.Unlock()
	if d != nil {
		d.Close()
	}
}

// logRing keeps the last lines of the log for the app's log screen.
type logRing struct {
	mu  sync.Mutex
	buf []string
}

var ring = &logRing{}

func (l *logRing) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = append(l.buf, strings.TrimRight(string(b), "\r\n"))
	if len(l.buf) > 300 {
		l.buf = append([]string(nil), l.buf[len(l.buf)-300:]...)
	}
	return len(b), nil
}

func (l *logRing) lines() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.buf...)
}
