package i18n

import "strings"

// parseAppleLanguages reads the output of "defaults read -g AppleLanguages",
// a property list array, and returns the language names in order:
//
//	(
//	    "pt-BR",
//	    "en-US",
//	    "zh-Hans-CN"
//	)
//
// Parentheses, line breaks and quotes are ignored; an empty array gives nil.
func parseAppleLanguages(out string) []string {
	s := strings.NewReplacer("(", " ", ")", " ", "\r", " ", "\n", " ").Replace(out)
	var langs []string
	for _, item := range strings.Split(s, ",") {
		if name := strings.Trim(strings.TrimSpace(item), `"`); name != "" {
			langs = append(langs, name)
		}
	}
	return langs
}
