package mobile

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"vpn_specify/internal/latency"
	"vpn_specify/internal/riseup"
	"vpn_specify/internal/vpngate"
)

func call(t *testing.T, req Request) (result json.RawMessage, errMsg string) {
	t.Helper()
	b, _ := json.Marshal(req)
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(Call(b), &r); err != nil {
		t.Fatal(err)
	}
	return r.Result, r.Error
}

func TestLanguages(t *testing.T) {
	res, e := call(t, Request{Method: "setLanguage", Text: "pt-BR"})
	if e != "" {
		t.Fatal(e)
	}
	var cat map[string]string
	if err := json.Unmarshal(res, &cat); err != nil || cat["Disconnect"] == "" {
		t.Fatalf("catalog: %v, %d entries", err, len(cat))
	}
	// errors coming from Go follow the same language
	_, e = call(t, Request{Method: "plan", Config: "[Interface]\n"})
	call(t, Request{Method: "setLanguage", Text: "en"})
	if e == "" || strings.Contains(e, "PrivateKey missing") {
		t.Errorf("error not translated: %q", e)
	}
	res, _ = call(t, Request{Method: "matchLanguage", Text: "pt-PT"})
	if string(res) != `"pt-BR"` {
		t.Errorf("match: %s", res)
	}
}

func TestBadRequests(t *testing.T) {
	if _, e := call(t, Request{Method: "nope"}); e == "" {
		t.Error("unknown method must fail")
	}
	if !strings.Contains(string(Call([]byte("not json"))), "error") {
		t.Error("bad JSON must fail")
	}
	// with nothing running these are harmless
	for _, m := range []string{"stop", "rebind", "kick", "stats", "log"} {
		if _, e := call(t, Request{Method: m}); e != "" {
			t.Errorf("%s: %s", m, e)
		}
	}
}

// riseupPEM returns a new RSA key and a self-signed certificate of it, as PEM.
func riseupPEM(t *testing.T) (keyPEM, certPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return keyPEM, certPEM
}

func TestRiseUp(t *testing.T) {
	eip, err := os.ReadFile("../riseup/testdata/eip.json")
	if err != nil {
		t.Fatal(err)
	}
	res, e := call(t, Request{Method: "riseupList", RiseupEIP: string(eip)})
	if e != "" {
		t.Fatal(e)
	}
	var l riseup.List
	if err := json.Unmarshal(res, &l); err != nil || len(l.Servers) != 32 {
		t.Fatalf("list: %v, %d servers", err, len(l.Servers))
	}

	key, cert := riseupPEM(t)
	_, ca := riseupPEM(t)
	res, e = call(t, Request{Method: "riseupConfig", Text: "vpn22-mia.riseup.net-udp-1194",
		RiseupCert: key + cert, RiseupCA: ca})
	if e != "" {
		t.Fatal(e)
	}
	var out struct {
		Config string `json:"config"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "RiseUp Miami vpn22-mia UDP" || !strings.Contains(out.Config, "remote 89.187.173.169 1194") {
		t.Errorf("config of %q: %.120s", out.Name, out.Config)
	}

	if _, e := call(t, Request{Method: "riseupConfig", Text: "vpn22-mia.riseup.net-udp-1194",
		RiseupCert: cert, RiseupCA: ca}); e == "" {
		t.Error("certificate without key: expected an error")
	}
	if _, e := call(t, Request{Method: "riseupConfig", Text: "unknown", RiseupCert: key + cert, RiseupCA: ca}); e == "" {
		t.Error("unknown server: expected an error")
	}
	if _, e := call(t, Request{Method: "riseupList", RiseupEIP: "<html>blocked</html>"}); e == "" {
		t.Error("garbage list: expected an error")
	}
}

// closedPort returns a port of 127.0.0.1 where nothing listens.
func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// openPort returns a port of 127.0.0.1 that accepts connections until the test ends.
func openPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestBest(t *testing.T) {
	gateMu.Lock()
	gateList = nil
	gateMu.Unlock()
	riseupMu.Lock()
	riseupList = nil
	riseupMu.Unlock()
	for _, m := range []string{"vpnGateBest", "riseupBest"} {
		if _, e := call(t, Request{Method: m}); e == "" {
			t.Errorf("%s without a list: expected an error", m)
		}
	}

	up, down := openPort(t), closedPort(t)
	gateMu.Lock()
	gateList = &vpngate.List{Servers: []vpngate.Server{
		{ID: "up", IP: "127.0.0.1", Proto: "tcp", Port: up},
		{ID: "down", IP: "127.0.0.1", Proto: "tcp", Port: down},
	}}
	gateMu.Unlock()
	t.Cleanup(func() { gateMu.Lock(); gateList = nil; gateMu.Unlock() })
	riseupMu.Lock()
	riseupList = &riseup.List{Servers: []riseup.Server{
		{ID: "u", Host: "up.test", IP: "127.0.0.1", Proto: "tcp", Port: up},
		{ID: "d", Host: "down.test", IP: "127.0.0.1", Proto: "tcp", Port: down},
	}}
	riseupMu.Unlock()
	t.Cleanup(func() { riseupMu.Lock(); riseupList = nil; riseupMu.Unlock() })

	check := func(method, upID, downID string) {
		t.Helper()
		res, e := call(t, Request{Method: method})
		if e != "" {
			t.Fatal(e)
		}
		var got []latency.Result
		if err := json.Unmarshal(res, &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].ID != upID || got[0].Ms < 1 || got[1].ID != downID || got[1].Ms != -1 {
			t.Errorf("%s: %+v", method, got)
		}
	}
	check("vpnGateBest", "up", "down")
	check("riseupBest", "up.test", "down.test")
}
