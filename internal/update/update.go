// Package update procura e baixa novas versões nas releases do GitHub
// (https://github.com/D3T3F/vpn_specify/releases).
//
// Cada release tem um pacote por sistema, gerado pelo scripts\build.ps1:
//   - Windows: vpn_specify-<versão>-win64.zip (com o vpn_specify-setup.exe);
//   - Linux:   vpn_specify-<versão>-linux-<amd64|arm64>.tar.gz.
//
// O download só é aceito se o SHA-256 bater com o que o GitHub informa para
// o arquivo (campo "digest" da API) ou com um arquivo SHA256SUMS da release.
// Instalar a versão baixada fica a cargo de Prepare (apply_*.go).
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
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"vpn_specify/internal/version"
)

// Repo é o repositório cujas releases são consultadas.
const Repo = "D3T3F/vpn_specify"

// ReleasesPage é a página das releases (para o usuário ver no navegador).
const ReleasesPage = "https://github.com/" + Repo + "/releases"

var (
	apiURL         = "https://api.github.com/repos/" + Repo + "/releases/latest"
	downloadPrefix = "https://github.com/" + Repo + "/releases/download/"
	apiClient      = &http.Client{Timeout: 20 * time.Second}
	dlClient       = &http.Client{} // sem limite total: o pacote pode demorar; quem chama passa um contexto
)

// maxPackage limita o tamanho aceito de um pacote (os atuais têm ~11 MB).
const maxPackage = 200 << 20

type Release struct {
	Version string // "1.2.3"
	Name    string
	Notes   string // texto da release (Markdown)
	URL     string // página da release
	Asset   Asset
}

type Asset struct {
	Name   string
	URL    string
	Size   int64
	SHA256 string // hex
}

// AssetName é o nome do pacote desta plataforma para a versão ver.
func AssetName(ver string) string {
	switch runtime.GOOS {
	case "windows":
		return "vpn_specify-" + ver + "-win64.zip"
	case "linux":
		return "vpn_specify-" + ver + "-linux-" + runtime.GOARCH + ".tar.gz"
	}
	return ""
}

// parseVersion aceita "1.2.3" ou "v1.2.3".
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

// IsRelease diz se ver é uma versão publicada ("dev" e afins não são).
func IsRelease(ver string) bool {
	_, ok := parseVersion(ver)
	return ok
}

// Newer diz se a é mais nova que b.
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

// Latest consulta a release mais recente (sem rascunhos nem pré-releases).
func Latest(ctx context.Context) (*Release, error) {
	req, err := newRequest(ctx, apiURL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := apiClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("não consegui consultar o GitHub: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errors.New("nenhuma versão publicada ainda")
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("o GitHub limitou as consultas; tente de novo mais tarde")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("o GitHub respondeu %s", resp.Status)
	}
	var gr ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&gr); err != nil {
		return nil, fmt.Errorf("resposta inválida do GitHub: %w", err)
	}
	if !IsRelease(gr.TagName) {
		return nil, fmt.Errorf("a última release (%q) não tem um número de versão válido", gr.TagName)
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
		return r, fmt.Errorf("a versão %s não tem pacote para este sistema (%s)", r.Version, want)
	}
	if r.Asset.SHA256 == "" && sums != "" {
		if h, err := sumFromFile(ctx, sums, want); err == nil {
			r.Asset.SHA256 = h
		}
	}
	return r, nil
}

// sumFromFile lê o hash de name num arquivo no formato do sha256sum.
func sumFromFile(ctx context.Context, url, name string) (string, error) {
	if !strings.HasPrefix(url, downloadPrefix) {
		return "", errors.New("endereço inesperado")
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
	return "", errors.New("hash não encontrado")
}

// Check devolve a release se houver uma versão mais nova que esta, ou nil
// se já está atualizado. Builds sem versão ("dev") nunca se atualizam.
func Check(ctx context.Context) (*Release, error) {
	if !IsRelease(version.Version) {
		return nil, nil
	}
	r, err := Latest(ctx)
	if err != nil {
		if r != nil && !Newer(r.Version, version.Version) {
			return nil, nil // sem pacote para este sistema, mas também não é mais nova
		}
		return nil, err
	}
	if !Newer(r.Version, version.Version) {
		return nil, nil
	}
	return r, nil
}

// Download baixa o pacote numa pasta temporária nova e confere o SHA-256.
// progress (opcional) recebe bytes baixados e o total.
func Download(ctx context.Context, a Asset, progress func(done, total int64)) (string, error) {
	if !strings.HasPrefix(a.URL, downloadPrefix) {
		return "", fmt.Errorf("endereço de download inesperado: %s", a.URL)
	}
	want, err := hex.DecodeString(a.SHA256)
	if err != nil || len(want) != sha256.Size {
		return "", errors.New("a release não informa o SHA-256 do pacote; por segurança a atualização não foi instalada")
	}
	if a.Size > maxPackage {
		return "", errors.New("pacote grande demais")
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
		return fail(fmt.Errorf("download falhou: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fail(fmt.Errorf("download falhou: %s", resp.Status))
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
		return fail(fmt.Errorf("download falhou: %w", err))
	}
	if pw.done > maxPackage {
		return fail(errors.New("pacote grande demais"))
	}
	if got := h.Sum(nil); !equalBytes(got, want) {
		return fail(fmt.Errorf("o pacote baixado está corrompido ou foi alterado (SHA-256 %x, esperado %x)", got, want))
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

// CleanupOld apaga pastas de atualizações antigas (mais de um dia).
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

// extractZipFile extrai do zip o arquivo de nome base (em qualquer pasta).
func extractZipFile(archive, base, dst string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		// zips do Compress-Archive (PowerShell 5.1) usam "\" como separador
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
	return fmt.Errorf("o pacote não contém %s", base)
}

// extractTarFile extrai do .tar.gz o arquivo comum de nome base.
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
			return fmt.Errorf("o pacote não contém %s", base)
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

// Plan é a atualização pronta para aplicar. Run é chamado depois que o
// vpn_specify desconectou e liberou as portas, logo antes de sair.
type Plan struct {
	Description string
	Run         func() error
}
