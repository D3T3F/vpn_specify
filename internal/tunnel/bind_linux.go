//go:build linux

package tunnel

import (
	"fmt"
	"log"
	"net"
	"os"
	"syscall"

	"vpn_specify/internal/i18n"

	"golang.org/x/sys/unix"
)

// Linux sends with the "weak host" model: binding the socket to the tunnel IP
// is not enough; the kernel would pick the normal default route (lower
// metric) and the packet would leave through the regular network with the
// tunnel IP as source. So the proxy sockets are bound to the tunnel
// interface with SO_BINDTODEVICE (allowed for root, and for normal users
// since kernel 5.7).

// interfaceWithIP returns the name of the interface holding this IP (e.g. tun0).
func interfaceWithIP(ip net.IP) (string, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, ifc := range ifs {
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
				return ifc.Name, nil
			}
		}
	}
	return "", i18n.Error("no interface has the IP {0}", ip)
}

// prepareTunnelInterface finds the tunnel interface and sets its rp_filter
// to "loose". On distributions with strict rp_filter (Fedora, RHEL) replies
// arriving through the tunnel would be dropped, because the best route back
// to the internet is the normal network's, not the tunnel's.
func prepareTunnelInterface(ip net.IP) (string, error) {
	name, err := interfaceWithIP(ip)
	if err != nil {
		return "", err
	}
	if os.Geteuid() == 0 { // without root, the openvpn-helper does it
		setLooseRPFilter(name)
	}
	return name, nil
}

func setLooseRPFilter(name string) {
	if err := os.WriteFile("/proc/sys/net/ipv4/conf/"+name+"/rp_filter", []byte("2"), 0o644); err != nil {
		log.Printf("warning: could not set rp_filter of %s (%v); if replies do not arrive, run: sysctl -w net.ipv4.conf.%s.rp_filter=2", name, err, name)
	}
}

// bindControl binds the socket to the ifName interface.
func bindControl(ifName string) func(network, address string, c syscall.RawConn) error {
	if ifName == "" {
		return nil
	}
	return func(network, address string, c syscall.RawConn) error {
		var serr error
		if err := c.Control(func(fd uintptr) {
			serr = unix.BindToDevice(int(fd), ifName)
		}); err != nil {
			return err
		}
		if serr == unix.EPERM {
			return i18n.Error("SO_BINDTODEVICE {0}: permission denied (without root, Linux only allows it since kernel 5.7)", ifName)
		}
		if serr != nil {
			return fmt.Errorf("SO_BINDTODEVICE %s: %w", ifName, serr)
		}
		return nil
	}
}
