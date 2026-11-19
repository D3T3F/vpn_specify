package ovpninstall

import "testing"

func TestFindOnMacOS(t *testing.T) {
	if p, ok := FindOnMacOS(func(string) bool { return false }); ok || p != "" {
		t.Errorf("nothing installed: got %q, %v", p, ok)
	}
	// Apple silicon Homebrew wins over the Intel one when both exist
	both := map[string]bool{"/opt/homebrew/sbin/openvpn": true, "/usr/local/sbin/openvpn": true}
	if p, ok := FindOnMacOS(func(s string) bool { return both[s] }); !ok || p != "/opt/homebrew/sbin/openvpn" {
		t.Errorf("got %q, %v", p, ok)
	}
	only := map[string]bool{"/usr/local/bin/openvpn": true}
	if p, ok := FindOnMacOS(func(s string) bool { return only[s] }); !ok || p != "/usr/local/bin/openvpn" {
		t.Errorf("got %q, %v", p, ok)
	}
}

func TestIsMacOSCandidate(t *testing.T) {
	if !IsMacOSCandidate("/opt/homebrew/sbin/openvpn") || IsMacOSCandidate("/opt/homebrew/sbin/openvpn-x") || IsMacOSCandidate("/tmp/openvpn") {
		t.Error("candidate list check is wrong")
	}
}
