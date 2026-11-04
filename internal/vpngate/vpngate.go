// Package vpngate lists the public servers of VPN Gate (https://www.vpngate.net),
// an academic project of the University of Tsukuba, closest country first.
//
// The API is one CSV file that already holds the OpenVPN configuration of each
// server. Those files come from the internet and, on the desktop, openvpn runs
// as Administrator or root, so a configuration that holds anything beyond the
// plain client options VPN Gate really uses is left out of the list (see vet).
package vpngate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/version"
)

const (
	apiURL  = "https://www.vpngate.net/api/iphone/"
	homeURL = "https://www.vpngate.net/en/"

	maxList = 32 << 20
	maxHome = 4 << 20
)

// Server is one connection the user can add.
type Server struct {
	ID          string `json:"id"`
	Name        string `json:"name"` // name of the VPN once added
	Host        string `json:"host"`
	IP          string `json:"ip"`
	Country     string `json:"country"`     // ISO code, e.g. BR
	CountryName string `json:"countryName"` // in English, as VPN Gate writes it
	Proto       string `json:"proto"`       // tcp | udp
	Port        int    `json:"port"`
	Speed       int64  `json:"speed"` // bits per second
	Sessions    int    `json:"sessions"`
	Score       int64  `json:"score"`

	config []byte
}

// List is the result of a search.
type List struct {
	Servers []Server `json:"servers"`
	Near    string   `json:"near"`    // ISO code of the user's country, or ""
	Skipped int      `json:"skipped"` // servers left out by vet
}

// Config returns the OpenVPN configuration of a server of the list.
func (l *List) Config(id string) (*Server, []byte, error) {
	if l != nil {
		for i := range l.Servers {
			if l.Servers[i].ID == id {
				return &l.Servers[i], l.Servers[i].config, nil
			}
		}
	}
	return nil, nil, i18n.Error("this server is no longer on the list; search again")
}

var hostName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

// Parse reads the CSV of the API. The rows are "*vpn_servers", a "#HostName"
// header, the servers, and "*". Rows it cannot trust are counted, not fatal.
func Parse(data []byte) (servers []Server, skipped int, err error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	sawHeader := false
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				skipped++
				continue
			}
			return nil, 0, err
		}
		if len(row) == 0 || strings.HasPrefix(row[0], "*") {
			continue
		}
		if strings.HasPrefix(row[0], "#") {
			sawHeader = strings.Contains(row[0], "HostName")
			continue
		}
		s, ok := parseRow(row)
		if !ok {
			skipped++
			continue
		}
		servers = append(servers, s)
	}
	if !sawHeader {
		return nil, 0, i18n.Error("VPN Gate sent a list this version cannot read")
	}
	return servers, skipped, nil
}

func parseRow(f []string) (Server, bool) {
	const (
		host, ip, score                  = 0, 1, 2
		speed, countryLong, countryShort = 4, 5, 6
		sessions                         = 7
		configB64                        = 14
		columns                          = 15
	)
	if len(f) < columns || !hostName.MatchString(f[host]) {
		return Server{}, false
	}
	addr := net.ParseIP(f[ip])
	code := strings.ToUpper(f[countryShort])
	if addr == nil || len(code) != 2 {
		return Server{}, false
	}
	cfg, err := base64.StdEncoding.DecodeString(strings.TrimSpace(f[configB64]))
	if err != nil {
		return Server{}, false
	}
	proto, port, ok := vet(cfg, f[ip])
	if !ok {
		return Server{}, false
	}
	if proto == "udp" {
		// On some of these servers, big packets get lost in the UDP tunnel
		// (1000 bytes passed, 1200 did not). Modern TLS greetings are that
		// big, so sites would fail in the apps while the VPN shows connected.
		cfg = append(cfg, []byte("\n# added by vpn_specify: keeps TCP packets small enough\nmssfix 1200\n")...)
	}
	num := func(s string) int64 {
		n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return n
	}
	return Server{
		ID:          f[host],
		Name:        fmt.Sprintf("VPN Gate %s %s %s", code, f[host], strings.ToUpper(proto)),
		Host:        f[host],
		IP:          f[ip],
		Country:     code,
		CountryName: f[countryLong],
		Proto:       proto,
		Port:        port,
		Speed:       num(f[speed]),
		Sessions:    int(num(f[sessions])),
		Score:       num(f[score]),
		config:      cfg,
	}, true
}

// Only plain client options. The list is an allow list on purpose: a directive
// that is not here (up, script-security, plugin, config, log...) drops the server.
var (
	allowedOptions = map[string]bool{
		"auth": true, "auth-nocache": true, "cipher": true, "client": true, "data-ciphers": true,
		"dev": true, "float": true, "keepalive": true, "mssfix": true, "mute": true, "nobind": true,
		"persist-key": true, "persist-tun": true, "proto": true, "pull": true, "rcvbuf": true,
		"remote": true, "remote-cert-tls": true, "resolv-retry": true, "sndbuf": true,
		"tls-client": true, "tun-mtu": true, "verb": true,
	}
	allowedBlocks = map[string]bool{"ca": true, "cert": true, "key": true, "tls-auth": true, "tls-crypt": true}
)

