package riseup

import (
	"context"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"vpn_specify/internal/latency"
)

// locateServer is a TLS server that answers the gateway call of Locate with the
// given body, and returns its CA as PEM.
func locateServer(t *testing.T, body string) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/json" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
}

func useLocateURL(t *testing.T, url string) {
	t.Helper()
	old := locateURL
	locateURL = url
	t.Cleanup(func() { locateURL = old })
}

func TestLocate(t *testing.T) {
	srv, ca := locateServer(t, `{"ip":"192.0.2.1","cc":"BR","city":"Sao Paulo","lat":-23.5,"lon":-46.6,
		"gateways":["vpn1-sao.riseup.net","bad host","vpn1-sao.riseup.net","vpn2-mia.riseup.net"]}`)
	serve(t, &fake{ca: ca})
	useLocateURL(t, srv.URL+"/json")

	got, err := Locate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"vpn1-sao.riseup.net", "vpn2-mia.riseup.net"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLocateRejectsOtherCA(t *testing.T) {
	srv, _ := locateServer(t, `{"gateways":["vpn1-sao.riseup.net"]}`)
	// httptest servers share one certificate, so the other CA is a new one.
	_, otherCA := selfSigned(t)
	serve(t, &fake{ca: otherCA})
	useLocateURL(t, srv.URL+"/json")

	if _, err := Locate(context.Background()); err == nil {
		t.Fatal("Locate accepted a certificate that the RiseUp CA did not sign")
	}
}

func TestLocateNoGateways(t *testing.T) {
	srv, ca := locateServer(t, `{"gateways":["bad host"]}`)
	serve(t, &fake{ca: ca})
	useLocateURL(t, srv.URL+"/json")

	if _, err := Locate(context.Background()); err == nil {
		t.Fatal("Locate returned no error for a list without valid gateways")
	}
}

func TestTargets(t *testing.T) {
	l := &List{Servers: []Server{
		{ID: "a.riseup.net-tcp-80", Host: "a.riseup.net", IP: "10.0.0.1", Proto: "tcp", Port: 80},
		{ID: "a.riseup.net-udp-1194", Host: "a.riseup.net", IP: "10.0.0.1", Proto: "udp", Port: 1194},
		{ID: "b.riseup.net-tcp-80", Host: "b.riseup.net", IP: "10.0.0.2", Proto: "tcp", Port: 80},
		{ID: "b.riseup.net-udp-1194", Host: "b.riseup.net", IP: "10.0.0.2", Proto: "udp", Port: 1194},
		{ID: "c.riseup.net-udp-1194", Host: "c.riseup.net", IP: "10.0.0.3", Proto: "udp", Port: 1194},
		{ID: "d.riseup.net-tcp-80", Host: "d.riseup.net", IP: "10.0.0.4", Proto: "tcp", Port: 80},
	}}

	got := l.Targets([]string{"d.riseup.net", "x.riseup.net", "b.riseup.net", "c.riseup.net"}, 12)
	want := []latency.Target{
		{ID: "d.riseup.net", IP: "10.0.0.4", Ports: []int{80}},
		{ID: "b.riseup.net", IP: "10.0.0.2", Ports: []int{80}},
		{ID: "a.riseup.net", IP: "10.0.0.1", Ports: []int{80}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	got = l.Targets(nil, 1)
	want = []latency.Target{{ID: "a.riseup.net", IP: "10.0.0.1", Ports: []int{80}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
