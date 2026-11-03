package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutostartSameVPN(t *testing.T) {
	st, err := OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	for _, n := range []string{"home", "work"} {
		p := filepath.Join(src, n+".conf")
		os.WriteFile(p, []byte("[Interface]\nPrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=\nAllowedIPs = 0.0.0.0/0\nEndpoint = 127.0.0.1:51820\n"), 0o600)
		if _, err := st.ImportConfig(p); err != nil {
			t.Fatal(err)
		}
	}
	a, err := st.SaveApp(AppEntry{Name: "A", Path: "/bin/a", Config: "home", Mode: "proxy", Autostart: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveApp(AppEntry{Name: "B", Path: "/bin/b", Config: "home", Mode: "proxy", Autostart: true}); err != nil {
		t.Fatalf("the same VPN should be accepted: %v", err)
	}
	c, err := st.SaveApp(AppEntry{Name: "C", Path: "/bin/c", Config: "work", Mode: "proxy", Autostart: true})
	if err == nil || !strings.Contains(err.Error(), "same VPN") {
		t.Fatalf("a different VPN should fail: %v", err)
	}
	// without the option, another VPN is fine
	c.Autostart = false
	if _, err := st.SaveApp(c); err != nil {
		t.Fatal(err)
	}
	if n := len(st.AutostartApps()); n != 2 {
		t.Errorf("AutostartApps = %d; want 2", n)
	}
	// unmarking A removes it from the list
	a.Autostart = false
	if _, err := st.SaveApp(a); err != nil {
		t.Fatal(err)
	}
	if n := len(st.AutostartApps()); n != 1 {
		t.Errorf("AutostartApps = %d; want 1", n)
	}
}
