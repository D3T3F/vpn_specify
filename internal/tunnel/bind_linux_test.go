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

// TestBindToTunnelInterface cria uma interface tun de teste (precisa de root)
// e confere que um socket preso a ela sai com o IP dela, mesmo havendo uma
// rota padrão normal: é isso que manda o tráfego do proxy pelo OpenVPN no Linux.
func TestBindToTunnelInterface(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("precisa de root (cria uma interface tun)")
	}
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("comando ip não encontrado")
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
		t.Fatalf("prepareTunnelInterface = %q, %v; quero %q", name, err, dev)
	}
	if b, _ := os.ReadFile("/proc/sys/net/ipv4/conf/" + dev + "/rp_filter"); strings.TrimSpace(string(b)) != "2" {
		t.Errorf("rp_filter = %q; quero 2", b)
	}

	// UDP "conectado" não manda nada, mas o kernel já escolhe rota e origem.
	d := net.Dialer{Control: bindControl(dev)}
	c, err := d.DialContext(context.Background(), "udp4", "1.1.1.1:53")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := c.LocalAddr().(*net.UDPAddr).IP.String(); got != addr {
		t.Errorf("origem = %s; quero %s (o socket não saiu pela interface do túnel)", got, addr)
	}
}
