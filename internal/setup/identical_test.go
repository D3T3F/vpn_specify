package setup

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestSameContent(t *testing.T) {
	src := fstest.MapFS{
		"a.bin": {Data: []byte("payload")},
		"b.bin": {Data: []byte("payload2")},
		"e.bin": {Data: []byte{}},
	}
	dir := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	same := write("same.bin", "payload")
	if ok, err := sameContent(src, "a.bin", same); err != nil || !ok {
		t.Errorf("identical file: got %v, %v; want true, nil", ok, err)
	}

	differs := write("differs.bin", "PAYLOAD")
	if ok, err := sameContent(src, "a.bin", differs); err != nil || ok {
		t.Errorf("same size, different bytes: got %v, %v; want false, nil", ok, err)
	}

	short := write("short.bin", "pay")
	if ok, err := sameContent(src, "a.bin", short); err != nil || ok {
		t.Errorf("size mismatch: got %v, %v; want false, nil", ok, err)
	}

	if ok, err := sameContent(src, "a.bin", filepath.Join(dir, "missing.bin")); err == nil || ok {
		t.Errorf("missing dst: got %v, %v; want false with an error", ok, err)
	}

	if ok, err := sameContent(src, "missing.bin", same); err == nil || ok {
		t.Errorf("missing src: got %v, %v; want false with an error", ok, err)
	}

	empty := write("empty.bin", "")
	if ok, err := sameContent(src, "e.bin", empty); err != nil || !ok {
		t.Errorf("both empty: got %v, %v; want true, nil", ok, err)
	}

	// A directory has no readable content, so the comparison must fail
	// (the caller then falls back to replacing it).
	emptySrc := fstest.MapFS{"e.bin": {Data: []byte{}}}
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if ok, err := sameContent(emptySrc, "e.bin", subdir); err == nil || ok {
		t.Errorf("unreadable dst: got %v, %v; want false with an error", ok, err)
	}
}
