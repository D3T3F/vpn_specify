//go:build !windows

package i18n

import (
	"os"
	"strings"
)

// systemLocales follows the POSIX precedence: LANGUAGE (a ":" list), then
// LC_ALL, LC_MESSAGES and LANG.
func systemLocales() []string {
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
