package paths

import (
	"path/filepath"
	"testing"
)

func TestMacOSConfigBase(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	if got, err := macOSConfigBase(); err != nil || got != "/custom/config" {
		t.Errorf("with XDG_CONFIG_HOME: %q, %v", got, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if got, err := macOSConfigBase(); err != nil || got != filepath.Join(home, ".config") {
		t.Errorf("default: %q, %v", got, err)
	}
}
