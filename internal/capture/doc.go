// Package capture puts ALL the traffic of chosen processes (by executable
// name) inside the VPN, including UDP (voice), using the WinDivert driver.
// TCP goes through a transparent proxy; UDP through tunnel sockets.
//
// Windows only; requires Administrator.
package capture
