//go:build !linux

package tunnel

import "errors"

// RunOpenVPNHelper is only used on Linux (see helper_linux.go).
func RunOpenVPNHelper(args []string) error {
	return errors.New("openvpn-helper only exists on Linux")
}
