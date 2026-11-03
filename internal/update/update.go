// Package update finds and downloads new versions from the GitHub releases
// (https://github.com/D3T3F/vpn_specify/releases).
//
// Each release has one package per system, built by scripts\build.ps1:
//   - Windows: vpn_specify-<version>-win64.zip (with vpn_specify-setup.exe);
//   - Linux:   vpn_specify-<version>-linux-<amd64|arm64>.tar.gz.
//
// A download is only accepted if its SHA-256 matches what GitHub reports for
// the file (the API's "digest" field) or a SHA256SUMS file in the release.
// Installing the downloaded version is up to Prepare (apply_*.go).
package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/version"
)

// Repo is the repository whose releases are checked.
const Repo = "D3T3F/vpn_specify"

// ReleasesPage is the releases page (for the user to see in the browser).
const ReleasesPage = "https://github.com/" + Repo + "/releases"

var (
	apiURL         = "https://api.github.com/repos/" + Repo + "/releases/latest"
	downloadPrefix = "https://github.com/" + Repo + "/releases/download/"
	apiClient      = &http.Client{Timeout: 20 * time.Second}
	dlClient       = &http.Client{} // no overall limit: the package may take a while; callers pass a context
)

// maxPackage limits the accepted package size (current ones are ~11 MB).
const maxPackage = 200 << 20

type Release struct {
	Version string // "1.2.3"
	Name    string
	Notes   string // release text (Markdown)
	URL     string // release page
	Asset   Asset
}

type Asset struct {
	Name   string
	URL    string
	Size   int64
	SHA256 string // hex
}

// AssetName is this platform's package name for version ver.
func AssetName(ver string) string {
	switch runtime.GOOS {
	case "windows":
		return "vpn_specify-" + ver + "-win64.zip"
	case "linux":
		return "vpn_specify-" + ver + "-linux-" + runtime.GOARCH + ".tar.gz"
	}
	return ""
}

// parseVersion accepts "1.2.3" or "v1.2.3".
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// IsRelease reports whether ver is a published version ("dev" and the like are not).
func IsRelease(ver string) bool {
	_, ok := parseVersion(ver)
	return ok
}

// Newer reports whether a is newer than b.
func Newer(a, b string) bool {
	va, ok1 := parseVersion(a)
	vb, ok2 := parseVersion(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

func newRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "vpn_specify/"+version.Version)
	return req, nil
}

type ghRelease struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"` // "sha256:<hex>"
	} `json:"assets"`
}

// Latest fetches the most recent release (no drafts or pre-releases).
func Latest(ctx context.Context) (*Release, error) {
	req, err := newRequest(ctx, apiURL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := apiClient.Do(req)
	if err != nil {
		return nil, i18n.Wrap(err, "could not reach GitHub")
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, i18n.Error("no published version yet")
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return nil, i18n.Error("GitHub rate-limited the requests; try again later")
	case resp.StatusCode != http.StatusOK:
		return nil, i18n.Error("GitHub answered {0}", resp.Status)
	}
	var gr ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&gr); err != nil {
		return nil, i18n.Wrap(err, "invalid reply from GitHub")
	}
	if !IsRelease(gr.TagName) {
		return nil, i18n.Error("the latest release ({0}) has no valid version number", strconv.Quote(gr.TagName))
	}
	r := &Release{Version: strings.TrimPrefix(gr.TagName, "v"), Name: gr.Name, Notes: gr.Body, URL: gr.HTMLURL}
	want := AssetName(r.Version)
	sums := ""
	for _, a := range gr.Assets {
		switch {
		case a.Name == want:
			r.Asset = Asset{Name: a.Name, URL: a.URL, Size: a.Size, SHA256: strings.TrimPrefix(a.Digest, "sha256:")}
		case strings.EqualFold(a.Name, "SHA256SUMS") || strings.EqualFold(a.Name, "SHA256SUMS.txt"):
			sums = a.URL
		}
	}
	if r.Asset.Name == "" {
		return r, i18n.Error("version {0} has no package for this system ({1})", r.Version, want)
	}
	if r.Asset.SHA256 == "" && sums != "" {
		if h, err := sumFromFile(ctx, sums, want); err == nil {
			r.Asset.SHA256 = h
		}
	}
	return r, nil
}

