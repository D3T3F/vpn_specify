package proxy

import (
	"context"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"vpn_specify/internal/tunnel"
)

// ---------- HTTP proxy (CONNECT + plain http:// requests) ----------

type httpProxy struct {
	tun     tunnel.Tunnel
	tr      *http.Transport
	verbose bool
}

func ServeHTTP(ln net.Listener, tun tunnel.Tunnel, verbose bool) {
	p := &httpProxy{
		tun:     tun,
		verbose: verbose,
		tr: &http.Transport{
			Proxy:               nil,
			DialContext:         tun.DialContext,
			MaxIdleConns:        100,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 15 * time.Second,
		},
	}
	srv := &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second}
	_ = srv.Serve(ln)
}

var hopHeaders = []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
	"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"}

func removeHopHeaders(h http.Header) {
	for _, v := range h.Values("Connection") {
		for _, k := range strings.Split(v, ",") {
			h.Del(strings.TrimSpace(k))
		}
	}
	for _, k := range hopHeaders {
		h.Del(k)
	}
}

func (p *httpProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.connect(w, r)
		return
	}
	if !r.URL.IsAbs() {
		http.Error(w, "vpn_specify: this is an HTTP proxy", http.StatusBadRequest)
		return
	}
	out := r.Clone(r.Context())
	out.RequestURI = ""
	removeHopHeaders(out.Header)
	resp, err := p.tr.RoundTrip(out)
	if err != nil {
		if p.verbose {
			log.Printf("http %s: %v", r.URL, err)
		}
		http.Error(w, "vpn_specify: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	removeHopHeaders(resp.Header)
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	rc := http.NewResponseController(w)
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			_ = rc.Flush()
		}
		if err != nil {
			return
		}
	}
}

func (p *httpProxy) connect(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	up, err := p.tun.DialContext(ctx, "tcp", r.Host)
	cancel()
	if err != nil {
		if p.verbose {
			log.Printf("connect %s: %v", r.Host, err)
		}
		http.Error(w, "vpn_specify: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer up.Close()
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	c, brw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer c.Close()
	if _, err := c.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}
	if p.verbose {
		log.Printf("connect -> %s", r.Host)
	}
	Pipe(c, brw.Reader, up)
}
