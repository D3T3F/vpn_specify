//go:build darwin

package ovpninstall

import (
	"context"

	"vpn_specify/internal/i18n"
)

func source() string { return "Homebrew" }

// Install does not install anything on macOS: OpenVPN comes from Homebrew, and
// the user runs its command in a terminal (no automatic install, no root).
func Install(ctx context.Context, progress func(string)) error {
	return i18n.Error("OpenVPN is not installed; install it with Homebrew: brew install openvpn")
}
