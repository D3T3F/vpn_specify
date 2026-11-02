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
		t.Fatal("esperava erro sem PrivateKey")
	}
}

func TestSanitizeOVPN(t *testing.T) {
	in := `client
dev tun
remote vpn.exemplo.com 1194
redirect-gateway def1
block-outside-dns
dhcp-option DNS 10.1.1.1
route 10.0.0.0 255.0.0.0
<ca>
-----BEGIN CERTIFICATE-----
route isto-nao-e-diretiva
-----END CERTIFICATE-----
</ca>
`
	out, dns, _ := sanitizeOVPN([]byte(in))
	s := string(out)
	for _, bad := range []string{"\nredirect-gateway", "\nblock-outside-dns", "\ndhcp-option", "\nroute 10."} {
		if strings.Contains(s, bad) {
			t.Errorf("deveria ter removido %q:\n%s", bad, s)
		}
	}
	if !strings.Contains(s, "route isto-nao-e-diretiva") || !strings.Contains(s, "remote vpn.exemplo.com 1194") {
		t.Errorf("removeu coisa demais:\n%s", s)
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
		t.Skip("precisa de internet")
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
