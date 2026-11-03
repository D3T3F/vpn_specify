package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"

	"vpn_specify/internal/tunnel"
)

// ---------- SOCKS5 (RFC 1928, no authentication) ----------

func ServeSOCKS(ln net.Listener, tun tunnel.Tunnel, verbose bool) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go handleSOCKS(c, tun, verbose)
	}
}

func socksReply(c net.Conn, code byte) {
	_, _ = c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
}

func handleSOCKS(c net.Conn, tun tunnel.Tunnel, verbose bool) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))
	br := bufio.NewReader(c)

	var hdr [2]byte
	if _, err := io.ReadFull(br, hdr[:]); err != nil || hdr[0] != 5 {
		return
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(br, methods); err != nil {
		return
	}
	if !bytes.Contains(methods, []byte{0}) {
		_, _ = c.Write([]byte{5, 0xff})
		return
	}
	_, _ = c.Write([]byte{5, 0})

	var req [4]byte
	if _, err := io.ReadFull(br, req[:]); err != nil || req[0] != 5 {
		return
	}
	var host string
	switch req[3] {
	case 1: // IPv4
		b := make([]byte, 4)
		if _, err := io.ReadFull(br, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case 3: // domain name (resolved through the VPN)
		l, err := br.ReadByte()
		if err != nil {
			return
		}
		b := make([]byte, l)
		if _, err := io.ReadFull(br, b); err != nil {
			return
		}
		host = string(b)
	case 4: // IPv6
		b := make([]byte, 16)
		if _, err := io.ReadFull(br, b); err != nil {
			return
		}
		host = net.IP(b).String()
	default:
		socksReply(c, 8)
		return
	}
	var pb [2]byte
	if _, err := io.ReadFull(br, pb[:]); err != nil {
		return
	}
	switch req[1] {
	case 1: // CONNECT
	case 3: // UDP ASSOCIATE
		handleUDPAssociate(c, tun, verbose)
		return
	default:
		socksReply(c, 7)
		return
	}
	target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb[:]))))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	up, err := tun.DialContext(ctx, "tcp", target)
	cancel()
	if err != nil {
		if verbose {
			log.Printf("socks %s: %v", target, err)
		}
		socksReply(c, socksErrCode(err))
		return
	}
	defer up.Close()
	socksReply(c, 0)
	_ = c.SetDeadline(time.Time{})
	if verbose {
		log.Printf("socks -> %s", target)
	}
	Pipe(c, br, up)
}

func socksErrCode(err error) byte {
	switch {
	case errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(err.Error(), "refused"):
		return 5
	case strings.Contains(err.Error(), "unreachable"):
		return 3
	default:
		return 4 // host unreachable
	}
}
