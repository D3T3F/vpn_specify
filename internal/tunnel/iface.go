//go:build linux || darwin

package tunnel

import (
	"net"

	"vpn_specify/internal/i18n"
)

// interfaceWithIP returns the name of the interface holding this IP (e.g. tun0, utun3).
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
