package mobile

import (
	"golang.org/x/sys/unix"
	"golang.zx2c4.com/wireguard/tun"
)

// openTun wraps the descriptor received from VpnService. "Unmonitored":
// Android owns the interface, so there is no need to watch it through netlink,
// which apps are not allowed to do anyway.
func openTun(fd int) (tun.Device, error) {
	d, _, err := tun.CreateUnmonitoredTUNFromFD(fd)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	return d, nil
}