// vet checks a configuration and returns its protocol and port. It must be a
// tun client with one remote, the address the list shows for this server.
func vet(cfg []byte, ip string) (proto string, port int, ok bool) {
	remotes := 0
	block := ""
	for _, raw := range strings.Split(string(cfg), "\n") {
		l := strings.TrimSpace(raw)
		if block != "" {
			if strings.EqualFold(l, "</"+block+">") {
				block = ""
			}
			continue
		}
		if l == "" || l[0] == '#' || l[0] == ';' {
			continue
		}
		if strings.HasPrefix(l, "<") {
			block = strings.ToLower(strings.Trim(l, "<>"))
			if !allowedBlocks[block] {
				return "", 0, false
			}
			continue
		}
		f := strings.Fields(l)
		d := strings.ToLower(strings.TrimPrefix(f[0], "--"))
		if !allowedOptions[d] {
			return "", 0, false
		}
		switch d {
		case "dev":
			if len(f) != 2 || !strings.HasPrefix(strings.ToLower(f[1]), "tun") {
				return "", 0, false
			}
		case "proto":
			if len(f) < 2 {
				return "", 0, false
			}
			switch p := strings.ToLower(f[1]); {
			case strings.HasPrefix(p, "tcp"):
				proto = "tcp"
			case strings.HasPrefix(p, "udp"):
				proto = "udp"
			default:
				return "", 0, false
			}
		case "remote":
			remotes++
			if len(f) < 3 || f[1] != ip {
				return "", 0, false
			}
			n, err := strconv.Atoi(f[2])
			if err != nil || n < 1 || n > 65535 {
				return "", 0, false
			}
			port = n
		}
	}
	if block != "" || remotes != 1 || proto == "" {
		return "", 0, false
	}
	return proto, port, true
}

// ---- closest first ----------------------------------------------------------

var yourCountry = regexp.MustCompile(`(?i)Your country:\s*(?:<[^>]*>\s*)*([^<\r\n]+)`)

// Names VPN Gate writes differently from the country table.
var countryAliases = map[string]string{
	"korea republic of":  "KR",
	"russian federation": "RU",
	"viet nam":           "VN",
	"iran":               "IR",
	"taiwan":             "TW",
	"united states":      "US",
	"united kingdom":     "GB",
	"czechia":            "CZ",
	"tanzania":           "TZ",
	"moldova":            "MD",
}

// CountryOfHome finds the visitor's country in the home page of VPN Gate. This
// is the one place the API does not cover, and it avoids sending the user's
// address to one more service. Returns "" when the page changes.
func CountryOfHome(page []byte) string {
	m := yourCountry.FindSubmatch(page)
	if m == nil {
		return ""
	}
	return countryByName(string(m[1]))
}

func countryByName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if c, ok := countryAliases[name]; ok {
		return c
	}
	for code, c := range countries {
		if strings.ToLower(c.name) == name {
			return code
		}
	}
	return ""
}

var localeRegion = regexp.MustCompile(`^[A-Za-z]{2,3}[-_](?:[A-Za-z]{4}[-_])?([A-Za-z]{2})(?:[^A-Za-z]|$)`)

// RegionOfLocales returns the country of the first locale that names one
// ("pt-BR", "pt_BR.UTF-8"), for when the home page does not answer.
func RegionOfLocales(locales []string) string {
	for _, l := range locales {
		if m := localeRegion.FindStringSubmatch(l); m != nil {
			code := strings.ToUpper(m[1])
			if _, ok := countries[code]; ok {
				return code
			}
		}
	}
	return ""
}

func distanceKm(a, b country) float64 {
	const earthKm = 6371
	rad := math.Pi / 180
	dLat, dLon := (b.lat-a.lat)*rad, (b.lon-a.lon)*rad
	h := math.Pow(math.Sin(dLat/2), 2) + math.Cos(a.lat*rad)*math.Cos(b.lat*rad)*math.Pow(math.Sin(dLon/2), 2)
	return 2 * earthKm * math.Asin(math.Sqrt(h))
}

// Order sorts by distance from the country near (the same country first, then
// by distance between country centers), and by score inside a country. With an
// unknown country it is only by score.
func Order(servers []Server, near string) {
	from, known := countries[near]
	if !known {
		sort.SliceStable(servers, func(i, j int) bool { return servers[i].Score > servers[j].Score })
		return
	}
	km := func(code string) float64 {
		if code == near {
			return -1 // also when a table center is shared
		}
		to, ok := countries[code]
		if !ok {
			return math.MaxFloat64 // an unknown country goes last
		}
		return distanceKm(from, to)
	}
	sort.SliceStable(servers, func(i, j int) bool {
		a, b := km(servers[i].Country), km(servers[j].Country)
		if a != b {
			return a < b
		}
		if servers[i].Country != servers[j].Country {
			return servers[i].Country < servers[j].Country // keeps a country together
		}
		return servers[i].Score > servers[j].Score
	})
}

// Build makes the list from the CSV and, if available, the home page. The
// locales are the fallback for the user's country.
func Build(csvData, home []byte, locales []string) (*List, error) {
	servers, skipped, err := Parse(csvData)
	if err != nil {
		return nil, err
	}
	near := CountryOfHome(home)
	if near == "" {
		near = RegionOfLocales(locales)
	}
	Order(servers, near)
	if servers == nil {
		servers = []Server{} // the page expects a list
	}
	return &List{Servers: servers, Near: near, Skipped: skipped}, nil
}

// ---- download (desktop; on Android the app downloads and calls Build) -------

func get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "vpn_specify/"+version.Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, i18n.Error("VPN Gate answered {0}", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// Fetch downloads the list. Only the list itself is required: without the home
// page the user's country comes from the system language.
func Fetch(ctx context.Context, locales []string) (*List, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	var home []byte
	wg.Add(1)
	go func() {
		defer wg.Done()
		c, stop := context.WithTimeout(ctx, 30*time.Second)
		defer stop()
		home, _ = get(c, homeURL, maxHome)
	}()
	data, err := get(ctx, apiURL, maxList)
	wg.Wait()
	if err != nil {
		return nil, i18n.Error("could not reach VPN Gate: {0}", err)
	}
	return Build(data, home, locales)
}
