//go:build !windows

package capture

import (
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/tunnel"
)

// Start only exists on Windows (it uses the WinDivert driver).
func Start(tun tunnel.Tunnel, exeNames []string, verbose bool) (Controller, error) {
	return nil, i18n.Error("-capture is only available on Windows; here use \"vpn_specify run <program>\" (proxy) or -L for fixed addresses")
}
