//go:build linux

package gui

import (
	"errors"
	"net/url"

	"vpn_specify/internal/control"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/platform"
)

var errNoDisplay = i18n.Sentinel("no graphical session (DISPLAY/WAYLAND_DISPLAY are empty); use the terminal commands (vpn_specify help)")

// Main: "vpn_specify" without arguments.
func Main() error {
	if err := control.Call("show", nil); err == nil {
		return nil // an instance already exists: it shows the window
	}
	if !hasDisplay() {
		return errNoDisplay
	}
	return runAgent(true, "", false)
}

// LaunchMain: "vpn_specify launch <id>" (used by the .desktop shortcuts).
func LaunchMain(id string) error {
	err := control.Call("launch", url.Values{"id": {id}})
	if err == nil {
		return nil
	}
	if !errors.Is(err, control.ErrNoInstance) {
		platform.MessageBox("vpn_specify", err.Error(), 0x10)
		return err
	}
	return runAgent(false, id, false)
}

// AutostartMain: "vpn_specify autostart", run when the session starts.
func AutostartMain() error {
	if err := control.Call("autostart", nil); err == nil {
		return nil
	}
	return runAgent(false, "", true)
}

// AgentMain: "vpn_specify agent [-launch <id>]" (on Linux the same as Main).
func AgentMain(args []string) error {
	id := ""
	if len(args) == 2 && args[0] == "-launch" {
		id = args[1]
	}
	if len(args) == 1 && args[0] == "-autostart" {
		return runAgent(false, "", true)
	}
	return runAgent(id == "", id, false)
}
