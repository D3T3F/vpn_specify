package vpngate

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const goodConfig = `###############
# OpenVPN 2.0 Sample Configuration File
###############
dev tun
proto %s
remote %s %d
cipher AES-128-CBC
data-ciphers AES-128-CBC
auth SHA1
resolv-retry infinite
nobind
persist-key
persist-tun
client
verb 3
#auth-user-pass
<ca>
-----BEGIN CERTIFICATE-----
up /bin/sh
-----END CERTIFICATE-----
</ca>
<key>
abc
</key>
`

func row(host, ip, long, short string, score int, config string) string {
	return fmt.Sprintf("%s,%s,%d,12,166647166,%s,%s,124,987318564,13590785,790723194329799,2weeks,Someone_ Japan.,,%s",
		host, ip, score, long, short, base64.StdEncoding.EncodeToString([]byte(config)))
}

func csvOf(rows ...string) []byte {
	head := "*vpn_servers\r\n#HostName,IP,Score,Ping,Speed,CountryLong,CountryShort,NumVpnSessions,Uptime,TotalUsers,TotalTraffic,LogType,Operator,Message,OpenVPN_ConfigData_Base64\r\n"
	return []byte(head + strings.Join(rows, "\r\n") + "\r\n*\r\n")
}

func TestParse(t *testing.T) {
	servers, skipped, err := Parse(csvOf(
		row("public-vpn-1", "219.100.37.1", "Japan", "JP", 10, fmt.Sprintf(goodConfig, "tcp", "219.100.37.1", 443)),
		row("vpn2", "59.21.104.173", "Korea Republic of", "KR", 5, fmt.Sprintf(goodConfig, "udp", "59.21.104.173", 1348)),
	))
	if err != nil || skipped != 0 || len(servers) != 2 {
		t.Fatalf("servers %d, skipped %d, err %v", len(servers), skipped, err)
	}
	s := servers[0]
	if s.Proto != "tcp" || s.Port != 443 || s.Country != "JP" || s.Speed != 166647166 || s.Sessions != 124 {
		t.Errorf("wrong fields: %+v", s)
	}
	if s.Name != "VPN Gate JP public-vpn-1 TCP" {
		t.Errorf("name: %q", s.Name)
	}
	if strings.Contains(string(s.config), "mssfix") || !strings.Contains(string(servers[1].config), "mssfix 1200") {
		t.Error("only UDP servers need smaller TCP packets")
	}
	if servers[1].Proto != "udp" || servers[1].Port != 1348 {
		t.Errorf("wrong fields: %+v", servers[1])
	}
}

