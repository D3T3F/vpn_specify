//go:build !linux

package tunnel

import "errors"

// RunOpenVPNHelper só é usado no Linux (veja helper_linux.go).
func RunOpenVPNHelper(args []string) error {
	return errors.New("openvpn-helper só existe no Linux")
}
