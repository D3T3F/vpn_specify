package tunnel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/textutil"
)

// WireGuard runs entirely in user space: the VPN's TCP/IP stack (gVisor
// netstack) lives inside this process. No network adapter is created and no
// system route is changed, so no Administrator/root is needed.

type wgTunnel struct {
	dev   *device.Device
	tnet  *netstack.Net
	addr4 netip.Addr // first IPv4 of [Interface] Address
}

type wgPeer struct {
	publicKey, presharedKey, endpoint, keepalive string
	allowedIPs                                   []string
}

type wgConfig struct {
	privateKey string
	addrs      []netip.Addr
	dns        []netip.Addr
	mtu        int
	peers      []*wgPeer
}

func (c *wgConfig) ipc() string {
	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\nreplace_peers=true\n", c.privateKey)
	for _, p := range c.peers {
		fmt.Fprintf(&b, "public_key=%s\n", p.publicKey)
		if p.presharedKey != "" {
			fmt.Fprintf(&b, "preshared_key=%s\n", p.presharedKey)
		}
		if p.endpoint != "" {
			fmt.Fprintf(&b, "endpoint=%s\n", p.endpoint)
		}
		if p.keepalive != "" {
			fmt.Fprintf(&b, "persistent_keepalive_interval=%s\n", p.keepalive)
		}
		b.WriteString("replace_allowed_ips=true\n")
		for _, a := range p.allowedIPs {
			fmt.Fprintf(&b, "allowed_ip=%s\n", a)
		}
	}
	return b.String()
}

func ParseWireGuardConfig(data []byte) (*wgConfig, error) {
	cfg := &wgConfig{mtu: 1420}
	var cur *wgPeer
	section := ""
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[] "))
			if section == "peer" {
				cur = &wgPeer{}
				cfg.peers = append(cfg.peers, cur)
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, i18n.Error("invalid line {0}: {1}", n, strconv.Quote(line))
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		var err error
		switch section + "." + k {
		case "interface.privatekey":
			cfg.privateKey, err = keyToHex(v)
		case "interface.address":
			for _, s := range textutil.SplitList(v) {
				var a netip.Addr
				if p, perr := netip.ParsePrefix(s); perr == nil {
					a = p.Addr()
				} else if a, err = netip.ParseAddr(s); err != nil {
					return nil, i18n.Error("line {0}: invalid address {1}", n, strconv.Quote(s))
				}
				cfg.addrs = append(cfg.addrs, a)
			}
		case "interface.dns":
			for _, s := range textutil.SplitList(v) {
				if a, perr := netip.ParseAddr(s); perr == nil { // skip search domains
					cfg.dns = append(cfg.dns, a)
				}
			}
		case "interface.mtu":
			cfg.mtu, err = strconv.Atoi(v)
		case "peer.publickey":
			cur.publicKey, err = keyToHex(v)
		case "peer.presharedkey":
			cur.presharedKey, err = keyToHex(v)
		case "peer.endpoint":
			cur.endpoint = v
		case "peer.persistentkeepalive":
			if _, err = strconv.Atoi(v); err == nil {
				cur.keepalive = v
			}
		case "peer.allowedips":
			for _, s := range textutil.SplitList(v) {
				p, perr := netip.ParsePrefix(s)
				if perr != nil {
					return nil, i18n.Error("line {0}: invalid AllowedIPs {1}", n, strconv.Quote(s))
				}
				cur.allowedIPs = append(cur.allowedIPs, p.String())
			}
		}
		if err != nil {
			return nil, i18n.Wrap(err, "line {0} ({1})", n, k)
		}
	}
	if cfg.privateKey == "" {
		return nil, i18n.Error("PrivateKey missing in [Interface]")
	}
	if len(cfg.addrs) == 0 {
		return nil, i18n.Error("Address missing in [Interface]")
	}
	if len(cfg.peers) == 0 {
		return nil, i18n.Error("no [Peer] in the configuration")
	}
	for _, p := range cfg.peers {
		if p.publicKey == "" {
			return nil, i18n.Error("[Peer] without PublicKey")
		}
	}
	return cfg, nil
}

func keyToHex(b64 string) (string, error) {
	k, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(k) != 32 {
		return "", i18n.Error("invalid key (expected base64 of 32 bytes)")
	}
	return hex.EncodeToString(k), nil
}

