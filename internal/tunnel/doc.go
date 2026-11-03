// Package tunnel opens the VPN connection inside the process, without
// changing the system routes or DNS.
//
//   - WireGuard: wireguard-go with a user-space TCP/IP stack (gVisor); no
//     network adapter and no Administrator/root needed.
//   - OpenVPN: runs the installed openvpn, filtering everything that would
//     change the routing; traffic leaves through the VPN because the sockets
//     are bound to the tunnel IP (Windows) or the tun interface (Linux).
//
// Both expose the same [Tunnel] interface (TCP, UDP and DNS inside the VPN).
package tunnel
