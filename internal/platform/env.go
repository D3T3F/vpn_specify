package platform

import (
	"os"
	"regexp"
	"strings"
)

var winEnvRe = regexp.MustCompile(`%([^%]+)%`)

// ExpandEnv expands Windows-style variables (%LocalAppData%).
func ExpandEnv(s string) string {
	return winEnvRe.ReplaceAllStringFunc(s, func(m string) string { return os.Getenv(strings.Trim(m, "%")) })
}
