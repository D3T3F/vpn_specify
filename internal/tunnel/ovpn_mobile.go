package tunnel

import (
	"strings"

	"vpn_specify/internal/i18n"
)

// OpenVPN on Android. There the system decides which apps use the tunnel, so
// unlike the desktop versions the routes and DNS from the server are honored.
// What has to go is whatever would make openvpn run something or touch files:
// a .ovpn is often downloaded from somewhere, and it runs as this app.

// ovpnMobileStrip lists the directives removed from the configuration.
var ovpnMobileStrip = map[string]bool{
	// run programs
	"up": true, "down": true, "up-delay": true, "up-restart": true, "down-pre": true,
	"route-up": true, "route-pre-down": true, "ipchange": true, "tls-verify": true,
	"auth-user-pass-verify": true, "client-connect": true, "client-disconnect": true,
	"client-crresponse": true, "learn-address": true, "script-security": true,
	"plugin": true, "dns-updown": true, "iproute": true, "tls-export-cert": true,
	// process and files
	"daemon": true, "log": true, "log-append": true, "status": true, "status-version": true,
	"writepid": true, "cd": true, "chroot": true, "user": true, "group": true,
	"config": true, "askpass": true, "tmp-dir": true, "mlock": true, "dev-node": true,
	"replay-persist": true, "ifconfig-pool-persist": true, "syslog": true, "machine-readable-output": true,
	// the management interface belongs to the app
	"management": true, "management-client": true, "management-hold": true,
	"management-query-passwords": true, "management-up-down": true,
	"management-log-cache": true, "management-signal": true, "management-forget-disconnect": true,
	"management-external-key": true, "management-external-cert": true,
	"management-client-auth": true, "management-client-user": true, "management-client-group": true,
	"management-query-proxy": true, "management-query-remote": true,
}

// ovpnFileDirectives must come inline (<ca>...</ca>): the file they would name
// is not on the phone. The value is the position of the file name, and
// [inline] is what a line refers to an inline block with.
var ovpnFileDirectives = map[string]bool{
	"ca": true, "cert": true, "key": true, "extra-certs": true, "dh": true,
	"pkcs12": true, "tls-auth": true, "tls-crypt": true, "tls-crypt-v2": true,
	"crl-verify": true, "secret": true, "http-proxy-user-pass": true,
}

// OVPNPlan is an OpenVPN configuration made ready for Android.
type OVPNPlan struct {
	Config    string `json:"config"`
	NeedsAuth bool   `json:"needsAuth"` // asks for a user and password
}

// PlanOpenVPN validates a .ovpn and removes what must not run. Errors name
// what the user has to change, in the current language.
func PlanOpenVPN(data []byte) (*OVPNPlan, error) {
	var b strings.Builder
	plan := &OVPNPlan{}
	hasRemote := false
	inBlock := ""
	inline := map[string]bool{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		l := strings.TrimSpace(line)
		if inBlock != "" {
			if strings.EqualFold(l, "</"+inBlock+">") {
				inBlock = ""
			}
			b.WriteString(line + "\n")
			continue
		}
		if strings.HasPrefix(l, "<") && strings.HasSuffix(l, ">") && !strings.HasPrefix(l, "</") {
			inBlock = strings.ToLower(strings.Trim(l, "<>"))
			inline[inBlock] = true
			if inBlock == "connection" {
				hasRemote = true // holds its own remote
			}
			b.WriteString(line + "\n")
			continue
		}
		f := strings.Fields(l)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") || strings.HasPrefix(f[0], ";") {
			b.WriteString(line + "\n")
			continue
		}
		d := strings.ToLower(strings.TrimPrefix(f[0], "--"))
		switch {
		case ovpnMobileStrip[d]:
			b.WriteString("# [removed by vpn_specify] " + d + "\n")
			continue
		case d == "remote":
			hasRemote = true
		case d == "dev" && len(f) >= 2 && strings.HasPrefix(strings.ToLower(f[1]), "tap"),
			d == "dev-type" && len(f) >= 2 && strings.EqualFold(f[1], "tap"):
			return nil, i18n.Error("this VPN uses a TAP device, which Android does not support")
		case d == "auth-user-pass":
			if len(f) >= 2 {
				return nil, i18n.Error("the configuration refers to the file {0} ({1}): put its content inside the .ovpn", f[1], d)
			}
			plan.NeedsAuth = true
		case ovpnFileDirectives[d]:
			if len(f) >= 2 && f[1] != "[inline]" {
				return nil, i18n.Error("the configuration refers to the file {0} ({1}): put its content inside the .ovpn", f[1], d)
			}
		}
		b.WriteString(line + "\n")
	}
	if inline["auth-user-pass"] {
		plan.NeedsAuth = false
	}
	if !hasRemote {
		return nil, i18n.Error("no server (remote) in the configuration")
	}
	plan.Config = b.String()
	return plan, nil
}
