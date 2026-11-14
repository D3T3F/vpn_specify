// Package riseup lists the OpenVPN gateways of RiseupVPN, the VPN of Riseup
// Networks, and builds the OpenVPN configuration of each one.
//
// The gateway list is public. The client certificate is not: Config downloads a
// new one each time a server is added. It is short lived, so a VPN added from
// RiseUp must be added again when its certificate expires.
//
// Only the values checked here (IP, port, protocol) and the PEM blocks, decoded
// and encoded again, go into the configuration. Nothing else from the network is
// copied into it, so a server cannot make openvpn run anything.
package riseup

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/latency"
	"vpn_specify/internal/version"
)

// The endpoints of the RiseupVPN client. Tests point them at a local server.
var (
	gatewaysURL = "https://api.black.riseup.net/3/config/eip-service.json"
	certURL     = "https://api.black.riseup.net/3/cert"
	caURL       = "https://black.riseup.net/ca.crt"
	locateURL   = "https://api.black.riseup.net:9001/json"
)

const (
	requestTimeout = 20 * time.Second
	maxBody        = 1 << 20
)

// Server is one connection the user can add.
type Server struct {
	ID      string `json:"id"`
	Name    string `json:"name"` // name of the VPN once added
	Host    string `json:"host"`
	IP      string `json:"ip"`
	Country string `json:"country"` // ISO code, e.g. US
	City    string `json:"city"`    // name of the location, e.g. Miami
	Proto   string `json:"proto"`   // udp | tcp
	Port    int    `json:"port"`
}

// List is the result of a search.
type List struct {
	Servers []Server `json:"servers"`
}

// wanted are the connections each gateway is offered as: OpenVPN over UDP on
// port 1194 and over TCP on port 80.
var wanted = []struct {
	proto string
	port  int
}{{"udp", 1194}, {"tcp", 80}}

var (
	hostName    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)
	countryCode = regexp.MustCompile(`^[A-Z]{2}$`)
)

type transport struct {
	Type      string   `json:"type"`
	Protocols []string `json:"protocols"`
	Ports     []string `json:"ports"`
}

type gateway struct {
	Host         string `json:"host"`
	IPAddress    string `json:"ip_address"`
	Location     string `json:"location"`
	Capabilities struct {
		Limited   bool        `json:"limited"`
		Transport []transport `json:"transport"`
	} `json:"capabilities"`
}

type location struct {
	CountryCode string `json:"country_code"`
	Name        string `json:"name"`
}

