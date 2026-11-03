//go:build linux

package gui

import (
	"errors"
	"net/url"

	"vpn_specify/internal/control"
	"vpn_specify/internal/platform"
)

var errNoDisplay = errors.New("sem ambiente gráfico (DISPLAY/WAYLAND_DISPLAY vazios); use os comandos de terminal (vpn_specify help)")

// Main: "vpn_specify" sem argumentos.
func Main() error {
	if err := control.Call("show", nil); err == nil {
		return nil // já existe uma instância: ela mostra a janela
	}
	if !hasDisplay() {
		return errNoDisplay
	}
	return runAgent(true, "")
}

// LaunchMain: "vpn_specify launch <id>" (usado pelos atalhos .desktop).
func LaunchMain(id string) error {
	err := control.Call("launch", url.Values{"id": {id}})
	if err == nil {
		return nil
	}
	if !errors.Is(err, control.ErrNoInstance) {
		platform.MessageBox("vpn_specify", err.Error(), 0x10)
		return err
	}
	return runAgent(false, id)
}

// AgentMain: "vpn_specify agent [-launch <id>]" (no Linux é igual a Main).
func AgentMain(args []string) error {
	id := ""
	if len(args) == 2 && args[0] == "-launch" {
		id = args[1]
	}
	return runAgent(id == "", id)
}
