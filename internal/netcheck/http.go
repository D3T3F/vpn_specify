package netcheck

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vpn_specify/internal/i18n"
)

// FetchIP gets the public IP from an HTTP service (directly or via a proxy).
func FetchIP(u string, proxy *url.URL) (string, error) {
	tr := &http.Transport{Proxy: nil}
	if proxy != nil {
		tr.Proxy = http.ProxyURL(proxy)
	}
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// Describe formats a check result for display.
func Describe(s string, err error) string {
	if err != nil {
		return i18n.T("error ({0})", err)
	}
	return s
}
