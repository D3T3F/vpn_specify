// Package i18n translates the user-facing text of vpn_specify (GUI, installer,
// CLI messages and errors).
//
// Messages are written in English directly in the code, gettext-style: the
// English text is the key. Each locale in locales/<code>.json maps English
// text to its translation; English needs no file. Placeholders are {0}, {1},
// ... so translators can reorder them. The web pages use the same catalogs
// (elements marked with data-i18n, and t("...") in JavaScript).
//
// TestCatalogs extracts every message from the Go and HTML sources and fails
// if a locale is missing one or has mismatched placeholders.
//
// The language is the one saved by the user (installer or Preferences), else
// the system language when supported, else English.
package i18n

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"vpn_specify/internal/paths"
)

// Language is a supported UI language.
type Language struct {
	Code    string `json:"code"`    // BCP 47, e.g. "pt-BR"
	Name    string `json:"name"`    // native name, shown in the language picker
	English string `json:"english"` // English name
	RTL     bool   `json:"rtl"`     // written right to left
}

// Languages lists the supported languages; English is the default.
var Languages = []Language{
	{Code: "en", Name: "English", English: "English"},
	{Code: "pt-BR", Name: "Português (Brasil)", English: "Portuguese (Brazil)"},
	{Code: "es", Name: "Español", English: "Spanish"},
	{Code: "fr", Name: "Français", English: "French"},
	{Code: "de", Name: "Deutsch", English: "German"},
	{Code: "ru", Name: "Русский", English: "Russian"},
	{Code: "zh-CN", Name: "简体中文", English: "Chinese (Simplified)"},
	{Code: "ja", Name: "日本語", English: "Japanese"},
	{Code: "hi", Name: "हिन्दी", English: "Hindi"},
	{Code: "ar", Name: "العربية", English: "Arabic", RTL: true},
}

const Default = "en"

//go:embed locales/*.json
var localeFS embed.FS

var (
	mu       sync.RWMutex
	current  = Default
	catalogs = map[string]map[string]string{}
)

// Catalog returns the translations of one language (English → translation).
// English has an empty catalog: the key is the text.
func Catalog(code string) map[string]string {
	mu.Lock()
	defer mu.Unlock()
	return catalogLocked(code)
}

func catalogLocked(code string) map[string]string {
	if c, ok := catalogs[code]; ok {
		return c
	}
	c := map[string]string{}
	if code != Default {
		if b, err := localeFS.ReadFile("locales/" + code + ".json"); err == nil {
			_ = json.Unmarshal(b, &c)
		}
	}
	catalogs[code] = c
	return c
}

// Supported returns the language with this code, or false.
func Supported(code string) (Language, bool) {
	for _, l := range Languages {
		if strings.EqualFold(l.Code, code) {
			return l, true
		}
	}
	return Language{}, false
}

// Current returns the active language code.
func Current() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Set changes the active language (unsupported codes fall back to English).
func Set(code string) {
	l, ok := Supported(code)
	if !ok {
		l, _ = Supported(Default)
	}
	mu.Lock()
	current = l.Code
	catalogLocked(l.Code)
	mu.Unlock()
}

// T translates msg to the active language and fills in {0}, {1}, ... with args.
func T(msg string, args ...any) string {
	mu.RLock()
	s, ok := catalogs[current][msg]
	mu.RUnlock()
	if !ok || s == "" {
		s = msg
	}
	return format(s, args...)
}

func format(s string, args ...any) string {
	if len(args) == 0 {
		return s
	}
	pairs := make([]string, 0, 2*len(args))
	for i, a := range args {
		pairs = append(pairs, "{"+strconv.Itoa(i)+"}", fmt.Sprint(a))
	}
	return strings.NewReplacer(pairs...).Replace(s)
}

// Error returns a translated error.
func Error(msg string, args ...any) error { return errors.New(T(msg, args...)) }

// Wrap returns a translated error that also wraps err ("<message>: <err>").
func Wrap(err error, msg string, args ...any) error {
	return &wrapped{msg: T(msg, args...), err: err}
}

type wrapped struct {
	msg string
	err error
}

func (w *wrapped) Error() string { return w.msg + ": " + w.err.Error() }
func (w *wrapped) Unwrap() error { return w.err }

// Match maps a locale name ("pt_BR.UTF-8", "zh-Hans-CN", "ES") to a
// supported language code, or "".
func Match(locale string) string {
	locale = strings.TrimSpace(locale)
	if i := strings.IndexAny(locale, ".@"); i >= 0 {
		locale = locale[:i]
	}
	locale = strings.ReplaceAll(locale, "_", "-")
	if locale == "" || locale == "C" || locale == "POSIX" {
		return ""
	}
	if l, ok := Supported(locale); ok {
		return l.Code
	}
	base, _, _ := strings.Cut(strings.ToLower(locale), "-")
	switch base {
	case "pt":
		return "pt-BR"
	case "zh":
		return "zh-CN"
	}
	if l, ok := Supported(base); ok {
		return l.Code
	}
	return ""
}

// Detect returns the first supported system language, or English.
func Detect() string {
	for _, l := range systemLocales() {
		if c := Match(l); c != "" {
			return c
		}
	}
	return Default
}

// prefFile keeps the user's choice, next to the other app data.
func prefFile() string { return filepath.Join(paths.DataDir(), "language") }

// Saved returns the language the user chose, or "".
func Saved() string {
	b, err := os.ReadFile(prefFile())
	if err != nil {
		return ""
	}
	if l, ok := Supported(strings.TrimSpace(string(b))); ok {
		return l.Code
	}
	return ""
}

// Save stores the user's choice and makes it active.
func Save(code string) error {
	l, ok := Supported(code)
	if !ok {
		return fmt.Errorf("unsupported language %q", code)
	}
	if err := os.MkdirAll(filepath.Dir(prefFile()), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(prefFile(), []byte(l.Code+"\n"), 0o600); err != nil {
		return err
	}
	Set(l.Code)
	return nil
}

// Init activates the saved language, or the system one.
func Init() {
	if c := Saved(); c != "" {
		Set(c)
		return
	}
	Set(Detect())
}

// N marks a message for translation without translating it (for texts kept
// in tables and translated later with T). It returns s unchanged.
func N(s string) string { return s }

// Sentinel returns an error to keep in a package variable: it is compared
// with errors.Is and translated each time it is shown, in the language
// active at that moment (package variables are created before the language
// is loaded).
func Sentinel(msg string) error { return sentinel(msg) }

type sentinel string

func (s sentinel) Error() string { return T(string(s)) }
