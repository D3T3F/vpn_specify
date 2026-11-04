package tunnel

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

func wgKeys(t *testing.T) (private, public string) {
	t.Helper()
	var k device.NoisePrivateKey
	for i := range k {
		k[i] = byte(i*7 + 1 + len(t.Name())%3)
	}
	k[0] &= 248
	k[31] = (k[31] & 127) | 64
	return base64.StdEncoding.EncodeToString(k[:]), publicOf(t, k[:])
}

func TestPlanWireGuard(t *testing.T) {
	priv, pub := wgKeys(t)
	conf := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = 10.66.0.2/24, fd00::2/64
DNS = 10.66.0.1, corp.example
MTU = 1380

[Peer]
PublicKey = %s
Endpoint = 127.0.0.1:51820
AllowedIPs = 10.66.0.77/24, ::/0, 10.66.0.0/24
`, priv, pub)
	plan, err := PlanWireGuard([]byte(conf))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(plan.Addresses, " "); got != "10.66.0.2/32 fd00::2/128" {
		t.Errorf("addresses: %s", got)
	}
	// Android refuses a route with host bits set; duplicates are dropped
	if got := strings.Join(plan.Routes, " "); got != "10.66.0.0/24 ::/0" {
		t.Errorf("routes: %s", got)
	}
	if got := strings.Join(plan.DNS, " "); got != "10.66.0.1" || plan.MTU != 1380 {
		t.Errorf("dns %s, mtu %d", got, plan.MTU)
	}

	noDNS := strings.Replace(conf, "DNS = 10.66.0.1, corp.example\n", "", 1)
	if plan, err = PlanWireGuard([]byte(noDNS)); err != nil || plan.DNS[0] != "1.1.1.1" {
		t.Errorf("default DNS: %v %v", plan, err)
	}
	if _, err := PlanWireGuard([]byte("client\nremote vpn.example 1194\n")); err == nil {
		t.Error("an OpenVPN file must not pass as WireGuard")
	}
	if _, err := PlanWireGuard([]byte(strings.Replace(conf, "AllowedIPs = 10.66.0.77/24, ::/0, 10.66.0.0/24\n", "", 1))); err == nil {
		t.Error("no AllowedIPs: nothing would use the VPN")
	}
}

// TestWireGuardDeviceTraffic connects a device to a peer over real UDP, with
// netstack playing the part of the Android tun on both ends.
func TestWireGuardDeviceTraffic(t *testing.T) {
	serverPriv, serverPub := wgKeys(t)
	clientKey := device.NoisePrivateKey{9, 8, 7, 6, 5}
	clientKey[31] = 64
	clientPriv := base64.StdEncoding.EncodeToString(clientKey[:])

	// server
	stun, snet, err := netstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.66.0.1")}, nil, 1420)
	if err != nil {
		t.Fatal(err)
	}
	sdev := device.NewDevice(stun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, ""))
	defer sdev.Close()
	serverCfg, err := ParseWireGuardConfig([]byte(fmt.Sprintf("[Interface]\nPrivateKey=%s\nAddress=10.66.0.1/32\n[Peer]\nPublicKey=%s\nAllowedIPs=10.66.0.2/32\n", serverPriv, publicOf(t, clientKey[:]))))
	if err != nil {
		t.Fatal(err)
	}
	if err := sdev.IpcSet(serverCfg.ipc() + ""); err != nil {
		t.Fatal(err)
	}
	if err := sdev.IpcSet("listen_port=0\n"); err != nil {
		t.Fatal(err)
	}
	if err := sdev.Up(); err != nil {
		t.Fatal(err)
	}
	port := ""
	got, _ := sdev.IpcGet()
	for _, l := range strings.Split(got, "\n") {
		if v, ok := strings.CutPrefix(l, "listen_port="); ok {
			port = v
		}
	}
	ln, err := snet.ListenTCP(&net.TCPAddr{Port: 80})
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "inside") }))

	// client: the code under test
	ctun, cnet, err := netstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.66.0.2")}, nil, 1420)
	if err != nil {
		t.Fatal(err)
	}
	conf := fmt.Sprintf("[Interface]\nPrivateKey=%s\nAddress=10.66.0.2/32\n[Peer]\nPublicKey=%s\nEndpoint=127.0.0.1:%s\nAllowedIPs=10.66.0.0/24\n", clientPriv, serverPub, port)
	d, err := StartWireGuardDevice([]byte(conf), ctun)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// the handshake happens without any app sending traffic
	deadline := time.Now().Add(10 * time.Second)
	for {
		st, err := d.Stats()
		if err != nil {
			t.Fatal(err)
		}
		if st.LastHandshake > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no handshake")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := d.Rebind(); err != nil { // network change
		t.Fatal(err)
	}
	hc := http.Client{Transport: &http.Transport{DialContext: cnet.DialContext}, Timeout: 10 * time.Second}
	resp, err := hc.Get("http://10.66.0.1/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if b, _ := io.ReadAll(resp.Body); string(b) != "inside" {
		t.Fatalf("reply %q", b)
	}
	st, _ := d.Stats()
	if st.RxBytes == 0 || st.TxBytes == 0 {
		t.Errorf("counters: %+v", st)
	}
}
