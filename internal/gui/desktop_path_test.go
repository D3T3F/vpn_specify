package gui

import "testing"

func TestIsAppBundle(t *testing.T) {
	cases := map[string]bool{
		"/Applications/Foo.app":                    true,
		"/Applications/Foo.app/":                   true,
		"/Applications/Foo.app//":                  true,
		"/usr/bin/foo":                             false,
		"/Applications/app.app.txt":                false,
		"/Applications/Foo.app/Contents/MacOS/Foo": false,
	}
	for p, want := range cases {
		if got := isAppBundle(p); got != want {
			t.Errorf("isAppBundle(%q) = %v; want %v", p, got, want)
		}
	}
}

func TestCheckOpenTarget(t *testing.T) {
	for _, ok := range []string{"http://127.0.0.1:1/?t=x", "/Users/me/Library/Application Support/vpn_specify"} {
		if err := checkOpenTarget(ok); err != nil {
			t.Errorf("checkOpenTarget(%q) = %v; want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "-a", "--background", "-"} {
		if err := checkOpenTarget(bad); err == nil {
			t.Errorf("checkOpenTarget(%q) accepted a target that open could read as an option", bad)
		}
	}
}
