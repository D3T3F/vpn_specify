//go:build windows

package capture

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/platform"
	"vpn_specify/internal/tunnel"
)

// Per-process capture (-capture discord.exe)
//
// WinDivert hands vpn_specify every outgoing IP packet of the system. For each
// packet we decide, by its local port, whether it belongs to a captured
// process (Windows connection tables):
//
//   - It does not: the packet goes back to the network unchanged.
//   - Captured UDP (voice, games, DNS): the packet is dropped and its payload
//     leaves through a UDP socket inside the VPN. Replies become "inbound" IP
//     packets injected back, as if they came from the internet.
//   - Captured TCP: the packet is "reflected" to a local transparent proxy
//     (source/destination swapped, pointing at the proxy port). The proxy
//     learns the original destination from the NAT table and connects
//     through the VPN.
//   - Captured IPv6: dropped, so the app falls back to IPv4 (no leak outside
//     the VPN).

const captureRule = "vpn_specify capture"

// legacyCaptureRule is the rule name used by versions before 1.1 (removed too).
const legacyCaptureRule = "vpn_specify captura"

type tcpFlow struct {
	local, remote [4]byte
	rport         uint16
	ifIdx, subIdx uint32
	last          atomic.Int64
	done          atomic.Bool
}

type udpFlow struct {
	local         [4]byte
	lport         uint16
	ifIdx, subIdx uint32
	pc            net.PacketConn
	last          atomic.Int64
}

type ownerEntry struct {
	name    string
	target  bool
	expires time.Time
}

type captureEngine struct {
	tun     tunnel.Tunnel
	names   map[string]bool
	verbose bool

	h         divertHandle
	ln        net.Listener
	proxyPort uint16

	mu       sync.Mutex
	tcp      map[uint16]*tcpFlow // key: the app's local port
	udp      map[uint16]*udpFlow
	owners   map[string]ownerEntry
	stop     chan struct{}
	stopOnce sync.Once
}

func normalizeExeNames(list []string) map[string]bool {
	m := map[string]bool{}
	for _, n := range list {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" {
			continue
		}
		if !strings.HasSuffix(n, ".exe") {
			n += ".exe"
		}
		m[n] = true
	}
	return m
}

func Start(tun tunnel.Tunnel, exeNames []string, verbose bool) (Controller, error) {
	if !platform.IsElevated() {
		return nil, i18n.Error("-capture needs a terminal running as Administrator (it uses the WinDivert driver)")
	}
	if err := loadWinDivert(); err != nil {
		return nil, err
	}
	e := &captureEngine{
		tun: tun, names: normalizeExeNames(exeNames), verbose: verbose,
		tcp: map[uint16]*tcpFlow{}, udp: map[uint16]*udpFlow{}, owners: map[string]ownerEntry{},
		stop: make(chan struct{}),
	}

	ln, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		return nil, err
	}
	e.ln = ln
	e.proxyPort = uint16(ln.Addr().(*net.TCPAddr).Port)
	e.addFirewallRule()

	h, err := divertOpen("outbound and !loopback and (tcp or udp)")
	if err != nil {
		ln.Close()
		e.deleteFirewallRule()
		return nil, err
	}
	e.h = h

	go e.acceptLoop()
	for i := 0; i < 4; i++ {
		go e.worker()
	}
	go e.janitor()

	log.Printf("capture active for: %s (TCP, UDP; IPv6 blocked)", strings.Join(e.Names(), ", "))
	e.resetRunning(e.names)
	return e, nil
}

// resetRunning drops the TCP connections of processes that were already
// running, so they reconnect (now captured).
func (e *captureEngine) resetRunning(names map[string]bool) {
	if running := platform.RunningPIDs(names); len(running) > 0 {
		n := resetTCPConnections(running)
		log.Printf("warning: %d process(es) were already running; %d old TCP connection(s) were dropped so they reconnect through the VPN. "+
			"If something stays outside the VPN, close and reopen the app.", len(running), n)
	}
}

func (e *captureEngine) AddNames(list []string) {
	added := map[string]bool{}
	e.mu.Lock()
	for n := range normalizeExeNames(list) {
		if !e.names[n] {
			e.names[n] = true
			added[n] = true
		}
	}
	if len(added) > 0 {
		e.owners = map[string]ownerEntry{} // re-evaluate ports with the new list
	}
	e.mu.Unlock()
	if len(added) > 0 {
		log.Printf("capture: added %s", strings.Join(sortedKeys(added), ", "))
		e.resetRunning(added)
	}
}

