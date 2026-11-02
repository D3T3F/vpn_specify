//go:build !windows

package capture

import (
	"errors"

	"vpn_specify/internal/tunnel"
)

// Start só existe no Windows (usa o driver WinDivert).
func Start(tun tunnel.Tunnel, exeNames []string, verbose bool) (Controller, error) {
	return nil, errors.New("-capture só está disponível no Windows")
}
