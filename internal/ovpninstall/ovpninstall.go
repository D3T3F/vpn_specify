// Package ovpninstall installs OpenVPN when a VPN needs it and it is missing.
//
// vpn_specify drives the openvpn program (it cannot be linked in), so .ovpn
// configurations depend on it. Install is only called after the user agreed:
//   - Windows: downloads the official MSI, at a pinned version checked against
//     a pinned SHA-256, and installs it silently, without the OpenVPN GUI;
//   - Linux: installs the distribution's openvpn package with its package
//     manager, through pkexec (or directly when already root).
package ovpninstall

import "vpn_specify/internal/i18n"

// Question is what the user is asked before installing.
func Question() string {
	return i18n.T("This VPN uses OpenVPN, which is not installed.\n\nDownload and install OpenVPN now? It comes from {0}.", source())
}
