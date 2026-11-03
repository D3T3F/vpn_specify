//go:build windows

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Prepares a manual end-to-end test in the real data folder.
// VPN_SPECIFY_E2E_OVPN=<file> VPN_SPECIFY_E2E_IPCHECK=<ipcheck.exe> go test -run E2ESetup ./internal/store
func TestE2ESetup(t *testing.T) {
	ovpn, ipcheck := os.Getenv("VPN_SPECIFY_E2E_OVPN"), os.Getenv("VPN_SPECIFY_E2E_IPCHECK")
	if ovpn == "" || ipcheck == "" {
		t.Skip("manual test")
	}
	wd, _ := os.Getwd()
	st, err := OpenAt(filepath.Join(wd, "..", "..", "data")) // the project root's data folder
	if err != nil {
		t.Fatal(err)
	}
	c, err := st.ImportConfig(ovpn)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(filepath.Dir(ipcheck), "out.txt")
	a, err := st.SaveApp(AppEntry{
		Name: "ipcheck (test)", Path: filepath.Join(os.Getenv("WINDIR"), `System32\cmd.exe`),
		Args: fmt.Sprintf(`/c ""%s" > "%s""`, ipcheck, out), Config: c.Name, Capture: []string{"ipcheck.exe"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("CONFIG=%s\nAPP=%s\nOUT=%s\n", c.Name, a.ID, out)
}
