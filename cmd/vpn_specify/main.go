// Command vpn_specify connects to a VPN (WireGuard or OpenVPN) without
// changing the system routes and puts only the chosen apps inside it.
//
// Without arguments it opens the graphical interface; "up", "run", "status"
// and "update" are the terminal commands (see usage).
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"

	"vpn_specify/internal/gui"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/setup"
	"vpn_specify/internal/tunnel"
	"vpn_specify/internal/version"
)

func usage() {
	fmt.Fprint(os.Stderr, i18n.T(`vpn_specify - VPN only for the apps you choose

Usage:
  vpn_specify
      Opens the graphical interface.

  vpn_specify up <file.conf|file.ovpn> [options] [-- program args...]
      Connects to the VPN and opens the local proxy. Runs until Ctrl+C.
      If a program is given after "--", it is opened already in the VPN.

  vpn_specify run [options] <program> [args...]
      Opens a program using the VPN (needs a running "up").

  vpn_specify status [options]
      Shows your public IP, direct and through the VPN.

  vpn_specify update [-check] [-y]
      Looks for a new version in the GitHub releases and offers to install it.

  vpn_specify version
      Shows the version.

  vpn_specify autostart
      Opens in the VPN the apps marked to start with the system (used by the
      system startup; normally not called by hand).

Examples:
  vpn_specify up work.ovpn
  vpn_specify up wg0.conf -- chrome
  vpn_specify run chrome
  vpn_specify run firefox
  vpn_specify run curl https://ifconfig.me
  vpn_specify run -profile client2 msedge

Use "vpn_specify <command> -h" to see the options of each command.
The language follows the system or the choice made in the interface.
`))
}

// fail prints the error and exits (with the program's exit code, if the
// error came from one).
func fail(err error) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	fmt.Fprintln(os.Stderr, i18n.T("error: {0}", err))
	os.Exit(1)
}

func main() {
	log.SetFlags(log.Ltime)
	i18n.Init()
	if len(os.Args) < 2 {
		if err := gui.Main(); err != nil && !isCancelled(err) { // no arguments: graphical interface
			fail(err)
		}
		return
	}
	var err error
	switch os.Args[1] {
	case "launch":
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: vpn_specify launch <app-id>")
			os.Exit(2)
		}
		if err := gui.LaunchMain(os.Args[2]); err != nil && !isCancelled(err) {
			os.Exit(1)
		}
		return
	case "autostart": // system startup entry (see internal/autostart)
		if err := gui.AutostartMain(); err != nil && !isCancelled(err) {
			fail(err)
		}
		return
	case "agent":
		if err := gui.AgentMain(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		return
	case "openvpn-helper": // Linux: started via pkexec by the GUI (see tunnel.RunOpenVPNHelper)
		if err := tunnel.RunOpenVPNHelper(os.Args[2:]); err != nil {
			fail(err)
		}
		return
	case "uninstall": // called from Settings › Installed apps
		if err := setup.Uninstall(os.Args[2:]); err != nil && !isCancelled(err) {
			os.Exit(1)
		}
		return
	case "up":
		err = cmdUp(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "status":
		err = cmdStatus(os.Args[2:])
	case "update":
		err = cmdUpdate(os.Args[2:])
	case "version", "-version", "--version":
		fmt.Println(version.Version)
		return
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintln(os.Stderr, i18n.T("unknown command: {0}", os.Args[1]))
		fmt.Fprintln(os.Stderr)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fail(err)
	}
}

// isCancelled: the user refused the UAC prompt (ERROR_CANCELLED).
func isCancelled(err error) bool {
	var en syscall.Errno
	return errors.As(err, &en) && en == 1223
}
