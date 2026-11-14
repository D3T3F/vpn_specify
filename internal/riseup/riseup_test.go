package riseup

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fake serves the three endpoints from fields and counts the certificate requests.
type fake struct {
	eip, cert, ca string
	eipStatus     int
	certHits      atomic.Int32
}

// serve points the endpoints at a local server for the rest of the test.
func serve(t *testing.T, f *fake) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/eip", func(w http.ResponseWriter, r *http.Request) {
		if f.eipStatus != 0 {
			w.WriteHeader(f.eipStatus)
		}
		io.WriteString(w, f.eip)
	})
	mux.HandleFunc("/cert", func(w http.ResponseWriter, r *http.Request) {
		f.certHits.Add(1)
		io.WriteString(w, f.cert)
	})
	mux.HandleFunc("/ca", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, f.ca)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	oldG, oldC, oldA := gatewaysURL, certURL, caURL
	gatewaysURL, certURL, caURL = srv.URL+"/eip", srv.URL+"/cert", srv.URL+"/ca"
	t.Cleanup(func() { gatewaysURL, certURL, caURL = oldG, oldC, oldA })
}

// selfSigned returns a new RSA key and a self-signed certificate, as PEM.
func selfSigned(t *testing.T) (keyPEM, certPEM string) {
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

func sample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/eip.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseSample(t *testing.T) {
	servers, err := Parse(sample(t))
	if err != nil {
		t.Fatal(err)
	}
	// The sample has 16 gateways, all with OpenVPN over tcp and udp, plus obfs4.
	if len(servers) != 32 {
		t.Fatalf("got %d servers, want 32", len(servers))
	}
	if servers[0].Country != "CA" {
		t.Errorf("sorted by country: first is %q, want CA", servers[0].Country)
	}
	var found bool
	for _, s := range servers {
		if s.Port != 1194 && s.Port != 80 {
			t.Errorf("unexpected port %d in %+v", s.Port, s)
		}
		if s.ID == "vpn22-mia.riseup.net-udp-1194" {
			found = true
			want := Server{ID: s.ID, Name: "RiseUp Miami vpn22-mia UDP", Host: "vpn22-mia.riseup.net",
				IP: "89.187.173.169", Country: "US", City: "Miami", Proto: "udp", Port: 1194}
			if s != want {
				t.Errorf("got %+v, want %+v", s, want)
			}
		}
	}
	if !found {
		t.Error("vpn22-mia udp server missing")
	}
}

func TestParseSkipsLimitedAndOtherTransports(t *testing.T) {
	servers, err := Parse([]byte(`{
		"gateways": [
			{"host": "limited.riseup.net", "ip_address": "10.0.0.1", "location": "Paris",
			 "capabilities": {"limited": true, "transport": [{"type": "openvpn", "protocols": ["udp"], "ports": ["1194"]}]}},
			{"host": "obfs.riseup.net", "ip_address": "10.0.0.2", "location": "Paris",
			 "capabilities": {"transport": [{"type": "obfs4", "protocols": ["tcp"], "ports": ["9443"]}]}},
			{"host": "udponly.riseup.net", "ip_address": "10.0.0.3", "location": "Paris",
			 "capabilities": {"transport": [{"type": "openvpn", "protocols": ["udp"], "ports": ["1194"]}]}},
			{"host": "bad host.riseup.net", "ip_address": "10.0.0.4", "location": "Paris",
			 "capabilities": {"transport": [{"type": "openvpn", "protocols": ["udp"], "ports": ["1194"]}]}},
			{"host": "badip.riseup.net", "ip_address": "not-an-ip", "location": "Paris",
			 "capabilities": {"transport": [{"type": "openvpn", "protocols": ["udp"], "ports": ["1194"]}]}}
		],
		"locations": {"Paris": {"country_code": "FR", "name": "Paris"}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].ID != "udponly.riseup.net-udp-1194" {
		t.Fatalf("got %+v, want only the udp server of udponly", servers)
	}
}

func TestParseRejectsOtherFormats(t *testing.T) {
	for _, in := range []string{"<html>blocked</html>", "not json"} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
}

func TestFetchDoesNotRequestCertificate(t *testing.T) {
	f := &fake{eip: string(sample(t))}
	serve(t, f)
	l, err := Fetch(context.Background())
	if err != nil || len(l.Servers) != 32 {
		t.Fatalf("servers %d, err %v", len(l.Servers), err)
	}
	if n := f.certHits.Load(); n != 0 {
		t.Errorf("certificate requested %d times while listing", n)
	}
}

func TestFetchErrors(t *testing.T) {
	f := &fake{eip: string(sample(t)), eipStatus: http.StatusInternalServerError}
	serve(t, f)
	if _, err := Fetch(context.Background()); err == nil {
		t.Error("HTTP error: expected an error")
	}

	big := strings.Repeat("a", maxBody+1)
	f2 := &fake{eip: big}
	serve(t, f2)
	if _, err := Fetch(context.Background()); err == nil {
		t.Error("oversized list: expected an error")
	}
}

// The directives a configuration may have: the same kind of allow list that
// vpngate applies to the configurations it takes from the network.
var allowedDirectives = map[string]bool{
	"client": true, "dev": true, "proto": true, "remote": true, "auth": true, "data-ciphers": true,
	"float": true, "keepalive": true, "nobind": true, "persist-key": true, "rcvbuf": true,
	"sndbuf": true, "verb": true, "remote-cert-tls": true, "resolv-retry": true,
}

func TestConfigPassesAllowList(t *testing.T) {
	key, cert := selfSigned(t)
	_, ca := selfSigned(t)
	// Text outside the PEM blocks must not reach the configuration.
	f := &fake{eip: string(sample(t)), cert: "up /bin/sh\n" + key + cert + "script-security 2\n", ca: "x\n" + ca}
	serve(t, f)
	l, err := Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s, cfg, err := l.Config(context.Background(), "vpn22-mia.riseup.net-udp-1194")
	if err != nil {
		t.Fatal(err)
	}
	if f.certHits.Load() != 1 || s.Name != "RiseUp Miami vpn22-mia UDP" {
		t.Fatalf("certificate requests %d, server %+v", f.certHits.Load(), s)
	}

	want := []string{
		"client", "dev tun", "proto udp", "remote 89.187.173.169 1194",
		"auth SHA512", "data-ciphers AES-256-GCM", "float", "keepalive 10 30", "nobind",
		"persist-key", "rcvbuf 0", "sndbuf 0", "verb 3", "remote-cert-tls server", "resolv-retry infinite",
	}
	var got, blocks []string
	inBlock := false
	for _, raw := range strings.Split(string(cfg), "\n") {
		line := strings.TrimSpace(raw)
		if inBlock {
			if strings.HasPrefix(line, "</") {
				inBlock = false
			}
			continue
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "<") {
			blocks = append(blocks, strings.Trim(line, "<>"))
			inBlock = true
			continue
		}
		if !allowedDirectives[strings.Fields(line)[0]] {
			t.Errorf("directive not allowed: %q", line)
		}
		got = append(got, line)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if strings.Join(blocks, ",") != "ca,cert,key" {
		t.Errorf("blocks %v", blocks)
	}
	for _, p := range []string{
		"<ca>\n" + ca + "</ca>\n",
		"<cert>\n" + cert + "</cert>\n",
		"<key>\n" + key + "</key>\n",
	} {
		if !strings.Contains(string(cfg), p) {
			t.Errorf("block missing or altered:\n%s", p)
		}
	}
	if strings.Contains(string(cfg), "/bin/sh") || strings.Contains(string(cfg), "script-security") {
		t.Error("text from the network reached the configuration")
	}
}

func TestConfigRejectsBadCredentials(t *testing.T) {
	key, cert := selfSigned(t)
	otherKey, _ := selfSigned(t)
	_, ca := selfSigned(t)
	encrypted := string(pem.EncodeToMemory(&pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: map[string]string{"Proc-Type": "4,ENCRYPTED"},
		Bytes:   []byte("x"),
	}))
	wrongType := strings.Replace(key, "RSA PRIVATE KEY", "PUBLIC KEY", 2)

	cases := map[string]struct{ cert, ca string }{
		"garbage":        {"<html>blocked</html>", ca},
		"no certificate": {key, ca},
		"no key":         {cert, ca},
		"wrong type":     {wrongType + cert, ca},
		"encrypted key":  {encrypted + cert, ca},
		"mismatch":       {otherKey + cert, ca},
		"bad CA":         {key + cert, "garbage"},
	}
	for name, c := range cases {
		f := &fake{eip: string(sample(t)), cert: c.cert, ca: c.ca}
		serve(t, f)
		l, err := Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := l.Config(context.Background(), "vpn22-mia.riseup.net-udp-1194"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestConfigUnknownServer(t *testing.T) {
	var l *List
	if _, _, err := l.Config(context.Background(), "x"); err == nil {
		t.Error("nil list: expected an error")
	}
	l = &List{Servers: []Server{}}
	if _, _, err := l.Config(context.Background(), "x"); err == nil {
		t.Error("unknown id: expected an error")
	}
}

// ecSelfSigned returns one EC key and a certificate of it, with the key in the
// SEC 1 and the PKCS#8 encodings.
func ecSelfSigned(t *testing.T) (sec1, pkcs8, certPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-ec"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	s1, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	p8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	sec1 = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: s1}))
	pkcs8 = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: p8}))
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return sec1, pkcs8, certPEM
}

func TestConfigFromDataKeyTypes(t *testing.T) {
	list, err := Build(sample(t))
	if err != nil {
		t.Fatal(err)
	}
	const id = "vpn22-mia.riseup.net-udp-1194"
	rsaKey, rsaCert := selfSigned(t)
	_, ca := selfSigned(t)
	ecSEC1, ecPKCS8, ecCert := ecSelfSigned(t)

	cases := map[string]struct{ key, cert, wantType string }{
		"RSA PKCS#1": {rsaKey, rsaCert, "RSA PRIVATE KEY"},
		"EC SEC 1":   {ecSEC1, ecCert, "EC PRIVATE KEY"},
		"PKCS#8":     {ecPKCS8, ecCert, "PRIVATE KEY"},
	}
	for name, c := range cases {
		_, cfg, err := list.ConfigFromData(id, []byte(c.key+c.cert), []byte(ca))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		// The key is written again with the type it had.
		if !strings.Contains(string(cfg), "<key>\n-----BEGIN "+c.wantType+"-----") {
			t.Errorf("%s: key block not re-encoded as %s", name, c.wantType)
		}
	}
}

func TestConfigFromDataRejectsBadData(t *testing.T) {
	list, err := Build(sample(t))
	if err != nil {
		t.Fatal(err)
	}
	const id = "vpn22-mia.riseup.net-udp-1194"
	key, cert := selfSigned(t)
	_, ca := selfSigned(t)
	if _, _, err := list.ConfigFromData(id, []byte(key+cert), []byte(ca)); err != nil {
		t.Fatalf("valid data refused: %v", err)
	}
	cases := map[string]struct{ cert, ca string }{
		"garbage cert": {"<html>blocked</html>", ca},
		"no key":       {cert, ca},
		"garbage CA":   {key + cert, "garbage"},
		"no CA":        {key + cert, ""},
	}
	for name, c := range cases {
		if _, _, err := list.ConfigFromData(id, []byte(c.cert), []byte(c.ca)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, _, err := list.ConfigFromData("unknown", []byte(key+cert), []byte(ca)); err == nil {
		t.Error("unknown id: expected an error")
	}
}

func TestBuildFromBytes(t *testing.T) {
	l, err := Build(sample(t))
	if err != nil || len(l.Servers) != 32 {
		t.Fatalf("servers %v, err %v", l, err)
	}
	if _, err := Build([]byte("<html>blocked</html>")); err == nil {
		t.Error("garbage list: expected an error")
	}
}

// Two gateways of one city with the same first label get names with their full
// host; other names stay short.
func TestNamesAreUnique(t *testing.T) {
	servers, err := Parse([]byte(`{
		"gateways": [
			{"host": "vpn1.a.riseup.net", "ip_address": "10.0.0.1", "location": "Paris",
			 "capabilities": {"transport": [{"type": "openvpn", "protocols": ["udp"], "ports": ["1194"]}]}},
			{"host": "vpn1.b.riseup.net", "ip_address": "10.0.0.2", "location": "Paris",
			 "capabilities": {"transport": [{"type": "openvpn", "protocols": ["udp"], "ports": ["1194"]}]}},
			{"host": "vpn2.riseup.net", "ip_address": "10.0.0.3", "location": "Paris",
			 "capabilities": {"transport": [{"type": "openvpn", "protocols": ["udp"], "ports": ["1194"]}]}}
		],
		"locations": {"Paris": {"country_code": "FR", "name": "Paris"}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"vpn1.a.riseup.net-udp-1194": "RiseUp Paris vpn1.a.riseup.net UDP",
		"vpn1.b.riseup.net-udp-1194": "RiseUp Paris vpn1.b.riseup.net UDP",
		"vpn2.riseup.net-udp-1194":   "RiseUp Paris vpn2 UDP",
	}
	for _, s := range servers {
		if want[s.ID] != s.Name {
			t.Errorf("%s: name %q, want %q", s.ID, s.Name, want[s.ID])
		}
	}
	if len(servers) != len(want) {
		t.Fatalf("got %d servers, want %d", len(servers), len(want))
	}
}

func TestCheckRedirect(t *testing.T) {
	req := func(u string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	ok := []string{"https://riseup.net/ca.crt", "https://black.riseup.net/ca.crt", "https://api.black.riseup.net/3/cert"}
	for _, u := range ok {
		if err := checkRedirect(req(u), nil); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	bad := []string{
		"http://black.riseup.net/ca.crt",
		"https://evil.example/ca.crt",
		"https://riseup.net.evil.example/ca.crt",
		"https://evilriseup.net/ca.crt",
		"ftp://black.riseup.net/ca.crt",
	}
	for _, u := range bad {
		if err := checkRedirect(req(u), nil); err == nil {
			t.Errorf("%s: expected a refusal", u)
		}
	}
	// Too many redirects, even to riseup.net.
	via := make([]*http.Request, maxRedirects)
	if err := checkRedirect(req("https://black.riseup.net/ca.crt"), via); err == nil {
		t.Error("redirect loop: expected a refusal")
	}
}

// A redirect to a host outside riseup.net is not followed.
func TestGetRefusesRedirectAway(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "secret")
	}))
	t.Cleanup(target.Close)
	mux := http.NewServeMux()
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	if _, err := get(context.Background(), srv.URL+"/redirect", maxBody); err == nil {
		t.Error("redirect away from riseup.net: expected an error")
	}
}
