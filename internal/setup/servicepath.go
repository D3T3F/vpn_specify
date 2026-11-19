package setup

import (
	"path/filepath"
	"strings"
)

// serviceBinaryMatches reports whether the image path of a service
// configuration (BinaryPathName, for example \??\C:\...\WinDivert64.sys or a
// quoted path) is the file at dst. The comparison ignores case.
func serviceBinaryMatches(binaryPath, dst string) bool {
	p := strings.TrimSpace(binaryPath)
	p = strings.TrimPrefix(p, `\??\`)
	p = strings.Trim(p, `"`)
	if p == "" {
		return false
	}
	want, err := filepath.Abs(dst)
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(p), filepath.Clean(want))
}
