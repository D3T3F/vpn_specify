//go:build darwin

package tunnel

import (
	"fmt"
	"net"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// macOS sends with the "weak host" model too: binding the socket to the
// tunnel IP is not enough, the kernel would use the normal default route. So,
// as on Linux, the proxy sockets are bound to the tunnel interface, with
// IP_BOUND_IF (IPV6_BOUND_IF for IPv6). That needs no root.

func prepareTunnelInterface(ip net.IP) (string, error) {
	return interfaceWithIP(ip)
}

// bindControl binds the socket to the ifName interface.
func bindControl(ifName string) func(network, address string, c syscall.RawConn) error {
	if ifName == "" {
		return nil
	}
	return func(network, address string, c syscall.RawConn) error {
		ifc, err := net.InterfaceByName(ifName)
		if err != nil {
			return err
		}
		level, opt := unix.IPPROTO_IP, unix.IP_BOUND_IF
		if strings.HasSuffix(network, "6") {
			level, opt = unix.IPPROTO_IPV6, unix.IPV6_BOUND_IF
		}
		var serr error
		if err := c.Control(func(fd uintptr) {
			serr = unix.SetsockoptInt(int(fd), level, opt, ifc.Index)
		}); err != nil {
			return err
		}
		if serr != nil {
			return fmt.Errorf("IP_BOUND_IF %s: %w", ifName, serr)
		}
		return nil
	}
}
