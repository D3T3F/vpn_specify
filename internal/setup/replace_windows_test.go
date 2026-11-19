//go:build windows

package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/sys/windows"
)

// stubRename replaces renameFile for the test. fail gets the number of the
// rename call (1 is the first one) and returns the error to report for it, or
// nil to let the real rename run.
func stubRename(t *testing.T, fail func(call int) error) {
	t.Helper()
	orig := renameFile
	calls := 0
	renameFile = func(from, to string) error {
		calls++
		if err := fail(calls); err != nil {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
		}
		return orig(from, to)
	}
	t.Cleanup(func() { renameFile = orig })
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Errorf("%s = %q, %v; want %q", path, got, err, want)
	}
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s should not exist (stat error: %v)", path, err)
	}
}

// The first rename fails as if the old driver image were in use. replaceAside
// moves the old file aside and puts the new one in place, and the leftover
// .old from an earlier update does not get in the way.
func TestCopyFileFallbackWhenRenameIsLocked(t *testing.T) {
	src := fstest.MapFS{"tool.exe": {Data: []byte("new version")}}
	dst := filepath.Join(t.TempDir(), "tool.exe")
	writeFile(t, dst, "old version")
	writeFile(t, dst+".old", "leftover")
	stubRename(t, func(call int) error {
		if call == 1 {
			return windows.ERROR_ACCESS_DENIED
		}
		return nil
	})

	n, restart, err := copyFile(src, "tool.exe", dst)
	if err != nil || n != int64(len("new version")) || restart {
		t.Fatalf("copyFile = %d, %v, %v; want %d, false, nil", n, restart, err, len("new version"))
	}
	assertFile(t, dst, "new version")
	assertNoFile(t, dst+".new")
	assertNoFile(t, dst+".old")
}

// A real file held open without FILE_SHARE_DELETE cannot be renamed or
// replaced, so the update fails and the old file is left as it was.
func TestCopyFileLockedDestinationKeepsOldFile(t *testing.T) {
	src := fstest.MapFS{"tool.exe": {Data: []byte("new version")}}
	dst := filepath.Join(t.TempDir(), "tool.exe")
	writeFile(t, dst, "old version")
	held, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	if _, _, err := copyFile(src, "tool.exe", dst); err == nil {
		t.Fatal("copyFile over a locked file should fail")
	}
	assertFile(t, dst, "old version")
	assertNoFile(t, dst+".new")
	assertNoFile(t, dst+".old")
}

// The new file cannot be put in place and the old one cannot be moved back:
// the error names the .old file and reports both failures.
func TestReplaceAsideReportsBothErrorsWhenRestoreFails(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "tool.exe")
	tmp := dst + ".new"
	writeFile(t, dst, "old version")
	writeFile(t, tmp, "new version")
	stubRename(t, func(call int) error {
		switch call {
		case 2: // tmp -> dst
			return windows.ERROR_ACCESS_DENIED
		case 3: // .old -> dst (restore)
			return windows.ERROR_SHARING_VIOLATION
		}
		return nil
	})

	err := replaceAside(tmp, dst)
	if err == nil {
		t.Fatal("replaceAside should fail when the old file cannot be restored")
	}
	old := dst + ".old"
	for _, want := range []string{old, windows.ERROR_ACCESS_DENIED.Error(), windows.ERROR_SHARING_VIOLATION.Error()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	assertFile(t, old, "old version")
	assertFile(t, tmp, "new version")
}
