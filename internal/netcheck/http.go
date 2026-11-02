package netcheck

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FetchIP busca o IP público num serviço HTTP (direto ou por um proxy).
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

// Describe formata o resultado de uma verificação para exibir.
func Describe(s string, err error) string {
	if err != nil {
		return "erro (" + err.Error() + ")"
	}
	return s
}
