package gui

import (
	"errors"
	"strings"
)

// Pure helpers used by the macOS build (desktop_darwin.go). They have no build
// tag, so their tests run on every system.

// isAppBundle reports whether a path chosen in the file picker is an
// application bundle. The picker returns bundles with a trailing slash.
func isAppBundle(p string) bool {
	return strings.HasSuffix(strings.TrimRight(p, "/"), ".app")
}

// checkOpenTarget refuses a target that "open" could read as an option (or
// that is empty, which would open the default folder).
func checkOpenTarget(target string) error {
	if target == "" || strings.HasPrefix(target, "-") {
		return errors.New("refusing to open a target that is empty or starts with '-'")
	}
	return nil
}
