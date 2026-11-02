//go:build windows

package gui

import (
	_ "embed"
	"errors"
	"net/url"

	"vpn_specify/internal/control"
	"vpn_specify/internal/platform"
)

//go:embed web/index.html
var uiHTML string

// Main: "vpn_specify.exe" sem argumentos.
func Main() error {
	platform.AllowForeground()
	if err := control.Call("show", nil); err == nil {
		return nil // já existe uma instância: ela mostra a janela
	}
	if !platform.IsElevated() {
		return platform.ElevateSelf([]string{"agent"})
	}
	return runAgent(true, "")
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
	return runAgent(false, id)
}

// AgentMain: instância elevada (aberta via UAC).
func AgentMain(args []string) error {
	id := ""
	if len(args) == 2 && args[0] == "-launch" {
		id = args[1]
	}
	return runAgent(id == "", id)
}
