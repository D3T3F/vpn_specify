//go:build windows

package gui

import (
	"errors"
	"net/url"

	"vpn_specify/internal/control"
	"vpn_specify/internal/platform"
)

// Main: "vpn_specify.exe" sem argumentos.
func Main() error {
	platform.AllowForeground()
	if err := control.Call("show", nil); err == nil {
		return nil // já existe uma instância: ela mostra a janela
	}
	if !platform.IsElevated() {
		return platform.ElevateSelf([]string{"agent"})
	}
	return runAgent(true, "", false)
}

// LaunchMain: "vpn_specify.exe launch <id>" (usado pelos atalhos).
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

// AutostartMain: "vpn_specify.exe autostart", rodado pela tarefa agendada no
// logon (já com privilégio máximo, sem UAC).
func AutostartMain() error {
	if err := control.Call("autostart", nil); err == nil {
		return nil
	}
	if !platform.IsElevated() {
		return platform.ElevateSelf([]string{"agent", "-autostart"})
	}
	return runAgent(false, "", true)
}

// AgentMain: instância elevada (aberta via UAC).
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
