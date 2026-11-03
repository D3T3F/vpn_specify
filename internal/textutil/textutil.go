package textutil

import (
	"strings"
)

// SplitList splits a comma-separated list, skipping empty items.
func SplitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
