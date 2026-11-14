package riseup

import (
	"strings"
	"testing"
)

func TestCityNameSanitized(t *testing.T) {
	for in, want := range map[string]string{
		"Miami":                     "Miami",
		"‮Miami‬":                   "Miami",
		"Miami\x00\x07":             "Miami",
		"  Miami \t  Los\n Angeles": "Miami Los Angeles",
		"":                          "",
		"   ":                       "",
		strings.Repeat("a", 50):     strings.Repeat("a", 40),
		strings.Repeat("é", 45):     strings.Repeat("é", 40),
	} {
		if got := cityName(in); got != want {
			t.Errorf("cityName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSanitizesCity(t *testing.T) {
	servers, err := Parse([]byte(`{
		"gateways": [
			{"host": "vpn1-sao.riseup.net", "ip_address": "10.0.0.1", "location": "sao",
			 "capabilities": {"transport": [{"type": "openvpn", "protocols": ["udp"], "ports": ["1194"]}]}}
		],
		"locations": {"sao": {"country_code": "BR", "name": "‮Sao\u0000  Paulo‬"}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].City != "Sao Paulo" || servers[0].Name != "RiseUp Sao Paulo vpn1-sao UDP" {
		t.Fatalf("got %+v", servers)
	}
}