func (e *captureEngine) Names() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return sortedKeys(e.names)
}

func (e *captureEngine) Stop() {
	e.stopOnce.Do(func() {
		close(e.stop)
		e.h.close()
		e.ln.Close()
		e.mu.Lock()
		for _, f := range e.udp {
			f.pc.Close()
		}
		e.mu.Unlock()
		e.deleteFirewallRule()
		log.Printf("capture stopped")
	})
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// The reflected packet reaches the proxy as an inbound connection from the
// remote IP; without a rule, Windows Firewall blocks it.
func (e *captureEngine) addFirewallRule() {
	exe, _ := os.Executable()
	e.deleteFirewallRule()
	out, err := exec.Command("netsh", "advfirewall", "firewall", "add", "rule", "name="+captureRule,
		"dir=in", "action=allow", "program="+exe, "protocol=TCP", "localport="+strconv.Itoa(int(e.proxyPort)), "enable=yes").CombinedOutput()
	if err != nil {
		log.Printf("warning: could not create the firewall rule (%v): %s", err, strings.TrimSpace(string(out)))
	}
}

func (e *captureEngine) deleteFirewallRule() {
	for _, name := range []string{captureRule, legacyCaptureRule} {
		_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+name).Run()
	}
}

// isTarget reports whether the local port belongs to a captured process.
func (e *captureEngine) isTarget(tcp, v6 bool, port uint16) (bool, string) {
	key := fmt.Sprint(tcp, v6, port)
	now := time.Now()
	e.mu.Lock()
	o, ok := e.owners[key]
	e.mu.Unlock()
	if ok && now.Before(o.expires) {
		return o.target, o.name
	}
	pid := portOwner(tcp, v6, port)
	var name string
	if pid != 0 {
		name = platform.ProcessName(pid)
	}
	e.mu.Lock()
	o = ownerEntry{name: name, target: e.names[name]}
	e.mu.Unlock()
	if o.target {
		o.expires = now.Add(10 * time.Second)
	} else {
		o.expires = now.Add(3 * time.Second)
	}
	e.mu.Lock()
	e.owners[key] = o
	e.mu.Unlock()
	return o.target, o.name
}

func (e *captureEngine) worker() {
	buf := make([]byte, 0xFFFF)
	var addr divertAddr
	for {
		n, err := e.h.recv(buf, &addr)
		if err != nil {
			select {
			case <-e.stop:
				return
			default:
			}
			if errors.Is(err, errNoData) {
				return
			}
			continue
		}
		pkt := buf[:n]
		if e.handle(pkt, &addr) {
			_ = e.h.send(pkt, &addr)
		}
	}
}

// handle returns true if the (possibly modified) packet must be sent.
func (e *captureEngine) handle(pkt []byte, addr *divertAddr) bool {
	if len(pkt) < 20 {
		return true
	}
	if pkt[0]>>4 == 6 {
		return e.handleIPv6(pkt)
	}
	ihl := int(pkt[0]&0x0f) * 4
	if ihl < 20 || len(pkt) < ihl+8 {
		return true
	}
	if binary.BigEndian.Uint16(pkt[6:8])&0x3fff != 0 { // fragment: let it through
		return true
	}
	dst := pkt[16:20]
	if dst[0] >= 224 || (dst[0] == 255 && dst[1] == 255) { // multicast/broadcast: local network
		return true
	}
	switch pkt[9] {
	case 6:
		return e.handleTCP(pkt, ihl, addr)
	case 17:
		return e.handleUDP(pkt, ihl, addr)
	}
	return true
}

func (e *captureEngine) handleIPv6(pkt []byte) bool {
	if len(pkt) < 44 {
		return true
	}
	next := pkt[6]
	if next != 6 && next != 17 {
		return true
	}
	sport := binary.BigEndian.Uint16(pkt[40:42])
	if target, _ := e.isTarget(next == 6, true, sport); target {
		return false // block: the app falls back to IPv4, which goes through the VPN
	}
	return true
}
