package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vpn_specify/internal/version"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.0.1", "1.0.0", true}, {"v1.10.0", "1.9.9", true}, {"2.0.0", "1.99.99", true},
		{"1.0.0", "1.0.0", false}, {"1.0.0", "1.0.1", false}, {"1.0.0", "dev", false},
		{"dev", "1.0.0", false}, {"1.0", "0.9.0", false}, {"1.0.0-rc1", "0.9.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
	if IsRelease("dev") || !IsRelease("v1.2.3") {
		t.Error("IsRelease is wrong")
	}
}

// fakeGitHub serves /releases/latest and the release files.
func fakeGitHub(t *testing.T, tag string, files map[string][]byte, digests map[string]string) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	oldAPI, oldPrefix := apiURL, downloadPrefix
	apiURL = srv.URL + "/api/latest"
	downloadPrefix = srv.URL + "/download/"
	t.Cleanup(func() { apiURL, downloadPrefix = oldAPI, oldPrefix })

	var assets []string
	for name, data := range files {
		d := digests[name]
		assets = append(assets, fmt.Sprintf(`{"name":%q,"browser_download_url":%q,"size":%d,"digest":%q}`,
			name, downloadPrefix+tag+"/"+name, len(data), d))
		data := data
		mux.HandleFunc("/download/"+tag+"/"+name, func(w http.ResponseWriter, r *http.Request) { w.Write(data) })
	}
	mux.HandleFunc("/api/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			http.Error(w, "no User-Agent", http.StatusForbidden)
			return
		}
		fmt.Fprintf(w, `{"tag_name":%q,"name":"vpn_specify","body":"news","html_url":"https://github.com/D3T3F/vpn_specify/releases/tag/%s","assets":[%s]}`,
			tag, tag, strings.Join(assets, ","))
	})
}

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func TestCheckAndDownload(t *testing.T) {
	old := version.Version
	version.Version = "1.0.0"
	t.Cleanup(func() { version.Version = old })

	pkg := []byte("package content")
	name := AssetName("1.2.0")
	fakeGitHub(t, "v1.2.0", map[string][]byte{name: pkg, "outro.zip": []byte("x")}, map[string]string{name: "sha256:" + sha(pkg)})

	rel, err := Check(context.Background())
	if err != nil || rel == nil || rel.Version != "1.2.0" || rel.Asset.Name != name || rel.Notes != "news" {
		t.Fatalf("Check = %+v, %v", rel, err)
	}
	var lastDone int64
	p, err := Download(context.Background(), rel.Asset, func(done, total int64) { lastDone = done })
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(p))
	if b, _ := os.ReadFile(p); !bytes.Equal(b, pkg) || lastDone != int64(len(pkg)) {
		t.Fatalf("wrong download: %q (progress %d)", b, lastDone)
	}

	// different hash: refused, no file left behind
	bad := rel.Asset
	bad.SHA256 = sha([]byte("something else"))
	if _, err := Download(context.Background(), bad, nil); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("a wrong hash should fail: %v", err)
	}
	// no hash: refused
	bad.SHA256 = ""
	if _, err := Download(context.Background(), bad, nil); err == nil {
		t.Error("no hash should fail")
	}
	// an address outside the repository's releases: refused
	bad = rel.Asset
	bad.URL = "https://example.com/" + name
	if _, err := Download(context.Background(), bad, nil); err == nil {
		t.Error("an outside URL should fail")
	}

	// same version: nothing to do
	version.Version = "1.2.0"
	if rel, err := Check(context.Background()); err != nil || rel != nil {
		t.Errorf("same version: %+v, %v", rel, err)
	}
	// a development build never updates
	version.Version = "dev"
	if rel, err := Check(context.Background()); err != nil || rel != nil {
		t.Errorf("dev: %+v, %v", rel, err)
	}
}

func TestSHA256SUMSFallback(t *testing.T) {
	old := version.Version
	version.Version = "1.0.0"
	t.Cleanup(func() { version.Version = old })
	pkg := []byte("package")
	name := AssetName("1.1.0")
	sums := []byte(sha(pkg) + "  " + name + "\n")
	fakeGitHub(t, "v1.1.0", map[string][]byte{name: pkg, "SHA256SUMS": sums}, nil)
	rel, err := Check(context.Background())
	if err != nil || rel == nil || rel.Asset.SHA256 != sha(pkg) {
		t.Fatalf("Check = %+v, %v", rel, err)
	}
}

func TestExtract(t *testing.T) {
	dir := t.TempDir()
	// zip with the setup inside a folder
	zp := filepath.Join(dir, "a.zip")
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, _ := zw.Create("vpn_specify-1.2.0-win64/vpn_specify-setup.exe")
	w.Write([]byte("MZsetup"))
	zw.Close()
	os.WriteFile(zp, zb.Bytes(), 0o644)
	out := filepath.Join(dir, "setup.exe")
	if err := extractZipFile(zp, "vpn_specify-setup.exe", out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "MZsetup" {
		t.Errorf("zip: %q", b)
	}
	if err := extractZipFile(zp, "missing", out); err == nil {
		t.Error("a missing file should fail")
	}

	// zip from Compress-Archive (PowerShell 5.1): "\" separator (the case of v1.0.0)
	zb.Reset()
	zw = zip.NewWriter(&zb)
	w, _ = zw.Create(`vpn_specify-1.0.0-win64\vpn_specify-setup.exe`)
	w.Write([]byte("MZold"))
	zw.Close()
	os.WriteFile(zp, zb.Bytes(), 0o644)
	if err := extractZipFile(zp, "vpn_specify-setup.exe", out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "MZold" {
		t.Errorf("zip with backslashes: %q", b)
	}

	// tar.gz like the one from build.ps1 -Linux
	tp := filepath.Join(dir, "a.tar.gz")
	var tb bytes.Buffer
	gz := gzip.NewWriter(&tb)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "./vpn_specify-1.2.0-linux-amd64/", Typeflag: tar.TypeDir, Mode: 0o755})
	tw.WriteHeader(&tar.Header{Name: "./vpn_specify-1.2.0-linux-amd64/vpn_specify", Typeflag: tar.TypeReg, Mode: 0o755, Size: 4})
	tw.Write([]byte("\x7fELF"))
	tw.Close()
	gz.Close()
	os.WriteFile(tp, tb.Bytes(), 0o644)
	out = filepath.Join(dir, "vpn_specify")
	if err := extractTarFile(tp, "vpn_specify", out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "\x7fELF" {
		t.Errorf("tar: %q", b)
	}
}
