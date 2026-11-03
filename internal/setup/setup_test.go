//go:build windows

package setup

import "testing"

func TestNormalizeDir(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Program Files\vpn_specify`:      `C:\Program Files\vpn_specify`,
		`C:\Program Files\VPN_SPECIFY\`:     `C:\Program Files\VPN_SPECIFY`,
		`D:\Apps`:                           `D:\Apps\vpn_specify`,
		` "D:\My Apps\" `:                   `D:\My Apps\vpn_specify`,
		`E:\`:                               `E:\vpn_specify`,
		`C:\Program Files\x\..\vpn_specify`: `C:\Program Files\vpn_specify`,
	} {
		got, err := NormalizeDir(in)
		if err != nil || got != want {
			t.Errorf("NormalizeDir(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", `relative\folder`, `\no\drive`} {
		if _, err := NormalizeDir(bad); err == nil {
			t.Errorf("NormalizeDir(%q) should fail", bad)
		}
	}
}
