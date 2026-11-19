//go:build !linux && !darwin

package tunnel

import "errors"

// RunOpenVPNHelper is only used on Linux and macOS (see helper_linux.go and
// helper_darwin.go).
func RunOpenVPNHelper(args []string) error {
	return errors.New("openvpn-helper only exists on Linux")
}
