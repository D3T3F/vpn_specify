//go:build windows

package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vpn_specify/internal/platform"
)

func TestStoreAndShortcut(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "minha vpn.ovpn")
	os.WriteFile(src, []byte("client\nremote x.example 1194\nauth-user-pass\n"), 0o600)
	c, err := st.ImportConfig(src)
	if err != nil || c.Kind != "openvpn" || !c.NeedAuth {
		t.Fatalf("import: %+v %v", c, err)
	}
	if c2, _ := st.ImportConfig(src); c2.Name != "minha vpn (2)" {
		t.Fatalf("duplicate name: %q", c2.Name)
	}
	if err := st.SetCredentials(c.Name, "joao", "s3nh@"); err != nil {
		t.Fatal(err)
	}
	_, o, err := st.ConfigFor(c.Name)
	if err != nil || o.AuthUser != "joao" || o.AuthPass != "s3nh@" {
		t.Fatalf("credentials: %+v %v", o, err)
	}
	if snap := st.Snapshot(); snap.Configs[0].PassEnc != "" {
		t.Fatal("the password leaked to the interface")
	}
	a, err := st.SaveApp(AppEntry{Name: "Discord", Path: `C:\x\Update.exe`, Config: c.Name, Capture: []string{"Discord"}})
	if err != nil || a.ID == "" || a.Capture[0] != "discord.exe" || a.Mode != "capture" {
		t.Fatalf("app: %+v %v", a, err)
	}
	if err := st.RemoveConfig(c.Name); err == nil {
		t.Fatal("should refuse to remove a VPN in use")
	}
	// reload from disk
	st2, _ := OpenAt(dir)
	if len(st2.Snapshot().Apps) != 1 {
		t.Fatal("not persisted")
	}

	lnk := filepath.Join(dir, "TesteVPN.lnk")
	exe, _ := os.Executable()
	if err := platform.CreateShortcut(lnk, exe, "launch "+a.ID, dir, exe+",0", "test"); err != nil {
		t.Fatal(err)
	}
	si, err := platform.ResolveShortcut(lnk)
	if err != nil || !filepath.IsAbs(si.Target) || si.Args != "launch "+a.ID {
		t.Fatalf("shortcut: %+v %v", si, err)
	}
}

func TestUpdateConfig(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "work.ovpn")
	os.WriteFile(src, []byte("client\r\nremote a.example 1194\r\n"), 0o600)
	c, err := st.ImportConfig(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetCredentials(c.Name, "ana", "x"); err != nil {
		t.Fatal(err)
	}
	a, _ := st.SaveApp(AppEntry{Name: "App", Path: `C:\a.exe`, Config: c.Name, Capture: []string{"a.exe"}})
	other, _ := st.ImportConfig(src) // "work (2)"

	txt, err := st.ConfigText(c.Name)
	if err != nil || strings.Contains(txt, "\r") || !strings.Contains(txt, "remote a.example") {
		t.Fatalf("ConfigText: %q %v", txt, err)
	}
	if _, err := st.UpdateConfig(c.Name, other.Name, txt); err == nil {
		t.Fatal("should refuse a repeated name")
	}
	if _, err := st.UpdateConfig(c.Name, "Home", "this is not a vpn"); err == nil {
		t.Fatal("should refuse invalid content")
	}
	u, err := st.UpdateConfig(c.Name, "Casa", txt+"auth-user-pass\n")
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "Casa" || u.File != "Casa.ovpn" || !u.NeedAuth || u.User != "ana" || u.PassEnc != "" {
		t.Fatalf("result: %+v", u)
	}
	if _, err := os.Stat(filepath.Join(dir, "configs", "work.ovpn")); !os.IsNotExist(err) {
		t.Fatal("the old file was left behind")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "configs", "Casa.ovpn"))
	if !strings.Contains(string(b), "auth-user-pass\r\n") {
		t.Fatalf("should keep CRLF: %q", b)
	}
	if got, _ := st.App(a.ID); got.Config != "Casa" {
		t.Fatalf("the app did not follow the rename: %q", got.Config)
	}
	if _, o, err := st.ConfigFor("Casa"); err != nil || o.AuthPass != "x" {
		t.Fatalf("credentials lost: %v", err)
	}
}
