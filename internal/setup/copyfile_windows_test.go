//go:build windows

package setup

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestCopyFileSkipsIdenticalFile(t *testing.T) {
	src := fstest.MapFS{"lib/tool.exe": {Data: []byte("same bytes")}}
	dst := filepath.Join(t.TempDir(), "lib", "tool.exe")
	writeFile(t, dst, "same bytes")
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(dst, old, old); err != nil {
		t.Fatal(err)
	}

	n, restart, err := copyFile(src, "lib/tool.exe", dst)
	if err != nil || n != int64(len("same bytes")) || restart {
		t.Fatalf("copyFile = %d, %v, %v; want %d, false, nil", n, restart, err, len("same bytes"))
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Errorf("identical file was rewritten (mtime %v)", info.ModTime())
	}
	assertNoFile(t, dst+".new")
	assertNoFile(t, dst+".old")
}

func TestCopyFileReplacesDifferentFile(t *testing.T) {
	src := fstest.MapFS{"tool.exe": {Data: []byte("new version")}}
	dst := filepath.Join(t.TempDir(), "tool.exe")
	writeFile(t, dst, "old version")

	n, restart, err := copyFile(src, "tool.exe", dst)
	if err != nil || n != int64(len("new version")) || restart {
		t.Fatalf("copyFile = %d, %v, %v; want %d, false, nil", n, restart, err, len("new version"))
	}
	assertFile(t, dst, "new version")
	assertNoFile(t, dst+".new")
	assertNoFile(t, dst+".old")
}

func TestCopyFileCreatesMissingFile(t *testing.T) {
	src := fstest.MapFS{"sub/new.dll": {Data: []byte("fresh")}}
	dst := filepath.Join(t.TempDir(), "sub", "new.dll")

	if _, _, err := copyFile(src, "sub/new.dll", dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	assertFile(t, dst, "fresh")
}
