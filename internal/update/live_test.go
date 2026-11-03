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

// TestLiveGitHub baixa de verdade o pacote da última release e confere o
// SHA-256. Só roda com VPN_SPECIFY_LIVE=1 (precisa de internet).
func TestLiveGitHub(t *testing.T) {
	if os.Getenv("VPN_SPECIFY_LIVE") != "1" {
		t.Skip("defina VPN_SPECIFY_LIVE=1 para testar contra o GitHub")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	rel, err := Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("última versão %s: %s (%d bytes, sha256 %s)", rel.Version, rel.Asset.Name, rel.Asset.Size, rel.Asset.SHA256)
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
			t.Fatal("o setup extraído não é um executável")
		}
	}
}
