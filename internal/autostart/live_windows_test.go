//go:build windows

package autostart

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiveScheduler really creates and removes the task (needs
// Administrator). It only runs with VPN_SPECIFY_LIVE=1.
func TestLiveScheduler(t *testing.T) {
	if os.Getenv("VPN_SPECIFY_LIVE") != "1" {
		t.Skip("set VPN_SPECIFY_LIVE=1 (and run as Administrator)")
	}
	if Enabled() {
		t.Skip("the task already exists on this machine; leaving it alone")
	}
	exe := `C:\nao existe\vpn_specify-teste.exe`
	if err := Enable(exe); err != nil {
		t.Fatal(err)
	}
	defer Disable()
	if !Enabled() {
		t.Fatal("the task does not show up")
	}
	out, _ := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "schtasks.exe"), "/Query", "/TN", TaskName, "/XML").Output()
	xml := string(out)
	for _, want := range []string{"<LogonTrigger>", "<RunLevel>HighestAvailable</RunLevel>", "<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",
		`<Command>"` + exe + `"</Command>`, "<Arguments>autostart</Arguments>", "<Delay>PT15S</Delay>"} {
		if !strings.Contains(xml, want) {
			t.Errorf("%s is missing from the registered task", want)
		}
	}
	if err := Enable(exe); err != nil { // update in place
		t.Fatal(err)
	}
	if err := Disable(); err != nil || Enabled() {
		t.Fatalf("Disable: %v", err)
	}
}
