package platform

import (
	"os"
	"regexp"
	"strings"
)

// ExpandEnv expande variáveis no formato do Windows (%LocalAppData%).
var winEnvRe = regexp.MustCompile(`%([^%]+)%`)

func ExpandEnv(s string) string {
	return winEnvRe.ReplaceAllStringFunc(s, func(m string) string { return os.Getenv(strings.Trim(m, "%")) })
}
