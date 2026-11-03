//go:build linux

package gui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSplitJoinArgs(t *testing.T) {
	cases := map[string][]string{
		`a b  c`:                 {"a", "b", "c"},
		`--x="um dois" 'tr"es'`:  {"--x=um dois", `tr"es`},
		`a\ b "c\"d" ''`:         {"a b", `c"d`, ""},
		`flatpak run com.app.Id`: {"flatpak", "run", "com.app.Id"},
	}
	for in, want := range cases {
		if got := splitArgs(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitArgs(%q) = %q; quero %q", in, got, want)
		}
	}
	for _, args := range [][]string{{"a b", "it's", "$HOME", ""}, {"--proxy-server=socks5://127.0.0.1:1"}} {
		if got := splitArgs(joinArgs(args)); !reflect.DeepEqual(got, args) {
			t.Errorf("ida e volta de %q deu %q (via %q)", args, got, joinArgs(args))
		}
	}
}

func TestDesktopFile(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "meu app")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LANG", "pt_BR.UTF-8")

	// o que o vpn_specify grava tem que voltar igual (aspas, $ e %)
	p := filepath.Join(dir, "x.desktop")
	execLine := desktopQuote(bin) + " launch " + desktopQuote(`id $1 100%`)
	if err := writeDesktopFile(p, "Meu App", "teste", execLine, "ícone"); err != nil {
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

	// .desktop típico de distribuição: nome traduzido, códigos de campo, env
	p2 := filepath.Join(dir, "y.desktop")
	os.WriteFile(p2, []byte(`[Desktop Entry]
Type=Application
Name=Firefox
Name[pt_BR]=Navegador Firefox
Exec=env MOZ_X=1 sh -c "echo oi" %u
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
	if err != nil || filepath.Base(path) != "sh" || !reflect.DeepEqual(args, []string{"-c", "echo oi"}) {
		t.Fatalf("execCommand = %q %q %v", path, args, err)
	}

	os.WriteFile(p2, []byte("[Desktop Entry]\nType=Link\nURL=http://x\n"), 0o644)
	if _, err := parseDesktopFile(p2); err == nil {
		t.Error("Type=Link deveria ser recusado")
	}
}

func TestRPCCall(t *testing.T) {
	s := &uiServer{handlers: map[string]reflect.Value{}}
	for n, f := range map[string]any{
		"soma":  func(a, b int) int { return a + b },
		"falha": func(x string) (string, error) { return "", errors.New("deu ruim: " + x) },
		"nada":  func() {},
		"obj":   func(m map[string]bool) (*struct{ N int }, error) { return &struct{ N int }{len(m)}, nil },
	} {
		s.handlers[n] = reflect.ValueOf(f)
	}
	if r, err := s.call("soma", []byte(`[2, 3]`)); err != nil || r != 5 {
		t.Errorf("soma = %v, %v", r, err)
	}
	if _, err := s.call("falha", []byte(`["x"]`)); err == nil || err.Error() != "deu ruim: x" {
		t.Errorf("falha = %v", err)
	}
	if r, err := s.call("nada", nil); err != nil || r != nil {
		t.Errorf("nada = %v, %v", r, err)
	}
	if r, err := s.call("obj", []byte(`[{"a":true,"b":false}]`)); err != nil || r.(*struct{ N int }).N != 2 {
		t.Errorf("obj = %v, %v", r, err)
	}
	if _, err := s.call("soma", []byte(`[1]`)); err == nil {
		t.Error("número errado de argumentos deveria falhar")
	}
	if _, err := s.call("soma", []byte(`["a", 1]`)); err == nil {
		t.Error("tipo errado deveria falhar")
	}
	if _, err := s.call("naoexiste", nil); err == nil {
		t.Error("função desconhecida deveria falhar")
	}
}
