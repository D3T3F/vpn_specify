package textutil

import (
	"strings"
)

// SplitList separa uma lista por vírgulas, ignorando itens vazios.
func SplitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
