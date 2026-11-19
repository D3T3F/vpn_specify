package update

import "testing"

func TestAssetNamePerSystem(t *testing.T) {
	for _, c := range []struct{ goos, goarch, want string }{
		{"windows", "amd64", "vpn_specify-1.2.3-win64.zip"},
		{"linux", "amd64", "vpn_specify-1.2.3-linux-amd64.tar.gz"},
		{"linux", "arm64", "vpn_specify-1.2.3-linux-arm64.tar.gz"},
		{"darwin", "amd64", "vpn_specify-1.2.3-macos-amd64.tar.gz"},
		{"darwin", "arm64", "vpn_specify-1.2.3-macos-arm64.tar.gz"},
		{"plan9", "amd64", ""},
	} {
		if got := assetName(c.goos, c.goarch, "1.2.3"); got != c.want {
			t.Errorf("assetName(%s, %s) = %q; want %q", c.goos, c.goarch, got, c.want)
		}
	}
}