// resolveEndpoint resolves the VPN server hostname with the computer's normal
// DNS (the tunnel does not exist yet at that point).
func resolveEndpoint(ep string) (string, error) {
	host, port, err := net.SplitHostPort(ep)
	if err != nil {
		return "", i18n.Wrap(err, "invalid Endpoint {0}", strconv.Quote(ep))
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return ep, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", i18n.Wrap(err, "could not resolve the Endpoint {0}", host)
	}
	best := ips[0]
	for _, ip := range ips {
		if ip.To4() != nil {
			best = ip
			break
		}
	}
	return net.JoinHostPort(best.String(), port), nil
}

func OpenWireGuard(data []byte, o Options) (Tunnel, error) {
	cfg, err := ParseWireGuardConfig(data)
	if err != nil {
		return nil, i18n.Wrap(err, "WireGuard configuration")
	}
	for _, p := range cfg.peers {
		if p.endpoint != "" {
			if p.endpoint, err = resolveEndpoint(p.endpoint); err != nil {
				return nil, err
			}
		}
	}
	dns := cfg.dns
	if len(o.DNS) > 0 {
		dns = o.DNS
	}
	if len(dns) == 0 {
		dns = []netip.Addr{netip.MustParseAddr("1.1.1.1")}
		log.Printf("warning: the configuration sets no DNS; using 1.1.1.1 (inside the VPN)")
	}

	tdev, tnet, err := netstack.CreateNetTUN(cfg.addrs, dns, cfg.mtu)
	if err != nil {
		return nil, i18n.Wrap(err, "creating the virtual network stack")
	}
	level := device.LogLevelError
	if o.Verbose {
		level = device.LogLevelVerbose
	}
	dev := device.NewDevice(tdev, conn.NewDefaultBind(), device.NewLogger(level, "[wireguard] "))
	if err := dev.IpcSet(cfg.ipc()); err != nil {
		dev.Close()
		return nil, i18n.Wrap(err, "applying the WireGuard configuration")
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, err
	}
	t := &wgTunnel{dev: dev, tnet: tnet}
	for _, a := range cfg.addrs {
		if a.Is4() {
			t.addr4 = a
			break
		}
	}
	log.Printf("WireGuard: address %v, DNS %v", cfg.addrs, dns)
	if err := t.waitHandshake(15 * time.Second); err != nil {
		log.Printf("warning: %v", err)
	} else {
		log.Printf("WireGuard: handshake with the server OK")
	}
	return t, nil
}

// waitHandshake generates a bit of traffic (a DNS query) to trigger the
// handshake and waits for the server to answer.
func (t *wgTunnel) waitHandshake(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	go func() { _, _ = t.tnet.LookupContextHost(ctx, "www.google.com") }()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return i18n.Error("no handshake with the WireGuard server yet (check Endpoint, keys and firewall)")
		case <-tick.C:
			s, err := t.dev.IpcGet()
			if err != nil {
				continue
			}
			for _, line := range strings.Split(s, "\n") {
				if v, ok := strings.CutPrefix(line, "last_handshake_time_sec="); ok && v != "0" {
					return nil
				}
			}
		}
	}
}

func (t *wgTunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.tnet.DialContext(ctx, network, addr)
}

// ListenUDP binds the socket to the tunnel's IPv4 (in gVisor, 0.0.0.0 finds no route).
func (t *wgTunnel) ListenUDP() (net.PacketConn, error) {
	if !t.addr4.IsValid() {
		return nil, i18n.Error("the WireGuard configuration has no IPv4 address (UDP only supports IPv4)")
	}
	return t.tnet.ListenUDPAddrPort(netip.AddrPortFrom(t.addr4, 0))
}

func (t *wgTunnel) LookupIP(ctx context.Context, host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return ip, nil
	}
	addrs, err := t.tnet.LookupContextHost(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		if ip := net.ParseIP(a); ip != nil && ip.To4() != nil {
			return ip, nil
		}
	}
	return nil, i18n.Error("{0}: no IPv4 address", host)
}

func (t *wgTunnel) Done() <-chan error { return nil } // WireGuard does not "go down": it reconnects by itself

func (t *wgTunnel) Close() error {
	t.dev.Close()
	return nil
}
