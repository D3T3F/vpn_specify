// Package autostart registers vpn_specify to run when the user signs in,
// with "vpn_specify autostart", which opens in the VPN the apps marked to
// start with the system. There is a single entry, created when some app is
// marked and removed when none is:
//   - Windows: scheduled task "vpn_specify\Start apps in VPN" at the user's
//     logon, with highest privileges (vpn_specify needs Administrator; the
//     task avoids a UAC prompt at every logon);
//   - Linux: ~/.config/autostart/vpn_specify.desktop (XDG standard, followed
//     by GNOME, KDE, XFCE, Cinnamon, MATE...).
package autostart

// Arg is the argument the entry passes to vpn_specify.
const Arg = "autostart"
