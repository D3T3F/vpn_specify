package ovpninstall

// macOSOpenVPNPaths are where Homebrew installs openvpn (Apple silicon and
// Intel). The sbin folder is listed first because the formula puts it there.
var macOSOpenVPNPaths = []string{
	"/opt/homebrew/sbin/openvpn",
	"/opt/homebrew/bin/openvpn",
	"/usr/local/sbin/openvpn",
	"/usr/local/bin/openvpn",
}

// FindOnMacOS returns the first macOS candidate for which exists reports true.
// exists is a parameter so that the choice can be tested anywhere.
func FindOnMacOS(exists func(path string) bool) (string, bool) {
	for _, p := range macOSOpenVPNPaths {
		if exists(p) {
			return p, true
		}
	}
	return "", false
}

// IsMacOSCandidate reports whether path is exactly one of the macOS candidates.
func IsMacOSCandidate(path string) bool {
	for _, p := range macOSOpenVPNPaths {
		if p == path {
			return true
		}
	}
	return false
}