// Parse reads the gateway list of the API. Gateways it cannot trust, limited
// ones, and transports other than OpenVPN are left out.
func Parse(data []byte) ([]Server, error) {
	var doc struct {
		Gateways  []gateway           `json:"gateways"`
		Locations map[string]location `json:"locations"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, i18n.Error("RiseUp sent a list this version cannot read")
	}
	servers := []Server{}
	seen := map[string]bool{}
	for _, g := range doc.Gateways {
		if g.Capabilities.Limited || !hostName.MatchString(g.Host) {
			continue
		}
		addr := net.ParseIP(g.IPAddress)
		if addr == nil {
			continue
		}
		loc := doc.Locations[g.Location]
		code := strings.ToUpper(loc.CountryCode)
		if !countryCode.MatchString(code) {
			code = ""
		}
		city := cityName(loc.Name)
		if city == "" {
			city = cityName(g.Location)
		}
		label := strings.Split(g.Host, ".")[0]
		for _, tr := range g.Capabilities.Transport {
			if tr.Type != "openvpn" {
				continue
			}
			for _, w := range wanted {
				if !slices.Contains(tr.Protocols, w.proto) || !slices.Contains(tr.Ports, strconv.Itoa(w.port)) {
					continue
				}
				id := fmt.Sprintf("%s-%s-%d", g.Host, w.proto, w.port)
				if seen[id] {
					continue
				}
				seen[id] = true
				servers = append(servers, Server{
					ID:      id,
					Name:    fmt.Sprintf("RiseUp %s %s %s", city, label, strings.ToUpper(w.proto)),
					Host:    g.Host,
					IP:      addr.String(),
					Country: code,
					City:    city,
					Proto:   w.proto,
					Port:    w.port,
				})
			}
		}
	}
	// Two gateways of one city can share the first label of their host. Their
	// names then use the full host, which is unique for each gateway and protocol.
	count := map[string]int{}
	for _, s := range servers {
		count[s.Name]++
	}
	for i := range servers {
		if count[servers[i].Name] > 1 {
			servers[i].Name = fmt.Sprintf("RiseUp %s %s %s", servers[i].City, servers[i].Host, strings.ToUpper(servers[i].Proto))
		}
	}
	sort.SliceStable(servers, func(i, j int) bool {
		a, b := servers[i], servers[j]
		if a.Country != b.Country {
			return a.Country < b.Country
		}
		if a.Host != b.Host {
			return a.Host < b.Host
		}
		return a.Proto < b.Proto
	})
	return servers, nil
}

// cityName cleans a location name sent by the network before it is shown or put
// in a server name: control and format characters (bidi overrides included) are
// dropped, runs of spaces become one space, and the name is cut to 40 runes.
func cityName(s string) string {
	var b strings.Builder
	pending := false // a space is owed before the next letter
	n := 0
	for _, r := range s {
		if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		if unicode.IsSpace(r) {
			pending = b.Len() > 0
			continue
		}
		if pending {
			b.WriteByte(' ')
			pending = false
		}
		if n == 40 {
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// find returns the server of the list with the id.
func (l *List) find(id string) (*Server, error) {
	if l != nil {
		for i := range l.Servers {
			if l.Servers[i].ID == id {
				return &l.Servers[i], nil
			}
		}
	}
	return nil, i18n.Error("this server is no longer on the list; search again")
}

// Config returns the server of the list and its OpenVPN configuration. It
// downloads a new client certificate and the CA now, so call it when the user
// adds the server, not when the list is shown.
func (l *List) Config(ctx context.Context, id string) (*Server, []byte, error) {
	if _, err := l.find(id); err != nil {
		return nil, nil, err
	}
	cert, err := download(ctx, certURL)
	if err != nil {
		return nil, nil, err
	}
	ca, err := download(ctx, caURL)
	if err != nil {
		return nil, nil, err
	}
	return l.ConfigFromData(id, cert, ca)
}

// ConfigFromData returns the server of the list and its OpenVPN configuration
// from data already downloaded: certPEM holds the client key and certificate,
// caPEM the CA. It does not use the network, and checks the data as Config does.
func (l *List) ConfigFromData(id string, certPEM, caPEM []byte) (*Server, []byte, error) {
	s, err := l.find(id)
	if err != nil {
		return nil, nil, err
	}
	if _, err := endpoint(s); err != nil {
		return nil, nil, err
	}
	key, cert, err := parseClientCredentials(certPEM)
	if err != nil {
		return nil, nil, err
	}
	ca, err := parseAuthority(caPEM)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := configOf(s, ca, cert, key)
	if err != nil {
		return nil, nil, err
	}
	return s, cfg, nil
}

// options are the lines every configuration has, after the remote line.
const options = "auth SHA512\n" +
	"data-ciphers AES-256-GCM\n" +
	"float\n" +
	"keepalive 10 30\n" +
	"nobind\n" +
	"persist-key\n" +
	"rcvbuf 0\n" +
	"sndbuf 0\n" +
	"verb 3\n" +
	"remote-cert-tls server\n" +
	"resolv-retry infinite\n"

// endpoint checks the fields that go into the configuration and returns the IP
// in its canonical form.
func endpoint(s *Server) (string, error) {
	addr := net.ParseIP(s.IP)
	if addr == nil || s.Port < 1 || s.Port > 65535 || (s.Proto != "udp" && s.Proto != "tcp") {
		return "", i18n.Error("RiseUp sent a list this version cannot read")
	}
	return addr.String(), nil
}

func configOf(s *Server, ca, cert, key []byte) ([]byte, error) {
	ip, err := endpoint(s)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "client\ndev tun\nproto %s\nremote %s %d\n", s.Proto, ip, s.Port)
	b.WriteString(options)
	b.WriteString("<ca>\n")
	b.Write(ca)
	b.WriteString("</ca>\n<cert>\n")
	b.Write(cert)
	b.WriteString("</cert>\n<key>\n")
	b.Write(key)
	b.WriteString("</key>\n")
	return b.Bytes(), nil
}

// keyTypes are the private key blocks the client key may have: PKCS#1 RSA,
// SEC 1 EC, and PKCS#8 of any algorithm.
var keyTypes = []string{"RSA PRIVATE KEY", "EC PRIVATE KEY", "PRIVATE KEY"}

// parseClientCredentials reads the client key and certificate. They must be one
// private key and one certificate that match each other.
func parseClientCredentials(data []byte) (key, cert []byte, err error) {
	bad := i18n.Error("RiseUp sent a certificate this version cannot read")
	blocks, ok := decodePEM(data, append([]string{"CERTIFICATE"}, keyTypes...)...)
	if !ok {
		return nil, nil, bad
	}
	var keys, certs []pemBlock
	for _, b := range blocks {
		if slices.Contains(keyTypes, b.typ) {
			keys = append(keys, b)
		} else {
			certs = append(certs, b)
		}
	}
	if len(keys) != 1 || len(certs) != 1 {
		return nil, nil, bad
	}
	key, cert = keys[0].encode(), certs[0].encode()
	if _, err := tls.X509KeyPair(cert, key); err != nil {
		return nil, nil, bad
	}
	return key, cert, nil
}

// parseAuthority reads the CA certificates of the servers.
func parseAuthority(data []byte) ([]byte, error) {
	bad := i18n.Error("RiseUp sent a CA certificate this version cannot read")
	blocks, ok := decodePEM(data, "CERTIFICATE")
	if !ok || len(blocks) == 0 {
		return nil, bad
	}
	var out []byte
	for _, b := range blocks {
		if _, err := x509.ParseCertificate(b.der); err != nil {
			return nil, bad
		}
		out = append(out, b.encode()...)
	}
	return out, nil
}

// pemBlock is one decoded PEM block.
type pemBlock struct {
	typ string
	der []byte
}

// encode writes the block again, so no text from the network reaches the
// configuration.
func (b pemBlock) encode() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: b.typ, Bytes: b.der})
}

// decodePEM returns the blocks of data. It fails if a block has headers (an
// encrypted key has them) or a type that is not allowed. Text outside the blocks
// is ignored: the blocks are encoded again, so it does not reach the result.
func decodePEM(data []byte, allowed ...string) ([]pemBlock, bool) {
	var out []pemBlock
	for {
		var b *pem.Block
		b, data = pem.Decode(data)
		if b == nil {
			return out, true
		}
		if len(b.Headers) > 0 || !slices.Contains(allowed, b.Type) {
			return nil, false
		}
		out = append(out, pemBlock{typ: b.Type, der: b.Bytes})
	}
}

// client is used only for the RiseUp endpoints. It follows redirects only to
// https URLs on riseup.net.
var client = &http.Client{Timeout: requestTimeout, CheckRedirect: checkRedirect}

const maxRedirects = 5

func checkRedirect(req *http.Request, via []*http.Request) error {
	host := req.URL.Hostname()
	if len(via) >= maxRedirects || req.URL.Scheme != "https" || (host != "riseup.net" && !strings.HasSuffix(host, ".riseup.net")) {
		return i18n.Error("RiseUp redirected to a place this version does not trust")
	}
	return nil
}

// download gets one of the files the configuration needs.
func download(ctx context.Context, url string) ([]byte, error) {
	data, err := get(ctx, url, maxBody)
	if err != nil {
		return nil, i18n.Error("could not reach RiseUp: {0}", err)
	}
	return data, nil
}

func get(ctx context.Context, url string, limit int64) ([]byte, error) {
	return request(ctx, http.MethodGet, url, client, limit)
}

// request makes one call to a RiseUp endpoint with the client c and returns the
// answer, of at most limit bytes.
func request(ctx context.Context, method, url string, c *http.Client, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "vpn_specify/"+version.Version)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, i18n.Error("RiseUp answered {0}", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, i18n.Error("RiseUp sent more data than this version accepts")
	}
	return data, nil
}

// Build makes the list from the gateway list of the API, already downloaded. It
// needs no network.
func Build(eipJSON []byte) (*List, error) {
	servers, err := Parse(eipJSON)
	if err != nil {
		return nil, err
	}
	return &List{Servers: servers}, nil
}

// Fetch downloads the gateway list. It requests no certificate: that happens in
// Config, when the user adds a server.
func Fetch(ctx context.Context) (*List, error) {
	data, err := download(ctx, gatewaysURL)
	if err != nil {
		return nil, err
	}
	return Build(data)
}

// Locate returns the names of the gateways, nearest to the user first. The
// service works out the user's place from the connection, and nothing from it is
// logged. Its certificate is signed by the RiseUp CA, not by a system root, so
// that CA is the only root this request trusts.
func Locate(ctx context.Context) ([]string, error) {
	data, err := download(ctx, caURL)
	if err != nil {
		return nil, err
	}
	ca, err := parseAuthority(data)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	defer tr.CloseIdleConnections()
	tr.TLSClientConfig = &tls.Config{RootCAs: pool}
	c := &http.Client{Timeout: requestTimeout, CheckRedirect: checkRedirect, Transport: tr}
	body, err := request(ctx, http.MethodPost, locateURL, c, maxBody)
	if err != nil {
		return nil, i18n.Error("could not reach RiseUp: {0}", err)
	}
	var doc struct {
		Gateways []string `json:"gateways"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, i18n.Error("RiseUp sent a list this version cannot read")
	}
	var hosts []string
	seen := map[string]bool{}
	for _, h := range doc.Gateways {
		if hostName.MatchString(h) && !seen[h] {
			seen[h] = true
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return nil, i18n.Error("RiseUp has no server available right now.")
	}
	return hosts, nil
}

// Targets returns up to limit gateways to measure, in the order of near (the
// names from Locate). The gateways near does not name go last, in the order of
// the list. Both protocols of a gateway make one target, with the address and the
// TCP ports of its servers. A gateway without a TCP server is left out, since
// only TCP can be measured.
func (l *List) Targets(near []string, limit int) []latency.Target {
	if l == nil {
		return nil
	}
	rank := map[string]int{}
	for i, h := range near {
		if _, ok := rank[h]; !ok {
			rank[h] = i
		}
	}
	at := func(h string) int {
		if i, ok := rank[h]; ok {
			return i
		}
		return len(near)
	}
	var targets []latency.Target
	index := map[string]int{}
	for _, s := range l.Servers {
		if s.Proto != "tcp" {
			continue
		}
		i, ok := index[s.Host]
		if !ok {
			i = len(targets)
			index[s.Host] = i
			targets = append(targets, latency.Target{ID: s.Host, IP: s.IP})
		}
		targets[i].Ports = append(targets[i].Ports, s.Port)
	}
	sort.SliceStable(targets, func(i, j int) bool { return at(targets[i].ID) < at(targets[j].ID) })
	if len(targets) > limit {
		targets = targets[:limit]
	}
	return targets
}
