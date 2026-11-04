//go:build !windows

package ovpninstall

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"vpn_specify/internal/i18n"
)

func source() string { return i18n.T("your distribution's packages") }

// packageManagers: the command that installs the openvpn package with each
// manager, without questions.
var packageManagers = []struct{ tool, script string }{
	{"apt-get", "DEBIAN_FRONTEND=noninteractive apt-get update -q; DEBIAN_FRONTEND=noninteractive apt-get install -y openvpn"},
	{"dnf", "dnf install -y openvpn"},
	{"yum", "yum install -y openvpn"},
	{"zypper", "zypper --non-interactive install openvpn"},
	{"pacman", "pacman -S --noconfirm --needed openvpn"},
	{"apk", "apk add openvpn"},
}

// Install installs the openvpn package. pkexec shows the system's own
// administrator password dialog; running as root needs no prompt.
func Install(ctx context.Context, progress func(string)) error {
	script := ""
	for _, m := range packageManagers {
		if _, err := exec.LookPath(m.tool); err == nil {
			script = m.script
			break
		}
	}
	if script == "" {
		return i18n.Error("no supported package manager found; install the openvpn package manually")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	progress(i18n.T("Installing OpenVPN…"))
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", script)
	} else {
		pk, err := exec.LookPath("pkexec")
		if err != nil {
			return i18n.Error("installing the openvpn package failed: {0}", "pkexec not found; run: sudo "+script)
		}
		cmd = exec.CommandContext(ctx, pk, "/bin/sh", "-c", script)
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	var ee *exec.ExitError
	if os.Geteuid() != 0 && errors.As(err, &ee) && (ee.ExitCode() == 126 || ee.ExitCode() == 127) {
		return i18n.Error("administrator permission denied; OpenVPN was not installed")
	}
	msg := strings.TrimSpace(string(out))
	if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
		msg = msg[i+1:] // the last line says why
	}
	if msg == "" {
		msg = err.Error()
	}
	return i18n.Error("installing the openvpn package failed: {0}", msg)
}
