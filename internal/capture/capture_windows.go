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

	"vpn_specify/internal/platform"
	"vpn_specify/internal/tunnel"
)

// Captura por processo (-capture discord.exe)
//
// O WinDivert entrega ao vpn_specify todo pacote IP de saída do sistema.
// Para cada pacote decidimos, pela porta local, se ele é de um processo
// capturado (tabelas de conexões do Windows):
//
//   - Não é: o pacote volta inalterado para a rede.
//   - UDP capturado (voz, jogos, DNS): o pacote é descartado e o conteúdo
//     sai por um socket UDP dentro da VPN. As respostas viram pacotes IP
//     "de entrada" injetados de volta, como se viessem da internet.
//   - TCP capturado: o pacote é "refletido" para um proxy transparente
//     local (troca origem/destino e aponta para a porta do proxy). O proxy
//     sabe o destino original pela tabela NAT e abre a conexão pela VPN.
//   - IPv6 capturado: descartado, para o app usar IPv4 (sem vazar fora da VPN).

const captureRule = "vpn_specify captura"

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
	tcp      map[uint16]*tcpFlow // chave: porta local do app
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
		return nil, errors.New("-capture precisa de um terminal como Administrador (usa o driver WinDivert)")
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

	log.Printf("captura ativa para: %s (TCP, UDP; IPv6 bloqueado)", strings.Join(e.Names(), ", "))
	e.resetRunning(e.names)
	return e, nil
}

// resetRunning derruba as conexões TCP de processos que já estavam abertos,
// para reconectarem (agora capturados).
func (e *captureEngine) resetRunning(names map[string]bool) {
	if running := platform.RunningPIDs(names); len(running) > 0 {
		n := resetTCPConnections(running)
		log.Printf("aviso: %d processo(s) já estavam abertos; %d conexão(ões) TCP antigas foram derrubadas para reconectarem pela VPN. "+
			"Se algo continuar fora da VPN, feche e abra o app de novo.", len(running), n)
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
		e.owners = map[string]ownerEntry{} // reavalia portas com a lista nova
	}
	e.mu.Unlock()
	if len(added) > 0 {
		log.Printf("captura: adicionado %s", strings.Join(sortedKeys(added), ", "))
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
		log.Printf("captura encerrada")
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

// O pacote refletido chega ao proxy como uma conexão de entrada vinda do
// IP remoto; sem regra, o Firewall do Windows bloqueia.
func (e *captureEngine) addFirewallRule() {
	exe, _ := os.Executable()
	e.deleteFirewallRule()
	out, err := exec.Command("netsh", "advfirewall", "firewall", "add", "rule", "name="+captureRule,
		"dir=in", "action=allow", "program="+exe, "protocol=TCP", "localport="+strconv.Itoa(int(e.proxyPort)), "enable=yes").CombinedOutput()
	if err != nil {
		log.Printf("aviso: não consegui criar a regra de firewall (%v): %s", err, strings.TrimSpace(string(out)))
	}
}

func (e *captureEngine) deleteFirewallRule() {
	_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+captureRule).Run()
}

// isTarget diz se a porta local pertence a um processo capturado.
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

// handle devolve true se o pacote (possivelmente modificado) deve ser enviado.
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
	if binary.BigEndian.Uint16(pkt[6:8])&0x3fff != 0 { // fragmento: deixa passar
		return true
	}
	dst := pkt[16:20]
	if dst[0] >= 224 || (dst[0] == 255 && dst[1] == 255) { // multicast/broadcast: rede local
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
		return false // bloqueia: o app cai para IPv4, que vai pela VPN
	}
	return true
}