// sumFromFile reads the hash of name from a file in sha256sum format.
func sumFromFile(ctx context.Context, url, name string) (string, error) {
	if !strings.HasPrefix(url, downloadPrefix) {
		return "", errors.New("unexpected address")
	}
	req, err := newRequest(ctx, url)
	if err != nil {
		return "", err
	}
	resp, err := apiClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", errors.New("hash not found")
}

// Check returns the release if there is a version newer than this one, or
// nil if already up to date. Builds without a version ("dev") never update.
func Check(ctx context.Context) (*Release, error) {
	if !IsRelease(version.Version) {
		return nil, nil
	}
	r, err := Latest(ctx)
	if err != nil {
		if r != nil && !Newer(r.Version, version.Version) {
			return nil, nil // no package for this system, but it is not newer either
		}
		return nil, err
	}
	if !Newer(r.Version, version.Version) {
		return nil, nil
	}
	return r, nil
}

// Download fetches the package into a new temporary folder and checks its
// SHA-256. progress (optional) receives the downloaded bytes and the total.
func Download(ctx context.Context, a Asset, progress func(done, total int64)) (string, error) {
	if !strings.HasPrefix(a.URL, downloadPrefix) {
		return "", i18n.Error("unexpected download address: {0}", a.URL)
	}
	want, err := hex.DecodeString(a.SHA256)
	if err != nil || len(want) != sha256.Size {
		return "", i18n.Error("the release does not report the package SHA-256; for safety the update was not installed")
	}
	if a.Size > maxPackage {
		return "", i18n.Error("package too large")
	}
	dir, err := os.MkdirTemp("", "vpn_specify-update-")
	if err != nil {
		return "", err
	}
	fail := func(err error) (string, error) {
		os.RemoveAll(dir)
		return "", err
	}
	req, err := newRequest(ctx, a.URL)
	if err != nil {
		return fail(err)
	}
	resp, err := dlClient.Do(req)
	if err != nil {
		return fail(i18n.Wrap(err, "download failed"))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fail(i18n.Error("download failed: {0}", resp.Status))
	}
	p := filepath.Join(dir, path.Base(a.Name))
	f, err := os.Create(p)
	if err != nil {
		return fail(err)
	}
	h := sha256.New()
	pw := &progressWriter{total: a.Size, fn: progress}
	_, err = io.Copy(io.MultiWriter(f, h, pw), io.LimitReader(resp.Body, maxPackage+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fail(i18n.Wrap(err, "download failed"))
	}
	if pw.done > maxPackage {
		return fail(i18n.Error("package too large"))
	}
	if got := h.Sum(nil); !equalBytes(got, want) {
		return fail(i18n.Error("the downloaded package is corrupted or was altered (SHA-256 {0}, expected {1})", hex.EncodeToString(got), hex.EncodeToString(want)))
	}
	return p, nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type progressWriter struct {
	done, total int64
	last        time.Time
	fn          func(done, total int64)
}

func (w *progressWriter) Write(b []byte) (int, error) {
	w.done += int64(len(b))
	if w.fn != nil && (time.Since(w.last) > 150*time.Millisecond || w.done == w.total) {
		w.last = time.Now()
		w.fn(w.done, w.total)
	}
	return len(b), nil
}

// CleanupOld deletes old update folders (more than a day old).
func CleanupOld() {
	ents, _ := os.ReadDir(os.TempDir())
	for _, e := range ents {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "vpn_specify-update-") {
			continue
		}
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > 24*time.Hour {
			os.RemoveAll(filepath.Join(os.TempDir(), e.Name()))
		}
	}
}

// extractZipFile extracts from the zip the file named base (in any folder).
func extractZipFile(archive, base, dst string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		// zips made by Compress-Archive (PowerShell 5.1) use "\" as separator
		if path.Base(strings.ReplaceAll(f.Name, `\`, "/")) != base || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return writeFile(dst, io.LimitReader(rc, maxPackage), 0o755)
	}
	return i18n.Error("the package does not contain {0}", base)
}

// extractTarFile extracts from the .tar.gz the regular file named base.
func extractTarFile(archive, base, dst string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return i18n.Error("the package does not contain {0}", base)
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeReg && path.Base(h.Name) == base {
			return writeFile(dst, io.LimitReader(tr, maxPackage), 0o755)
		}
	}
}

func writeFile(dst string, r io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, r)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// Plan is an update ready to apply. Run is called after vpn_specify has
// disconnected and released its ports, right before exiting.
type Plan struct {
	Description string
	Run         func() error
}
