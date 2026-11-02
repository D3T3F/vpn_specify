package tunnel

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
)

// Tunnel é uma conexão VPN capaz de abrir conexões TCP "por dentro" dela.
type Tunnel interface {
	// DialContext abre uma conexão TCP pela VPN. addr é "host:porta"; o host
	// pode ser um nome, que é resolvido pelo DNS da VPN.
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
	// ListenUDP abre um socket UDP (IPv4) dentro da VPN, usado com WriteTo/ReadFrom.
	ListenUDP() (net.PacketConn, error)
	// LookupIP resolve um nome pelo DNS da VPN (IPv4).
	LookupIP(ctx context.Context, host string) (net.IP, error)
	// Done recebe um erro se a VPN cair.
	Done() <-chan error
	Close() error
}

type Options struct {
	DNS         []netip.Addr
	OpenVPNPath string
	Verbose     bool
	// NoConsole: sem terminal (interface gráfica). Esconde a janela do
	// openvpn e usa evento para desligar.
	NoConsole          bool
	AuthUser, AuthPass string
}

// logWriter manda cada linha escrita para o log.
type logWriter string

func (p logWriter) Write(b []byte) (int, error) {
	for _, l := range strings.Split(strings.TrimRight(string(b), "\r\n"), "\n") {
		if l = strings.TrimRight(l, "\r"); l != "" {
			log.Printf("%s%s", string(p), l)
		}
	}
	return len(b), nil
}

func Open(path string, o Options) (Tunnel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lower := bytes.ToLower(data)
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case bytes.Contains(lower, []byte("[interface]")):
		return OpenWireGuard(data, o)
	case ext == ".ovpn" || bytes.Contains(lower, []byte("\nremote ")) || bytes.HasPrefix(lower, []byte("remote ")):
		return openOpenVPN(path, data, o)
	}
	return nil, fmt.Errorf("%s: formato não reconhecido (esperado WireGuard .conf ou OpenVPN .ovpn)", path)
}
