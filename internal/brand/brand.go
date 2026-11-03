// Package brand holds the vpn_specify logo, used by the app and installer
// pages.
package brand

import (
	_ "embed"
	"encoding/base64"
)

//go:embed logo.png
var logoPNG []byte

// LogoDataURI returns the logo as a data URI (for <img src> and favicons).
func LogoDataURI() string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(logoPNG)
}

// LogoPNG returns the logo as PNG (icon of the .desktop shortcuts on Linux).
func LogoPNG() []byte { return logoPNG }
