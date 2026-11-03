package proxy

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"vpn_specify/internal/tunnel"
)

// ---------- SOCKS5 UDP ASSOCIATE (RFC 1928, section 7) ----------
//
// The client keeps the TCP control connection open and sends datagrams to a
// local UDP socket (the "relay"), each with a SOCKS header naming the
// destination. The relay forwards them through the VPN and returns replies
// with the sender's header. The association ends when the TCP connection
// closes.

func handleUDPAssociate(ctrl net.Conn, tun tunnel.Tunnel, verbose bool) {
	clientIP := ctrl.RemoteAddr().(*net.TCPAddr).IP
	relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: ctrl.LocalAddr().(*net.TCPAddr).IP})
	if err != nil {
		socksReply(ctrl, 1)
		return
	}
	defer relay.Close()
	remote, err := tun.ListenUDP()
	if err != nil {
		if verbose {
			log.Printf("socks udp: %v", err)
		}
		socksReply(ctrl, 1)
		return
	}
	defer remote.Close()

	ra := relay.LocalAddr().(*net.UDPAddr)
	rep := append([]byte{5, 0, 0, 1}, ra.IP.To4()...)
	rep = binary.BigEndian.AppendUint16(rep, uint16(ra.Port))
	if _, err := ctrl.Write(rep); err != nil {
		return
	}
	_ = ctrl.SetDeadline(time.Time{})
	if verbose {
		log.Printf("socks udp: association opened for %s", ctrl.RemoteAddr())
	}

	var client atomic.Pointer[net.UDPAddr]
	resolver := newUDPResolver(tun)

	// client -> VPN
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, from, err := relay.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if !from.IP.Equal(clientIP) {
				continue // only accept datagrams from the control connection's client
			}
			client.Store(from)
			host, port, data, err := ParseSocksUDP(buf[:n])
			if err != nil {
				continue
			}
			dst, err := resolver.resolve(host, port)
			if err != nil {
				if verbose {
					log.Printf("socks udp %s: %v", host, err)
				}
				continue
			}
			_, _ = remote.WriteTo(data, dst)
		}
	}()

	// VPN -> client
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, from, err := remote.ReadFrom(buf)
			if err != nil {
				return
			}
			c := client.Load()
			ua, ok := from.(*net.UDPAddr)
			if c == nil || !ok {
				continue
			}
			_, _ = relay.WriteToUDP(append(SocksUDPHeader(ua), buf[:n]...), c)
		}
	}()

	_, _ = io.Copy(io.Discard, ctrl) // wait for the client to close the control connection
}

func ParseSocksUDP(b []byte) (host string, port int, data []byte, err error) {
	if len(b) < 4 || b[2] != 0 { // fragmentation not supported
		return "", 0, nil, errors.New("invalid SOCKS datagram")
	}
	p := b[4:]
	switch b[3] {
	case 1:
		if len(p) < 4+2 {
			return "", 0, nil, errors.New("short datagram")
		}
		host, p = net.IP(p[:4]).String(), p[4:]
	case 4:
		if len(p) < 16+2 {
			return "", 0, nil, errors.New("short datagram")
		}
		host, p = net.IP(p[:16]).String(), p[16:]
	case 3:
		if len(p) < 1 || len(p) < 1+int(p[0])+2 {
			return "", 0, nil, errors.New("short datagram")
		}
		host, p = string(p[1:1+p[0]]), p[1+p[0]:]
	default:
		return "", 0, nil, errors.New("invalid address type")
	}
	return host, int(binary.BigEndian.Uint16(p[:2])), p[2:], nil
}

func SocksUDPHeader(a *net.UDPAddr) []byte {
	h := []byte{0, 0, 0}
	if ip4 := a.IP.To4(); ip4 != nil {
		h = append(append(h, 1), ip4...)
	} else {
		h = append(append(h, 4), a.IP.To16()...)
	}
	return binary.BigEndian.AppendUint16(h, uint16(a.Port))
}

// udpResolver caches resolved names (each datagram may carry a name).
type udpResolver struct {
	tun   tunnel.Tunnel
	mu    sync.Mutex
	cache map[string]net.IP
}

func newUDPResolver(tun tunnel.Tunnel) *udpResolver {
	return &udpResolver{tun: tun, cache: map[string]net.IP{}}
}

// udpAddr normalizes IPv4 to 4 bytes: gVisor treats a 16-byte IP
// (::ffff:a.b.c.d) as IPv6 and drops the send on an IPv4 socket.
func udpAddr(ip net.IP, port int) *net.UDPAddr {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	return &net.UDPAddr{IP: ip, Port: port}
}

func (r *udpResolver) resolve(host string, port int) (*net.UDPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		return udpAddr(ip, port), nil
	}
	r.mu.Lock()
	ip, ok := r.cache[host]
	r.mu.Unlock()
	if !ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var err error
		if ip, err = r.tun.LookupIP(ctx, host); err != nil {
			return nil, err
		}
		r.mu.Lock()
		r.cache[host] = ip
		r.mu.Unlock()
	}
	return udpAddr(ip, port), nil
}
