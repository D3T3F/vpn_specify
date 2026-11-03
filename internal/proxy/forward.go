package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"vpn_specify/internal/i18n"

	"vpn_specify/internal/tunnel"
)

// ---------- Port forwarding (-L) ----------
//
// -L udp:5353=10.0.0.5:53   local UDP port 5353 -> 10.0.0.5:53 inside the VPN
// -L tcp:127.0.0.1:3389=srv:3389
// Works with any program: just point it at the local port.

type Forward struct {
	Proto, Local, Remote string
}

func (f Forward) String() string { return f.Proto + ":" + f.Local + "=" + f.Remote }

type ForwardList []Forward

func (l *ForwardList) String() string { return fmt.Sprint(*l) }

func (l *ForwardList) Set(s string) error {
	f := Forward{Proto: "tcp"}
	if p, rest, ok := strings.Cut(s, ":"); ok && (p == "tcp" || p == "udp") {
		f.Proto, s = p, rest
	}
	local, remote, ok := strings.Cut(s, "=")
	if !ok {
		return i18n.Error("format: [tcp|udp:]localPort=host:port")
	}
	if _, err := strconv.Atoi(local); err == nil {
		local = "127.0.0.1:" + local
	}
	if _, _, err := net.SplitHostPort(local); err != nil {
		return i18n.Error("invalid local address {0}", strconv.Quote(local))
	}
	if _, _, err := net.SplitHostPort(remote); err != nil {
		return i18n.Error("invalid destination {0} (use host:port)", strconv.Quote(remote))
	}
	f.Local, f.Remote = local, remote
	*l = append(*l, f)
	return nil
}

// ListenForward only opens the local port (before connecting, to fail fast).
func ListenForward(f Forward) (io.Closer, error) {
	if f.Proto == "tcp" {
		return net.Listen("tcp", f.Local)
	}
	la, err := net.ResolveUDPAddr("udp", f.Local)
	if err != nil {
		return nil, err
	}
	return net.ListenUDP("udp", la)
}

// StartForward opens the local port and serves in the background.
func StartForward(f Forward, tun tunnel.Tunnel, verbose bool) (io.Closer, error) {
	c, err := ListenForward(f)
	if err != nil {
		return nil, err
	}
	ServeForward(c, f, tun, verbose)
	return c, nil
}

func ServeForward(c io.Closer, f Forward, tun tunnel.Tunnel, verbose bool) {
	if ln, ok := c.(net.Listener); ok {
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go func() {
					defer c.Close()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					up, err := tun.DialContext(ctx, "tcp", f.Remote)
					cancel()
					if err != nil {
						if verbose {
							log.Printf("forward %s: %v", f, err)
						}
						return
					}
					defer up.Close()
					Pipe(c, c, up)
				}()
			}
		}()
		return
	}
	go serveUDPForward(c.(*net.UDPConn), tun, f, verbose)
}

const UDPIdleTimeout = 2 * time.Minute

// serveUDPForward does a "NAT": each local source address gets its own
// socket inside the VPN, so replies go back to whoever sent the request.
func serveUDPForward(pc *net.UDPConn, tun tunnel.Tunnel, f Forward, verbose bool) {
	host, portStr, _ := net.SplitHostPort(f.Remote)
	port, _ := strconv.Atoi(portStr)
	resolver := newUDPResolver(tun)
	var mu sync.Mutex
	sessions := map[string]net.PacketConn{}

	buf := make([]byte, 64*1024)
	for {
		n, from, err := pc.ReadFromUDP(buf)
		if err != nil {
			mu.Lock()
			for _, s := range sessions {
				s.Close()
			}
			mu.Unlock()
			return
		}
		dst, err := resolver.resolve(host, port)
		if err != nil {
			if verbose {
				log.Printf("forward %s: %v", f, err)
			}
			continue
		}
		key := from.String()
		mu.Lock()
		s, ok := sessions[key]
		if !ok {
			if s, err = tun.ListenUDP(); err != nil {
				mu.Unlock()
				continue
			}
			sessions[key] = s
			go func(s net.PacketConn, client *net.UDPAddr) {
				defer func() {
					mu.Lock()
					delete(sessions, key)
					mu.Unlock()
					s.Close()
				}()
				rb := make([]byte, 64*1024)
				for {
					_ = s.SetReadDeadline(time.Now().Add(UDPIdleTimeout))
					n, _, err := s.ReadFrom(rb)
					if err != nil {
						return
					}
					_, _ = pc.WriteToUDP(rb[:n], client)
				}
			}(s, from)
		}
		mu.Unlock()
		_, _ = s.WriteTo(buf[:n], dst)
	}
}
