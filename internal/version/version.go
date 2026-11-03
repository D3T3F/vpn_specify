// Package version holds the program version, set at build time:
//
//	go build -ldflags "-X vpn_specify/internal/version.Version=1.2.3"
package version

// Version is "dev" in builds without a version.
var Version = "dev"
