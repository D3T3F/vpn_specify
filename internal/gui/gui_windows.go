//go:build windows

package gui

import (
	"errors"
	"net/url"

	"vpn_specify/internal/control"
	"vpn_specify/internal/platform"
)

// Main: "vpn_specify.exe" without arguments.
func Main() error {
	platform.AllowForeground()
	if err := control.Call("show", nil); err == nil {
		return nil // an instance already exists: it shows the window
	}
	if !platform.IsElevated() {
		return platform.ElevateSelf([]string{"agent"})
	}
	return runAgent(true, "", false)
}

// LaunchMain: "vpn_specify.exe launch <id>" (used by the shortcuts).
func LaunchMain(id string) error {
	platform.AllowForeground()
	err := control.Call("launch", url.Values{"id": {id}})
	if err == nil {
		return nil
	}
	if !errors.Is(err, control.ErrNoInstance) {
		platform.MessageBox("vpn_specify", err.Error(), 0x10)
		return err
	}
	if !platform.IsElevated() {
		return platform.ElevateSelf([]string{"agent", "-launch", id})
	}
	return runAgent(false, id, false)
}

// AutostartMain: "vpn_specify.exe autostart", run by the scheduled task at
// logon (already with highest privileges, no UAC).
func AutostartMain() error {
	if err := control.Call("autostart", nil); err == nil {
		return nil
	}
	if !platform.IsElevated() {
		return platform.ElevateSelf([]string{"agent", "-autostart"})
	}
	return runAgent(false, "", true)
}

// AgentMain: elevated instance (opened through UAC).
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
