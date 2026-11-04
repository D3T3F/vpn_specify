package tunnel

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
)

func TestParseWGConfigErrors(t *testing.T) {
	if _, err := ParseWireGuardConfig([]byte("[Interface]\nAddress=10.0.0.2/32\n")); err == nil {
		t.Fatal("expected an error without PrivateKey")
	}
}

func TestSanitizeOVPN(t *testing.T) {
	in := `client
dev tun
remote vpn.example.com 1194
redirect-gateway def1
block-outside-dns
dhcp-option DNS 10.1.1.1
route 10.0.0.0 255.0.0.0
<ca>
-----BEGIN CERTIFICATE-----
route this-is-not-a-directive
-----END CERTIFICATE-----
</ca>
`
	out, dns, _ := sanitizeOVPN([]byte(in))
	s := string(out)
	for _, bad := range []string{"\nredirect-gateway", "\nblock-outside-dns", "\ndhcp-option", "\nroute 10."} {
		if strings.Contains(s, bad) {
			t.Errorf("should have removed %q:\n%s", bad, s)
		}
	}
	if !strings.Contains(s, "route this-is-not-a-directive") || !strings.Contains(s, "remote vpn.example.com 1194") {
		t.Errorf("removed too much:\n%s", s)
	}
	if len(dns) != 1 || dns[0].String() != "10.1.1.1" {
		t.Errorf("dns = %v", dns)
	}
}

func TestParsePushReply(t *testing.T) {
	line := `2026-11-02 10:00:00 PUSH: Received control message: 'PUSH_REPLY,redirect-gateway def1,dhcp-option DNS 10.8.0.1,dhcp-option DNS 10.8.0.2,route-gateway 10.8.0.1,topology subnet,ping 10,ifconfig 10.8.0.6 255.255.255.0,peer-id 0'`
	ip, dns := parsePushReply(line)
	if ip.String() != "10.8.0.6" {
		t.Errorf("ip = %v", ip)
	}
	if len(dns) != 2 || dns[1].String() != "10.8.0.2" {
		t.Errorf("dns = %v", dns)
	}
}

func TestLookupARealDNS(t *testing.T) {
	if testing.Short() {
		t.Skip("needs internet")
	}
	ips, err := lookupA(context.Background(), []netip.Addr{netip.MustParseAddr("1.1.1.1")}, "one.one.one.one",
		func(ctx context.Context, n, a string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, n, a)
		})
	if err != nil || len(ips) == 0 {
		t.Fatalf("lookupA: %v %v", ips, err)
	}
}

// After openvpn reconnects by itself, new connections must use what the server
// gave this time; the old address no longer exists.
func TestAdoptAfterReconnection(t *testing.T) {
	old := netip.MustParseAddr("10.0.0.1")
	tun := &ovpnTunnel{
		localIP: net.ParseIP("10.211.1.21"),
		dns:     []netip.Addr{old},
		cache:   map[string]dnsCacheEntry{"a.example": {}},
	}
	fresh := netip.MustParseAddr("10.0.0.2")
	tun.adopt(net.ParseIP("127.0.0.1"), []netip.Addr{fresh}, nil)
	ip, _, dns := tun.network()
	if !ip.Equal(net.ParseIP("127.0.0.1")) || len(dns) != 1 || dns[0] != fresh {
		t.Fatalf("ip %v, dns %v", ip, dns)
	}
	if a := tun.dialer("tcp").LocalAddr.(*net.TCPAddr); !a.IP.Equal(ip) {
		t.Errorf("the dialer still uses %v", a.IP)
	}
	if len(tun.cache) != 0 {
		t.Error("names resolved by the old connection must be forgotten")
	}
	// a reconnection without DNS keeps the current servers, and -dns always wins
	tun.adopt(net.ParseIP("127.0.0.1"), nil, nil)
	if _, _, dns := tun.network(); len(dns) != 1 || dns[0] != fresh {
		t.Errorf("dns lost: %v", dns)
	}
	tun.adopt(net.ParseIP("127.0.0.1"), []netip.Addr{fresh}, []netip.Addr{old})
	if _, _, dns := tun.network(); dns[0] != old {
		t.Errorf("-dns must win: %v", dns)
	}
}
