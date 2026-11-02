package proxy_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"golang.org/x/crypto/curve25519"
	xproxy "golang.org/x/net/proxy"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"vpn_specify/internal/netcheck"
	"vpn_specify/internal/proxy"
	"vpn_specify/internal/tunnel"
)

// directTunnel "conecta direto", para testar os proxies sem VPN.
type directTunnel struct{}

func (directTunnel) DialContext(ctx context.Context, n, a string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, n, a)
}
func (directTunnel) ListenUDP() (net.PacketConn, error) { return net.ListenUDP("udp4", nil) }
func (directTunnel) LookupIP(ctx context.Context, h string) (net.IP, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", h)
	if err != nil {
		return nil, err
	}
	return ips[0], nil
}
func (directTunnel) Done() <-chan error { return nil }
func (directTunnel) Close() error       { return nil }

func startProxies(t *testing.T, tun tunnel.Tunnel) (socksAddr, httpAddr string) {
	t.Helper()
	sl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sl.Close(); hl.Close() })
	go proxy.ServeSOCKS(sl, tun, false)
	go proxy.ServeHTTP(hl, tun, false)
	return sl.Addr().String(), hl.Addr().String()
}

func getVia(t *testing.T, target string, socksAddr, httpAddr string) {
	t.Helper()
	// via SOCKS5
	d, err := xproxy.SOCKS5("tcp", socksAddr, nil, xproxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	c := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		DialContext: d.(xproxy.ContextDialer).DialContext,
	}}
	checkBody(t, c, target)
	// via proxy HTTP (requisição normal)
	pu, _ := url.Parse("http://" + httpAddr)
	c = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
	checkBody(t, c, target)
	// via proxy HTTP (CONNECT), feito à mão
	u, _ := url.Parse(target)
	if u.Port() == "" {
		u.Host += ":80"
	}
	pc, err := net.Dial("tcp", httpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	fmt.Fprintf(pc, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", u.Host, u.Host)
	br := bufio.NewReader(pc)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("CONNECT falhou: %v %v", err, resp)
	}
	fmt.Fprintf(pc, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", u.Host)
	resp, err = http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "ola-vpn" {
		t.Fatalf("CONNECT: corpo inesperado %q", b)
	}
}

func checkBody(t *testing.T, c *http.Client, target string) {
	t.Helper()
	resp, err := c.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "ola-vpn" {
		t.Fatalf("corpo inesperado %q", b)
	}
}

func TestProxiesDirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ola-vpn") }))
	defer srv.Close()
	s, h := startProxies(t, directTunnel{})
	getVia(t, srv.URL, s, h)
}

func genKey(t *testing.T) (priv, pub []byte) {
	priv = make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		t.Fatal(err)
	}
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

// TestWireGuardEndToEnd sobe um servidor WireGuard em memória (com um
// servidor HTTP dentro da "rede da VPN" em 10.77.0.1) e conecta o
// vpn_specify nele usando um arquivo .conf normal.
func TestWireGuardEndToEnd(t *testing.T) {
	sPriv, sPub := genKey(t)
	cPriv, cPub := genKey(t)

	// porta UDP livre para o servidor
	uc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uc.LocalAddr().(*net.UDPAddr).Port
	uc.Close()

	stun, snet, err := netstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.77.0.1")}, nil, 1420)
	if err != nil {
		t.Fatal(err)
	}
	sdev := device.NewDevice(stun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, "[srv] "))
	defer sdev.Close()
	err = sdev.IpcSet(fmt.Sprintf("private_key=%s\nlisten_port=%d\npublic_key=%s\nallowed_ip=10.77.0.2/32\n",
		hex.EncodeToString(sPriv), port, hex.EncodeToString(cPub)))
	if err != nil {
		t.Fatal(err)
	}
	if err := sdev.Up(); err != nil {
		t.Fatal(err)
	}
	ln, err := snet.ListenTCP(&net.TCPAddr{IP: net.ParseIP("10.77.0.1"), Port: 80})
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ola-vpn") }))

	conf := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = 10.77.0.2/32
DNS = 10.77.0.1

[Peer]
PublicKey = %s
Endpoint = 127.0.0.1:%d
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`, base64.StdEncoding.EncodeToString(cPriv), base64.StdEncoding.EncodeToString(sPub), port)

	tun, err := tunnel.OpenWireGuard([]byte(conf), tunnel.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()

	echo, err := snet.ListenUDP(&net.UDPAddr{IP: net.ParseIP("10.77.0.1"), Port: 7})
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go udpEcho(echo)

	s, h := startProxies(t, tun)
	getVia(t, "http://10.77.0.1/", s, h)
	checkUDP(t, tun, s, "10.77.0.1:7")

	// 10.77.0.1 só existe dentro da VPN: direto tem que falhar.
	c := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	if _, err := c.Get("http://10.77.0.1/"); err == nil {
		t.Fatal("10.77.0.1 respondeu fora da VPN?!")
	}
}

// checkUDP testa SOCKS5 UDP e o encaminhamento -L até um servidor de eco.
func checkUDP(t *testing.T, tun tunnel.Tunnel, socksAddr, echoAddr string) {
	t.Helper()
	for i := 0; i < 3; i++ { // várias mensagens na mesma associação seriam ideais; aqui uma por associação
		msg := []byte(fmt.Sprintf("eco-%d", i))
		got, err := netcheck.SocksUDPExchange(socksAddr, echoAddr, msg, 5*time.Second)
		if err != nil || string(got) != string(msg) {
			t.Fatalf("socks udp: %q %v", got, err)
		}
	}
	var fl proxy.ForwardList
	if err := fl.Set("udp:127.0.0.1:0=" + echoAddr); err != nil {
		t.Fatal(err)
	}
	c, err := proxy.StartForward(fl[0], tun, false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	local := c.(*net.UDPConn).LocalAddr().String()
	uc, err := net.Dial("udp", local)
	if err != nil {
		t.Fatal(err)
	}
	defer uc.Close()
	uc.SetDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 100)
	for i := 0; i < 3; i++ {
		msg := fmt.Sprintf("fwd-%d", i)
		uc.Write([]byte(msg))
		n, err := uc.Read(buf)
		if err != nil || string(buf[:n]) != msg {
			t.Fatalf("forward udp: %q %v", buf[:n], err)
		}
	}
}

func udpEcho(pc net.PacketConn) {
	buf := make([]byte, 2000)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		pc.WriteTo(buf[:n], from)
	}
}

func TestUDPDirect(t *testing.T) {
	echo, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go udpEcho(echo)
	s, _ := startProxies(t, directTunnel{})
	checkUDP(t, directTunnel{}, s, echo.LocalAddr().String())
}
