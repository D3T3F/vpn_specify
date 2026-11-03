//go:build windows

package capture

import (
	"encoding/binary"
	"errors"
	"log"
	"math/rand/v2"
	"net"
	"strconv"
	"time"

	"vpn_specify/internal/proxy"
)

func (e *captureEngine) handleUDP(pkt []byte, ihl int, addr *divertAddr) bool {
	l4 := pkt[ihl:]
	sport := binary.BigEndian.Uint16(l4[0:2])
	dport := binary.BigEndian.Uint16(l4[2:4])
	ulen := int(binary.BigEndian.Uint16(l4[4:6]))
	if ulen < 8 || ulen > len(l4) {
		return true
	}

	e.mu.Lock()
	f := e.udp[sport]
	e.mu.Unlock()
	if f == nil {
		target, name := e.isTarget(false, false, sport)
		if !target {
			return true
		}
		pc, err := e.tun.ListenUDP()
		if err != nil {
			log.Printf("capture udp: %v", err)
			return false
		}
		f = &udpFlow{lport: sport, pc: pc}
		e.mu.Lock()
		e.udp[sport] = f
		e.mu.Unlock()
		log.Printf("capture udp %s :%d -> %s (through the VPN)", name, sport, net.JoinHostPort(net.IP(pkt[16:20]).String(), strconv.Itoa(int(dport))))
		go e.udpReadLoop(f)
	}
	copy(f.local[:], pkt[12:16])
	f.ifIdx, f.subIdx = addr.IfIdx, addr.SubIfIdx
	f.last.Store(time.Now().UnixNano())

	dst := &net.UDPAddr{IP: net.IPv4(pkt[16], pkt[17], pkt[18], pkt[19]).To4(), Port: int(dport)}
	_, _ = f.pc.WriteTo(l4[8:ulen], dst)
	return false // the original never leaves through the normal network
}

// udpReadLoop receives the VPN replies and injects them as inbound packets.
func (e *captureEngine) udpReadLoop(f *udpFlow) {
	defer func() {
		e.mu.Lock()
		if e.udp[f.lport] == f {
			delete(e.udp, f.lport)
		}
		e.mu.Unlock()
		f.pc.Close()
	}()
	buf := make([]byte, 0xFFFF)
	for {
		_ = f.pc.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, from, err := f.pc.ReadFrom(buf[28:])
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() && time.Since(time.Unix(0, f.last.Load())) < proxy.UDPIdleTimeout {
				continue
			}
			return
		}
		ua, ok := from.(*net.UDPAddr)
		if !ok || ua.IP.To4() == nil {
			continue
		}
		pkt := buf[:28+n]
		// IPv4 header
		pkt[0], pkt[1] = 0x45, 0
		binary.BigEndian.PutUint16(pkt[2:], uint16(len(pkt)))
		binary.BigEndian.PutUint16(pkt[4:], uint16(rand.Uint32()))
		binary.BigEndian.PutUint16(pkt[6:], 0)
		pkt[8], pkt[9] = 64, 17
		binary.BigEndian.PutUint16(pkt[10:], 0)
		copy(pkt[12:16], ua.IP.To4())
		copy(pkt[16:20], f.local[:])
		// UDP header
		binary.BigEndian.PutUint16(pkt[20:], uint16(ua.Port))
		binary.BigEndian.PutUint16(pkt[22:], f.lport)
		binary.BigEndian.PutUint16(pkt[24:], uint16(8+n))
		binary.BigEndian.PutUint16(pkt[26:], 0)

		addr := divertAddr{IfIdx: f.ifIdx, SubIfIdx: f.subIdx}
		addr.setOutbound(false)
		divertChecksums(pkt, &addr)
		_ = e.h.send(pkt, &addr)
	}
}

// janitor removes finished or idle TCP flows.
func (e *captureEngine) janitor() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-t.C:
		}
		now := time.Now()
		e.mu.Lock()
		for k, f := range e.tcp {
			idle := now.Sub(time.Unix(0, f.last.Load()))
			if (f.done.Load() && idle > 30*time.Second) || idle > 15*time.Minute {
				delete(e.tcp, k)
			}
		}
		for k, o := range e.owners {
			if now.After(o.expires) {
				delete(e.owners, k)
			}
		}
		e.mu.Unlock()
	}
}
