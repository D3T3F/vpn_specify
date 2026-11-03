//go:build !linux

package tunnel

import (
	"net"
	"syscall"
)

// No Windows o modelo "strong host" já garante que um socket ligado ao IP do
// túnel só sai pela interface do túnel; não é preciso prender à interface.

func prepareTunnelInterface(ip net.IP) (string, error) { return "", nil }

func bindControl(ifName string) func(network, address string, c syscall.RawConn) error {
	return nil
}
