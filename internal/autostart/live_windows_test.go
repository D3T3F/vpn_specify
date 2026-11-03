//go:build windows

package autostart

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiveScheduler cria e remove a tarefa de verdade (precisa de
// Administrador). Só roda com VPN_SPECIFY_LIVE=1.
func TestLiveScheduler(t *testing.T) {
	if os.Getenv("VPN_SPECIFY_LIVE") != "1" {
		t.Skip("defina VPN_SPECIFY_LIVE=1 (e rode como Administrador)")
	}
	if Enabled() {
		t.Skip("a tarefa já existe nesta máquina; não mexo nela")
	}
	exe := `C:\nao existe\vpn_specify-teste.exe`
	if err := Enable(exe); err != nil {
		t.Fatal(err)
	}
	defer Disable()
	if !Enabled() {
		t.Fatal("tarefa não aparece")
	}
	out, _ := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "schtasks.exe"), "/Query", "/TN", TaskName, "/XML").Output()
	xml := string(out)
	for _, want := range []string{"<LogonTrigger>", "<RunLevel>HighestAvailable</RunLevel>", "<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",
		`<Command>"` + exe + `"</Command>`, "<Arguments>autostart</Arguments>", "<Delay>PT15S</Delay>"} {
		if !strings.Contains(xml, want) {
			t.Errorf("faltou %s na tarefa registrada", want)
		}
	}
	if err := Enable(exe); err != nil { // atualizar por cima
		t.Fatal(err)
	}
	if err := Disable(); err != nil || Enabled() {
		t.Fatalf("Disable: %v", err)
	}
}
