package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Sources scanned for messages (relative to this package).
var sourceRoots = []string{"../../cmd", "../../internal", "../../android/app/src/main/kotlin"}

// goString matches a Go string literal: "interpreted" or `raw`.
const goString = `"(?:[^"\\]|\\.)*"|` + "`[^`]*`"

var (
	goCall   = regexp.MustCompile(`i18n\.(?:T|Error|N|Sentinel)\(\s*(` + goString + `)`)
	goWrap   = regexp.MustCompile(`i18n\.Wrap\([^,]+,\s*(` + goString + `)`)
	ktCall   = regexp.MustCompile(`\bT\(\s*("(?:[^"\\]|\\.)*")`)
	jsCall   = regexp.MustCompile(`\bt\(\s*("(?:[^"\\]|\\.)*")`)
	htmlText = regexp.MustCompile(`(?s)<(\w+)((?:[^>]*\s)?data-i18n(?:\s[^>]*)?)>(.*?)</(\w+)>`)
	htmlAttr = regexp.MustCompile(`(?s)<\w+([^>]*\bdata-i18n-attr="([^"]+)"[^>]*)>`)
	attrVal  = func(name string) *regexp.Regexp { return regexp.MustCompile(`\b` + name + `="([^"]*)"`) }
	spaces   = regexp.MustCompile(`\s+`)
	holder   = regexp.MustCompile(`\{\d\}`)
)

// normalize matches what the page does: trims and collapses whitespace.
func normalize(s string) string { return strings.TrimSpace(spaces.ReplaceAllString(s, " ")) }

func unescapeHTML(s string) string {
	return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(s)
}

// messages extracts every translatable message from the sources.
func messages(t *testing.T) map[string][]string {
	t.Helper()
	found := map[string][]string{} // message -> where
	add := func(msg, where string) {
		if msg != "" {
			found[msg] = append(found[msg], where)
		}
	}
	for _, root := range sourceRoots {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			isGo := strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go")
			isHTML := strings.HasSuffix(p, ".html")
			isKotlin := strings.HasSuffix(p, ".kt")
			if !isGo && !isHTML && !isKotlin {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			src := string(b)
			where := filepath.ToSlash(strings.TrimPrefix(p, "../../"))
			if isKotlin {
				for _, m := range ktCall.FindAllStringSubmatch(src, -1) {
					s, err := strconv.Unquote(m[1])
					if err != nil {
						t.Errorf("%s: %v in %s", where, err, m[1])
					}
					add(s, where)
				}
				return nil
			}
			if isGo {
				for _, re := range []*regexp.Regexp{goCall, goWrap} {
					for _, m := range re.FindAllStringSubmatch(src, -1) {
						s, err := strconv.Unquote(strings.ReplaceAll(m[1], "\r", "")) // the compiler drops \r from raw strings
						if err != nil {
							t.Errorf("%s: %v in %s", where, err, m[1])
						}
						add(s, where)
					}
				}
				return nil
			}
			for _, m := range htmlText.FindAllStringSubmatch(src, -1) {
				if m[1] != m[4] || strings.Contains(m[3], "<") {
					t.Errorf("%s: data-i18n element must contain plain text only: %.80s", where, m[0])
					continue
				}
				add(normalize(unescapeHTML(m[3])), where)
			}
			for _, m := range htmlAttr.FindAllStringSubmatch(src, -1) {
				for _, a := range strings.Split(m[2], ",") {
					if v := attrVal(strings.TrimSpace(a)).FindStringSubmatch(m[1]); v != nil {
						add(normalize(unescapeHTML(v[1])), where)
					}
				}
			}
			for _, m := range jsCall.FindAllStringSubmatch(src, -1) {
				s, err := strconv.Unquote(m[1])
				if err != nil {
					t.Errorf("%s: %v in %s", where, err, m[1])
				}
				add(s, where)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return found
}

func placeholders(s string) string {
	h := holder.FindAllString(s, -1)
	sort.Strings(h)
	return strings.Join(h, "")
}

// TestCatalogs: every message has a translation in every language, with the
// same placeholders, and no catalog keeps messages the code no longer uses.
//
// I18N_DUMP=<file> writes, for each language, the messages it is missing
// (handy when adding text or a language).
func TestCatalogs(t *testing.T) {
	msgs := messages(t)
	if len(msgs) < 50 {
		t.Fatalf("only %d messages found; is the extraction broken?", len(msgs))
	}
	missingAll := map[string][]string{}
	for _, l := range Languages {
		if l.Code == Default {
			continue
		}
		b, err := localeFS.ReadFile("locales/" + l.Code + ".json")
		if err != nil {
			t.Errorf("%s: no catalog: %v", l.Code, err)
			continue
		}
		var c map[string]string
		if err := json.Unmarshal(b, &c); err != nil {
			t.Errorf("%s: invalid JSON: %v", l.Code, err)
			continue
		}
		var missing []string
		for m := range msgs {
			tr, ok := c[m]
			switch {
			case !ok || strings.TrimSpace(tr) == "":
				missing = append(missing, m)
			case placeholders(tr) != placeholders(m):
				t.Errorf("%s: placeholders differ\n  en: %q\n  %s: %q", l.Code, m, l.Code, tr)
			}
		}
		for k := range c {
			if _, ok := msgs[k]; !ok {
				t.Errorf("%s: unused message (remove it): %q", l.Code, k)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			missingAll[l.Code] = missing
			t.Errorf("%s: %d messages without translation, e.g. %q", l.Code, len(missing), missing[0])
		}
	}
	if p := os.Getenv("I18N_DUMP"); p != "" {
		var all []string
		for m := range msgs {
			all = append(all, m)
		}
		sort.Strings(all)
		b, _ := json.MarshalIndent(map[string]any{"all": all, "missing": missingAll}, "", "  ")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMatch(t *testing.T) {
	for in, want := range map[string]string{
		"pt_BR.UTF-8": "pt-BR", "pt-PT": "pt-BR", "es_MX": "es", "zh-Hans-CN": "zh-CN", "zh_TW": "zh-CN",
		"ja-JP": "ja", "RU": "ru", "ar_EG.utf8": "ar", "en-US": "en", "C": "", "nl-NL": "", "": "",
	} {
		if got := Match(in); got != want {
			t.Errorf("Match(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestFormatAndFallback(t *testing.T) {
	Set("xx") // unsupported -> English
	if Current() != "en" || T("Hello {0}, {1}!", "Ana", 3) != "Hello Ana, 3!" {
		t.Fatalf("got %q in %q", T("Hello {0}, {1}!", "Ana", 3), Current())
	}
	Set("pt-BR")
	defer Set("en")
	if T("a message nobody translated {0}", 1) != "a message nobody translated 1" {
		t.Error("untranslated text must fall back to English")
	}
}
