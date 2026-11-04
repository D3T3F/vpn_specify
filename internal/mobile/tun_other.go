//go:build !linux

package mobile

import (
	"errors"

	"golang.zx2c4.com/wireguard/tun"
)

func openTun(fd int) (tun.Device, error) {
	return nil, errors.New("tun descriptors only exist on Android")
}
