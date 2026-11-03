//go:build !windows && !linux

package update

import "vpn_specify/internal/i18n"

func Prepare(archive, ver string, relaunch bool) (*Plan, error) {
	return nil, i18n.Error("automatic updates are not available on this system; download from {0}", ReleasesPage)
}
