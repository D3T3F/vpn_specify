//go:build windows

package capture

import (
	"context"
	"encoding/binary"
	"log"
	"net"
	"strconv"
	"time"

	"vpn_specify/internal/proxy"
)

func (e *captureEngine) handleTCP(pkt []byte, ihl int, addr *divertAddr) bool {
	l4 := pkt[ihl:]
	if len(l4) < 20 {
		return true
	}
	sport := binary.BigEndian.Uint16(l4[0:2])
	dport := binary.BigEndian.Uint16(l4[2:4])
	flags := l4[13]

	// Resposta do proxy transparente para o app: A:proxy -> D:sp vira D:dp -> A:sp (entrada).
	if sport == e.proxyPort {
		e.mu.Lock()
		f := e.tcp[dport]
		e.mu.Unlock()
		if f == nil {
			return false
		}
		copy(pkt[12:16], f.remote[:])
		copy(pkt[16:20], f.local[:])
		binary.BigEndian.PutUint16(l4[0:2], f.rport)
		addr.setOutbound(false)
		addr.IfIdx, addr.SubIfIdx = f.ifIdx, f.subIdx
		divertChecksums(pkt, addr)
		f.last.Store(time.Now().UnixNano())
		return true
	}

	syn := flags&0x02 != 0 && flags&0x10 == 0
	var src, dst [4]byte
	copy(src[:], pkt[12:16])
	copy(dst[:], pkt[16:20])

	e.mu.Lock()
	f := e.tcp[sport]
	e.mu.Unlock()
	if f != nil && (f.remote != dst || f.rport != dport) {
		f = nil // porta reutilizada para outra conexão
	}
	if f == nil {
		if !syn {
			return true // conexão que não é nossa (ou anterior à captura)
		}
		target, name := e.isTarget(true, false, sport)
		if !target {
			return true
		}
		f = &tcpFlow{local: src, remote: dst, rport: dport, ifIdx: addr.IfIdx, subIdx: addr.SubIfIdx}
		e.mu.Lock()
		e.tcp[sport] = f
		e.mu.Unlock()
		if e.verbose {
			log.Printf("captura tcp %s :%d -> %s", name, sport, net.JoinHostPort(net.IP(dst[:]).String(), strconv.Itoa(int(dport))))
		}
	}
	f.last.Store(time.Now().UnixNano())

	// App -> proxy: A:sp -> D:dp vira D:sp -> A:proxy (entrada).
	copy(pkt[12:16], dst[:])
	copy(pkt[16:20], src[:])
	binary.BigEndian.PutUint16(l4[2:4], e.proxyPort)
	addr.setOutbound(false)
	divertChecksums(pkt, addr)
	return true
}

func (e *captureEngine) acceptLoop() {
	for {
		c, err := e.ln.Accept()
		if err != nil {
			return
		}
		go e.proxyConn(c)
	}
}

func (e *captureEngine) proxyConn(c net.Conn) {
	defer c.Close()
	ra := c.RemoteAddr().(*net.TCPAddr)
	e.mu.Lock()
	f := e.tcp[uint16(ra.Port)]
	e.mu.Unlock()
	if f == nil || !ra.IP.Equal(net.IP(f.remote[:])) {
		return // não veio de um fluxo capturado
	}
	defer f.done.Store(true)
	target := net.JoinHostPort(net.IP(f.remote[:]).String(), strconv.Itoa(int(f.rport)))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	up, err := e.tun.DialContext(ctx, "tcp", target)
	cancel()
	if err != nil {
		if e.verbose {
			log.Printf("captura tcp %s: %v", target, err)
		}
		return
	}
	defer up.Close()
	proxy.Pipe(c, c, up)
}
