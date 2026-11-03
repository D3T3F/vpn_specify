package update

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestLiveGitHub really downloads the latest release package and checks its
// SHA-256. It only runs with VPN_SPECIFY_LIVE=1 (needs internet).
func TestLiveGitHub(t *testing.T) {
	if os.Getenv("VPN_SPECIFY_LIVE") != "1" {
		t.Skip("set VPN_SPECIFY_LIVE=1 to test against GitHub")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	rel, err := Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("latest version %s: %s (%d bytes, sha256 %s)", rel.Version, rel.Asset.Name, rel.Asset.Size, rel.Asset.SHA256)
	p, err := Download(ctx, rel.Asset, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(p))
	if runtime.GOOS == "windows" {
		out := filepath.Join(filepath.Dir(p), "vpn_specify-setup.exe")
		if err := extractZipFile(p, "vpn_specify-setup.exe", out); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(out); !bytes.HasPrefix(b, []byte("MZ")) {
			t.Fatal("the extracted setup is not an executable")
		}
	}
}
