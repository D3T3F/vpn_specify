//go:build linux

package update

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func writeTarGz(t *testing.T, path, script string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "./vpn_specify-x/vpn_specify", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(script))})
	tw.Write([]byte(script))
	tw.Close()
	gz.Close()
	f.Close()
}

func TestPrepareLinux(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "bin", "vpn_specify")
	os.MkdirAll(filepath.Dir(target), 0o755)
	os.WriteFile(target, []byte("#!/bin/sh\necho 1.0.0\n"), 0o755)

	// the new executable runs and reports the right version: replace
	pkg := filepath.Join(dir, "ok.tar.gz")
	writeTarGz(t, pkg, "#!/bin/sh\necho 1.2.0\n")
	plan, err := prepareAt(pkg, "1.2.0", false, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Run(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "#!/bin/sh\necho 1.2.0\n" {
		t.Errorf("not replaced: %q", b)
	}
	if fi, _ := os.Stat(target); fi.Mode()&0o111 == 0 {
		t.Error("lost the execute permission")
	}

	// a version other than announced (or a binary that does not run): no replace
	bad := filepath.Join(dir, "bad", "x.tar.gz")
	os.MkdirAll(filepath.Dir(bad), 0o755)
	writeTarGz(t, bad, "#!/bin/sh\necho 9.9.9\n")
	if _, err := prepareAt(bad, "1.3.0", false, target); err == nil {
		t.Error("a wrong version should fail")
	}
	if b, _ := os.ReadFile(target); string(b) != "#!/bin/sh\necho 1.2.0\n" {
		t.Errorf("replaced despite the error: %q", b)
	}
}
