package tunnel

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTailReaderFollowsTheWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openvpn.log")
	if err := os.WriteFile(path, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	r, err := openTail(path, done)
	if err != nil {
		t.Fatal(err)
	}
	defer r.f.Close()
	go func() {
		time.Sleep(300 * time.Millisecond)
		f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		f.WriteString("second\n")
		f.Close()
		close(done) // the writer is finished: the reader gives up at the end
	}()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "first\nsecond\n" {
		t.Errorf("read %q", b)
	}
}

func TestTailReaderEndsWhenDoneAndEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.log")
	os.WriteFile(path, nil, 0o600)
	done := make(chan struct{})
	close(done)
	r, err := openTail(path, done)
	if err != nil {
		t.Fatal(err)
	}
	defer r.f.Close()
	if b, err := io.ReadAll(r); err != nil || len(b) != 0 {
		t.Errorf("read %q, %v", b, err)
	}
}
