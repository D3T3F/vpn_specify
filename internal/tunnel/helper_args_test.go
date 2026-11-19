package tunnel

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestParseDarwinHelperArgs(t *testing.T) {
	// absolute paths on any system (macOS paths are not absolute on Windows)
	root := t.TempDir()
	p := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	openvpn := p("opt", "homebrew", "sbin", "openvpn")
	ok := []string{p("stop"), "4242", p("log"), openvpn,
		"--cd", p("vpn"), "--config", p("config.ovpn"), "--route-nopull"}
	h, err := parseDarwinHelperArgs(ok)
	if err != nil {
		t.Fatal(err)
	}
	if h.stopFile != p("stop") || h.owner != 4242 || h.logFile != p("log") || h.openvpn != openvpn ||
		h.configFile != p("config.ovpn") || len(h.args) != 5 {
		t.Errorf("parsed %+v", h)
	}

	bad := map[string][]string{
		"too few":            {p("stop"), "4242", p("log")},
		"relative stop":      {"stop", "4242", p("log"), openvpn},
		"pid not a number":   {p("stop"), "abc", p("log"), openvpn},
		"pid init":           {p("stop"), "1", p("log"), openvpn},
		"pid negative":       {p("stop"), "-5", p("log"), openvpn},
		"relative log":       {p("stop"), "4242", "log", openvpn},
		"relative openvpn":   {p("stop"), "4242", p("log"), "openvpn"},
		"other program":      {p("stop"), "4242", p("log"), p("bin", "sh")},
		"other name":         {p("stop"), "4242", p("log"), p("tmp", "evil", "openvpn-x")},
		"relative config":    {p("stop"), "4242", p("log"), openvpn, "--config", "config.ovpn"},
		"config missing":     {p("stop"), "4242", p("log"), openvpn, "--config"},
		"relative cd":        {p("stop"), "4242", p("log"), openvpn, "--cd", "vpn"},
		"relative auth file": {p("stop"), "4242", p("log"), openvpn, "--auth-user-pass", "auth.txt"},
	}
	for name, args := range bad {
		if _, err := parseDarwinHelperArgs(args); err == nil {
			t.Errorf("%s: should be refused: %q", name, args)
		}
	}
}

// fakeOwners is a fileOwnerStat with fixed answers (missing paths are errors).
type fakeOwners map[string]struct {
	uid  uint32
	perm fs.FileMode
}

func (f fakeOwners) OwnerAndMode(path string) (uint32, fs.FileMode, error) {
	e, ok := f[path]
	if !ok {
		return 0, 0, errors.New("no such file")
	}
	return e.uid, e.perm, nil
}

func TestCheckOpenVPNBinary(t *testing.T) {
	const user = 501
	fs := fakeOwners{
		"/usr/local/Cellar/openvpn/bin/openvpn": {0, 0o755},
		"/usr/local/bin/group-writable":         {0, 0o775},
		"/usr/local/bin/world-writable":         {0, 0o777},
		"/Users/x/bin/openvpn":                  {user, 0o755},
		"/opt/homebrew/sbin/openvpn":            {user, 0o755}, // Homebrew: owned by the user
		"/opt/homebrew/bin/other-openvpn":       {user, 0o755}, // not a Homebrew candidate
	}
	cases := map[string]bool{ // path -> accepted
		"/usr/local/Cellar/openvpn/bin/openvpn": true,
		"/usr/local/bin/group-writable":         false,
		"/usr/local/bin/world-writable":         false,
		"/Users/x/bin/openvpn":                  false,
		"/opt/homebrew/sbin/openvpn":            true,
		"/opt/homebrew/bin/other-openvpn":       false,
		"/usr/local/bin/missing":                false,
	}
	for path, want := range cases {
		err := checkOpenVPNBinary(path, fs)
		if (err == nil) != want {
			t.Errorf("checkOpenVPNBinary(%s) = %v; accepted want %v", path, err, want)
		}
	}
}
