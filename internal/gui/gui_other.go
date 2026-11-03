//go:build !windows && !linux

package gui

import "vpn_specify/internal/i18n"

func errNoGUI() error {
	return i18n.Error("the graphical interface is only available on Windows and Linux; use the terminal commands (vpn_specify help)")
}

func Main() error                   { return errNoGUI() }
func LaunchMain(id string) error    { return errNoGUI() }
func AgentMain(args []string) error { return errNoGUI() }
func AutostartMain() error          { return errNoGUI() }
