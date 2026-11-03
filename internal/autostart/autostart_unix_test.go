//go:build !windows

package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnableDisableXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	exe := "/opt/meus apps/vpn_specify"
	if Enabled() {
		t.Fatal("should not exist yet")
	}
	if err := Enable(exe); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(File())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `Exec="/opt/meus apps/vpn_specify" autostart`+"\n") || !strings.Contains(string(b), "Type=Application") {
		t.Errorf("content:\n%s", b)
	}
	if filepath.Base(filepath.Dir(File())) != "autostart" || !Enabled() {
		t.Error("wrong place or not enabled")
	}
	if err := Enable(exe); err != nil { // again: updates without error
		t.Fatal(err)
	}
	if err := Disable(); err != nil || Enabled() {
		t.Fatalf("Disable: %v", err)
	}
	if err := Disable(); err != nil { // already removed: fine
		t.Fatal(err)
	}
}
