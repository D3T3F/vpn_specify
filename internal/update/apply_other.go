//go:build !windows && !linux

package update

import "errors"

func Prepare(archive, ver string, relaunch bool) (*Plan, error) {
	return nil, errors.New("atualização automática não disponível neste sistema; baixe em " + ReleasesPage)
}