func TestParseRejectsOtherFormats(t *testing.T) {
	for _, in := range []string{"", "<html>blocked</html>", "a,b,c\n"} {
		if _, _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
}

// A configuration can make openvpn run programs, as Administrator on the
// desktop: anything beyond the plain client options drops the server.
func TestVetLeavesOutUnsafeConfigurations(t *testing.T) {
	ok := fmt.Sprintf(goodConfig, "tcp", "1.2.3.4", 443)
	if _, _, good := vet([]byte(ok), "1.2.3.4"); !good {
		t.Fatal("the normal configuration must pass")
	}
	bad := map[string]string{
		"script-security": ok + "script-security 2\n",
		"up":              ok + "up /tmp/x.sh\n",
		"--up":            ok + "--up /tmp/x.sh\n",
		"plugin":          ok + "plugin x.dll\n",
		"config":          ok + "config other.ovpn\n",
		"log":             ok + "log C:\\Windows\\x\n",
		"route-up":        ok + "route-up x\n",
		"unknown block":   ok + "<connection>\nremote 9.9.9.9 1\n</connection>\n",
		"open block":      ok + "<ca>\n",
		"second remote":   ok + "remote 1.2.3.4 444\n",
		"other address":   strings.Replace(ok, "remote 1.2.3.4", "remote 9.9.9.9", 1),
		"host name":       strings.Replace(ok, "remote 1.2.3.4", "remote evil.example", 1),
		"bad port":        strings.Replace(ok, "443", "0", 1),
		"no remote":       strings.Replace(ok, "remote 1.2.3.4 443", "", 1),
		"no protocol":     strings.Replace(ok, "proto tcp", "", 1),
		"tap":             strings.Replace(ok, "dev tun", "dev tap", 1),
		"ca from a file":  ok + "ca C:\\x.pem\n",
	}
	for name, cfg := range bad {
		if _, _, good := vet([]byte(cfg), "1.2.3.4"); good {
			t.Errorf("%s: must be left out", name)
		}
	}
	// the unsafe row is counted, and the others stay
	servers, skipped, err := Parse(csvOf(
		row("good", "1.2.3.4", "Japan", "JP", 1, ok),
		row("bad", "1.2.3.4", "Japan", "JP", 1, ok+"up x\n"),
		row("../evil", "1.2.3.4", "Japan", "JP", 1, ok),
		"short,row",
	))
	if err != nil || len(servers) != 1 || skipped != 3 {
		t.Errorf("servers %d, skipped %d, err %v", len(servers), skipped, err)
	}
}

func TestOrderClosestFirst(t *testing.T) {
	mk := func(code string, score int64) Server { return Server{ID: code, Country: code, Score: score} }
	servers := []Server{mk("JP", 900), mk("US", 500), mk("AR", 100), mk("ZZ", 999), mk("RU", 800), mk("BR", 5), mk("BR", 50)}
	Order(servers, "BR")
	var got []string
	for _, s := range servers {
		got = append(got, s.Country)
	}
	if s := strings.Join(got, " "); s != "BR BR AR US RU JP ZZ" {
		t.Errorf("order: %s", s)
	}
	if servers[0].Score != 50 {
		t.Error("inside a country, the best score comes first")
	}
	Order(servers, "")
	if servers[0].Country != "ZZ" || servers[1].Country != "JP" {
		t.Errorf("an unknown country must order by score: %+v", servers[:2])
	}
}

func TestUserCountry(t *testing.T) {
	cases := map[string]string{
		"Your IP: 1.2.3.4<BR>Your country: Brazil<BR><a href='#LIST'>Let's": "BR",
		"Your country: <b>Korea Republic of</b>":                            "KR",
		"Your country: Russian Federation\r\n":                              "RU",
		"Your country: Atlantis<BR>":                                        "",
		"no such text":                                                      "",
	}
	for page, want := range cases {
		if got := CountryOfHome([]byte(page)); got != want {
			t.Errorf("%q: got %q, want %q", page, got, want)
		}
	}
	regions := map[string]string{
		"pt-BR": "BR", "pt_BR.UTF-8": "BR", "zh-Hans-CN": "CN", "und-AR": "AR", "en": "", "C.UTF-8": "", "und-": "", "xx-ZZ": "",
	}
	for l, want := range regions {
		if got := RegionOfLocales([]string{l}); got != want {
			t.Errorf("%q: got %q, want %q", l, got, want)
		}
	}
}

func TestBuildFallsBackToTheLocale(t *testing.T) {
	data := csvOf(row("a", "1.2.3.4", "Japan", "JP", 1, fmt.Sprintf(goodConfig, "tcp", "1.2.3.4", 443)))
	l, err := Build(data, nil, []string{"es-AR"})
	if err != nil || l.Near != "AR" {
		t.Fatalf("near %q, err %v", l.Near, err)
	}
	l, err = Build(data, []byte("Your country: Brazil<BR>"), []string{"es-AR"})
	if err != nil || l.Near != "BR" {
		t.Fatalf("the page must win over the locale: near %q, err %v", l.Near, err)
	}
	s, cfg, err := l.Config("a")
	if err != nil || s.Host != "a" || !strings.Contains(string(cfg), "remote 1.2.3.4 443") {
		t.Fatalf("config: %v %v", s, err)
	}
	if _, _, err := l.Config("gone"); err == nil {
		t.Error("a server that left the list must give an error")
	}
}

// TestLive really asks VPN Gate. It only runs with VPN_SPECIFY_LIVE=1 and
// tells whether vet still accepts what the API sends.
func TestLive(t *testing.T) {
	if os.Getenv("VPN_SPECIFY_LIVE") != "1" {
		t.Skip("set VPN_SPECIFY_LIVE=1 to test against VPN Gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	l, err := Fetch(ctx, []string{"pt-BR"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d servers, %d left out, closest to %s", len(l.Servers), l.Skipped, l.Near)
	if len(l.Servers) == 0 || l.Skipped > 0 {
		t.Errorf("servers %d, left out %d", len(l.Servers), l.Skipped)
	}
	for i, s := range l.Servers {
		if i < 12 || i%15 == 0 {
			t.Logf("%-2s %-6s %-4s %5d %s", s.Country, s.Proto, s.Host, s.Port, s.Name)
		}
	}
}
