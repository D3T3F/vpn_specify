//go:build windows

package ovpninstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestInstallVerifiesTheDownload(t *testing.T) {
	body := []byte("fake msi")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	oldURL, oldSum, oldRun := msiURL, msiSHA256, runMSI
	defer func() { msiURL, msiSHA256, runMSI = oldURL, oldSum, oldRun }()
	msiURL = srv.URL

	ran := ""
	runMSI = func(ctx context.Context, msi string) error {
		ran = msi
		if b, _ := os.ReadFile(msi); string(b) != string(body) {
			t.Errorf("wrong file content: %q", b)
		}
		return nil
	}
	sum := sha256.Sum256(body)
	msiSHA256 = hex.EncodeToString(sum[:])
	var steps []string
	if err := Install(context.Background(), func(s string) { steps = append(steps, s) }); err != nil {
		t.Fatal(err)
	}
	if ran == "" || len(steps) != 2 {
		t.Fatalf("ran %q, steps %q", ran, steps)
	}
	if _, err := os.Stat(ran); !os.IsNotExist(err) {
		t.Error("the downloaded MSI was left behind")
	}

	// a different file must never reach msiexec
	ran = ""
	msiSHA256 = strings.Repeat("0", 64)
	if err := Install(context.Background(), func(string) {}); err == nil || ran != "" {
		t.Fatalf("a wrong hash must fail before installing: %v, ran %q", err, ran)
	}
}

func TestPinnedURLIsTheOfficialOne(t *testing.T) {
	if !strings.HasPrefix(msiURL, "https://swupdate.openvpn.org/community/releases/") || len(msiSHA256) != 64 {
		t.Errorf("unexpected pin: %s %s", msiURL, msiSHA256)
	}
}
