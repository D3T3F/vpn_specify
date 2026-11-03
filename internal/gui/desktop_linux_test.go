//go:build linux

package gui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"vpn_specify/internal/i18n"
)

func TestSplitJoinArgs(t *testing.T) {
	cases := map[string][]string{
		`a b  c`:                 {"a", "b", "c"},
		`--x="one two" 'thr"ee'`: {"--x=one two", `thr"ee`},
		`a\ b "c\"d" ''`:         {"a b", `c"d`, ""},
		`flatpak run com.app.Id`: {"flatpak", "run", "com.app.Id"},
	}
	for in, want := range cases {
		if got := splitArgs(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitArgs(%q) = %q; want %q", in, got, want)
		}
	}
	for _, args := range [][]string{{"a b", "it's", "$HOME", ""}, {"--proxy-server=socks5://127.0.0.1:1"}} {
		if got := splitArgs(joinArgs(args)); !reflect.DeepEqual(got, args) {
			t.Errorf("round trip of %q gave %q (via %q)", args, got, joinArgs(args))
		}
	}
}

func TestDesktopFile(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "my app")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	i18n.Set("pt-BR")
	defer i18n.Set("en")

	// what vpn_specify writes must read back the same (quotes, $ and %)
	p := filepath.Join(dir, "x.desktop")
	execLine := desktopQuote(bin) + " launch " + desktopQuote(`id $1 100%`)
	if err := writeDesktopFile(p, "My App", "test", execLine, "icon"); err != nil {
		t.Fatal(err)
	}
	e, err := parseDesktopFile(p)
	if err != nil {
		t.Fatal(err)
	}
	path, args, err := e.execCommand()
	if err != nil || path != bin || !reflect.DeepEqual(args, []string{"launch", `id $1 100%`}) {
		t.Fatalf("execCommand = %q %q %v", path, args, err)
	}

	// typical distribution .desktop: translated name (picked by the active
	// language, pt-BR here), field codes, env
	p2 := filepath.Join(dir, "y.desktop")
	os.WriteFile(p2, []byte(`[Desktop Entry]
Type=Application
Name=Firefox
Name[pt_BR]=Navegador Firefox
Exec=env MOZ_X=1 sh -c "echo hi" %u
Icon=firefox
[Desktop Action new-window]
Name=Nova janela
Exec=firefox --new-window
`), 0o644)
	e, err = parseDesktopFile(p2)
	if err != nil || e.Name != "Navegador Firefox" || e.Icon != "firefox" {
		t.Fatalf("parse = %+v, %v", e, err)
	}
	path, args, err = e.execCommand()
	if err != nil || filepath.Base(path) != "sh" || !reflect.DeepEqual(args, []string{"-c", "echo hi"}) {
		t.Fatalf("execCommand = %q %q %v", path, args, err)
	}

	os.WriteFile(p2, []byte("[Desktop Entry]\nType=Link\nURL=http://x\n"), 0o644)
	if _, err := parseDesktopFile(p2); err == nil {
		t.Error("Type=Link should be refused")
	}
}

func TestRPCCall(t *testing.T) {
	s := &uiServer{handlers: map[string]reflect.Value{}}
	for n, f := range map[string]any{
		"sum":  func(a, b int) int { return a + b },
		"fail": func(x string) (string, error) { return "", errors.New("went wrong: " + x) },
		"none": func() {},
		"obj":  func(m map[string]bool) (*struct{ N int }, error) { return &struct{ N int }{len(m)}, nil },
	} {
		s.handlers[n] = reflect.ValueOf(f)
	}
	if r, err := s.call("sum", []byte(`[2, 3]`)); err != nil || r != 5 {
		t.Errorf("sum = %v, %v", r, err)
	}
	if _, err := s.call("fail", []byte(`["x"]`)); err == nil || err.Error() != "went wrong: x" {
		t.Errorf("fail = %v", err)
	}
	if r, err := s.call("none", nil); err != nil || r != nil {
		t.Errorf("none = %v, %v", r, err)
	}
	if r, err := s.call("obj", []byte(`[{"a":true,"b":false}]`)); err != nil || r.(*struct{ N int }).N != 2 {
		t.Errorf("obj = %v, %v", r, err)
	}
	if _, err := s.call("sum", []byte(`[1]`)); err == nil {
		t.Error("a wrong number of arguments should fail")
	}
	if _, err := s.call("sum", []byte(`["a", 1]`)); err == nil {
		t.Error("a wrong type should fail")
	}
	if _, err := s.call("missing", nil); err == nil {
		t.Error("an unknown function should fail")
	}
}
