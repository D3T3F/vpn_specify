//go:build !linux

package tunnel

import (
	"net"
	"syscall"
)

// On Windows the "strong host" model already ensures that a socket bound to
// the tunnel IP only leaves through the tunnel interface; no device binding
// is needed.

func prepareTunnelInterface(ip net.IP) (string, error) { return "", nil }

func bindControl(ifName string) func(network, address string, c syscall.RawConn) error {
	return nil
}
