//go:build !windows

package i18n

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// systemLocales follows the POSIX precedence: LANGUAGE (a ":" list), then
// LC_ALL, LC_MESSAGES and LANG. On macOS the preferred languages of the
// system settings come first (the environment is often just "C" there).
func systemLocales() []string {
	if runtime.GOOS == "darwin" {
		if langs := macOSLocales(); len(langs) > 0 {
			return langs
		}
	}
	var out []string
	if l := os.Getenv("LANGUAGE"); l != "" {
		out = append(out, strings.Split(l, ":")...)
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// macOSLocales returns the languages from the global defaults, in the user's
// order; nil when they cannot be read.
func macOSLocales() []string {
	out, err := exec.Command("/usr/bin/defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return nil
	}
	return parseAppleLanguages(string(out))
}
