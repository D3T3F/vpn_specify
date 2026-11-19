package tunnel

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/ovpninstall"
)

// darwinHelper is what "vpn_specify openvpn-helper" receives on macOS:
//
//	openvpn-helper <stop file> <owner pid> <log file> <openvpn> <openvpn arguments...>
type darwinHelper struct {
	stopFile   string   // created by the owner to ask openvpn to stop
	owner      int      // pid of the unprivileged vpn_specify that started it
	logFile    string   // where openvpn's output goes (the owner reads it)
	openvpn    string   // absolute path of the openvpn program
	args       []string // openvpn's arguments
	configFile string   // the --config path ("" if none)
}

// parseDarwinHelperArgs checks the helper's arguments. The helper runs as root,
// so every path must be absolute, the program must be an openvpn, and the
// paths that openvpn reads (--config, --cd, --auth-user-pass) must be absolute.
func parseDarwinHelperArgs(args []string) (darwinHelper, error) {
	var h darwinHelper
	if len(args) < 4 {
		return h, errors.New("usage: vpn_specify openvpn-helper <stop file> <owner pid> <log file> <openvpn> [args...]")
	}
	if !filepath.IsAbs(args[0]) {
		return h, fmt.Errorf("openvpn-helper: the stop file must be an absolute path, not %q", args[0])
	}
	pid, err := strconv.Atoi(args[1])
	if err != nil || pid < 2 {
		return h, fmt.Errorf("openvpn-helper: invalid owner pid %q", args[1])
	}
	if !filepath.IsAbs(args[2]) {
		return h, fmt.Errorf("openvpn-helper: the log file must be an absolute path, not %q", args[2])
	}
	if !filepath.IsAbs(args[3]) || filepath.Base(args[3]) != "openvpn" {
		return h, fmt.Errorf("openvpn-helper only runs an absolute path to openvpn, not %q", args[3])
	}
	h = darwinHelper{stopFile: args[0], owner: pid, logFile: args[2], openvpn: args[3], args: args[4:]}
	for i := 0; i < len(h.args); i++ {
		switch h.args[i] {
		case "--config", "--cd", "--auth-user-pass":
			if i+1 >= len(h.args) || !filepath.IsAbs(h.args[i+1]) {
				return h, fmt.Errorf("openvpn-helper: %s needs an absolute path", h.args[i])
			}
			if h.args[i] == "--config" {
				h.configFile = h.args[i+1]
			}
			i++
		}
	}
	return h, nil
}

// fileOwnerStat reports who owns a file (following links) and its permission bits.
type fileOwnerStat interface {
	OwnerAndMode(path string) (uid uint32, perm fs.FileMode, err error)
}

// checkOpenVPNBinary decides whether the root helper may run the program at
// path: either one of the Homebrew locations (which the owner's Homebrew
// installs), or a program owned by root that no one but root can write.
func checkOpenVPNBinary(path string, st fileOwnerStat) error {
	if ovpninstall.IsMacOSCandidate(path) {
		return nil
	}
	uid, perm, err := st.OwnerAndMode(path)
	if err != nil || uid != 0 || perm&0o022 != 0 {
		return i18n.Error("openvpn-helper refuses {0}: it must be owned by root and not writable by others, or be the Homebrew openvpn", path)
	}
	return nil
}
