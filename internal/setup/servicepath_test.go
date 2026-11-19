//go:build windows

package setup

import "testing"

func TestServiceBinaryMatches(t *testing.T) {
	dst := `C:\Program Files\vpn_specify\windivert\WinDivert64.sys`
	for _, binary := range []string{
		`\??\C:\Program Files\vpn_specify\windivert\WinDivert64.sys`,
		`\??\c:\program files\VPN_SPECIFY\windivert\windivert64.sys`,
		`"C:\Program Files\vpn_specify\windivert\WinDivert64.sys"`,
		` C:\Program Files\vpn_specify\windivert\WinDivert64.sys `,
	} {
		if !serviceBinaryMatches(binary, dst) {
			t.Errorf("serviceBinaryMatches(%q) = false; want true", binary)
		}
	}
	for _, binary := range []string{
		``,
		`\??\C:\Users\David\vpn_specify\windivert\WinDivert64.sys`,
		`\??\C:\Tools\WinDivert\WinDivert64.sys`,
		`\??\C:\Program Files\vpn_specify\windivert\WinDivert64.sys.old`,
		`C:\Program Files\vpn_specify\windivert\WinDivert.dll`,
	} {
		if serviceBinaryMatches(binary, dst) {
			t.Errorf("serviceBinaryMatches(%q) = true; want false", binary)
		}
	}
}
