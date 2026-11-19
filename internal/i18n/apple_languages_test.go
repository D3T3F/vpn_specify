package i18n

import (
	"reflect"
	"testing"
)

func TestParseAppleLanguages(t *testing.T) {
	cases := map[string][]string{
		"(\n    \"pt-BR\",\n    \"en-US\",\n    \"zh-Hans-CN\"\n)\n": {"pt-BR", "en-US", "zh-Hans-CN"},
		"(\r\n    \"en-GB\"\r\n)\r\n":                                {"en-GB"},
		"(\n    en-US,\n    fr-FR\n)\n":                              {"en-US", "fr-FR"},
		"(\n)\n":                                                     nil,
		"":                                                           nil,
		"(\"de-DE\", \"es-ES\")":                                     {"de-DE", "es-ES"},
	}
	for in, want := range cases {
		if got := parseAppleLanguages(in); !reflect.DeepEqual(got, want) {
			t.Errorf("parseAppleLanguages(%q) = %q; want %q", in, got, want)
		}
	}
	// the names feed Match, which maps them to the catalogs
	if got := Match(parseAppleLanguages("(\n \"zh-Hans-CN\"\n)")[0]); got != "zh-CN" {
		t.Errorf("zh-Hans-CN matched %q", got)
	}
}
