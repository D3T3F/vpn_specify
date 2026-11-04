package tunnel

import (
	"bytes"
	"context"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"vpn_specify/internal/i18n"
)

// Tunnel is a VPN connection able to open connections "inside" it.
type Tunnel interface {
	// DialContext opens a TCP connection through the VPN. addr is "host:port";
	// the host may be a name, resolved by the VPN's DNS.
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
	// ListenUDP opens a UDP socket (IPv4) inside the VPN, used with WriteTo/ReadFrom.
	ListenUDP() (net.PacketConn, error)
	// LookupIP resolves a name with the VPN's DNS (IPv4).
	LookupIP(ctx context.Context, host string) (net.IP, error)
	// Done receives an error if the VPN goes down.
	Done() <-chan error
	Close() error
}

type Options struct {
	DNS         []netip.Addr
	OpenVPNPath string
	Verbose     bool
	// NoConsole: no terminal (graphical interface). Hides the openvpn
	// window and uses an event (Windows) or the helper (Linux) to stop it.
	NoConsole          bool
	AuthUser, AuthPass string
	// InstallOpenVPN, when set, is called if openvpn is missing: it asks the
	// user and installs it. Returning ErrOpenVPNMissing means "declined".
	InstallOpenVPN func(progress func(string)) error
	// Progress, when set, receives short messages about slow preparation steps.
	Progress func(string)
}

// Errors the caller may want to recognize (e.g. not to retry them).
var (
	// ErrNeedsCredentials: the VPN asks for a user and password and none is saved.
	ErrNeedsCredentials = i18n.Sentinel("this VPN asks for a user and password: fill in the configuration's credentials")
	// ErrAdminDenied: the administrator password prompt was refused.
	ErrAdminDenied = i18n.Sentinel("administrator permission denied (OpenVPN needs it to create the tun interface)")
	// ErrOpenVPNMissing: openvpn is not installed (and was not installed now).
	ErrOpenVPNMissing = openVPNMissing()
	// ErrNeedsAdmin: OpenVPN mode started without root in a terminal.
	ErrNeedsAdmin = i18n.Sentinel("OpenVPN mode needs root (OpenVPN creates a tun interface): run with sudo. WireGuard mode does not")
)

// logWriter sends each written line to the log.
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
	return nil, i18n.Error("{0}: unrecognized format (expected WireGuard .conf or OpenVPN .ovpn)", path)
}
