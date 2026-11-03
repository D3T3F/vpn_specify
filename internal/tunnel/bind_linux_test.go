//go:build linux

package tunnel

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestBindToTunnelInterface creates a test tun interface (needs root) and
// checks that a socket bound to it leaves with its IP, even with a normal
// default route: that is what sends the proxy traffic through OpenVPN on Linux.
func TestBindToTunnelInterface(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root (creates a tun interface)")
	}
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("the ip command was not found")
	}
	const dev, addr = "vstest0", "10.199.77.2"
	ipCmd := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("ip", args...).CombinedOutput(); err != nil {
			t.Fatalf("ip %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	_ = exec.Command("ip", "link", "del", dev).Run()
	ipCmd("tuntap", "add", "dev", dev, "mode", "tun")
	t.Cleanup(func() { _ = exec.Command("ip", "link", "del", dev).Run() })
	ipCmd("addr", "add", addr+"/24", "dev", dev)
	ipCmd("link", "set", dev, "up")

	name, err := prepareTunnelInterface(net.ParseIP(addr))
	if err != nil || name != dev {
		t.Fatalf("prepareTunnelInterface = %q, %v; want %q", name, err, dev)
	}
	if b, _ := os.ReadFile("/proc/sys/net/ipv4/conf/" + dev + "/rp_filter"); strings.TrimSpace(string(b)) != "2" {
		t.Errorf("rp_filter = %q; want 2", b)
	}

	// A "connected" UDP socket sends nothing, but the kernel already picks route and source.
	d := net.Dialer{Control: bindControl(dev)}
	c, err := d.DialContext(context.Background(), "udp4", "1.1.1.1:53")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := c.LocalAddr().(*net.UDPAddr).IP.String(); got != addr {
		t.Errorf("source = %s; want %s (the socket did not leave through the tunnel interface)", got, addr)
	}
}
