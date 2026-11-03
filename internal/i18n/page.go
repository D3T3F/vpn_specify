package i18n

import (
	"encoding/json"
	"strings"
)

// PageState is what a web page needs to show one language.
type PageState struct {
	Lang    string            `json:"lang"`
	RTL     bool              `json:"rtl"`
	Catalog map[string]string `json:"catalog"`
}

// State returns the PageState of the active language.
func State() PageState {
	code := Current()
	l, _ := Supported(code)
	return PageState{Lang: code, RTL: l.RTL, Catalog: Catalog(code)}
}

// pageScript is the translation runtime of the web pages (GUI and
// installer). It defines:
//   - t(msg, ...args): translates an English message, filling {0}, {1}, ...;
//   - applyI18n(): translates elements marked with data-i18n (text) and
//     data-i18n-attr="placeholder,title" (attributes), keeping the English
//     source so the language can change again;
//   - setI18n(state): switches language (state comes from vsSetLanguage);
//   - I18N_LANG, I18N_LANGUAGES: active code and the supported languages.
const pageScript = `
var I18N_LANGUAGES = %LANGS%;
var I18N_LANG = "en", I18N_RTL = false, I18N_CATALOG = {};
function t(msg) {
  var s = I18N_CATALOG[msg] || msg;
  for (var i = 1; i < arguments.length; i++) s = s.split("{" + (i - 1) + "}").join(String(arguments[i]));
  return s;
}
function applyI18n() {
  document.querySelectorAll("[data-i18n]").forEach(function (el) {
    if (el.dataset.i18nSrc === undefined) el.dataset.i18nSrc = el.textContent.trim().replace(/\s+/g, " ");
    el.textContent = t(el.dataset.i18nSrc);
  });
  document.querySelectorAll("[data-i18n-attr]").forEach(function (el) {
    el.dataset.i18nAttr.split(",").forEach(function (a) {
      a = a.trim();
      var key = "i18nSrc_" + a;
      if (el.dataset[key] === undefined) el.dataset[key] = el.getAttribute(a) || "";
      el.setAttribute(a, t(el.dataset[key]));
    });
  });
  document.documentElement.lang = I18N_LANG;
  document.documentElement.dir = I18N_RTL ? "rtl" : "ltr";
  if (document.title) {
    if (document.documentElement.dataset.i18nTitle === undefined) document.documentElement.dataset.i18nTitle = document.title;
    document.title = t(document.documentElement.dataset.i18nTitle);
  }
}
function setI18n(st) {
  I18N_LANG = st.lang; I18N_RTL = st.rtl; I18N_CATALOG = st.catalog || {};
  if (document.readyState !== "loading") applyI18n();
}
setI18n(%STATE%);
document.addEventListener("DOMContentLoaded", applyI18n);
`

// PageScript returns the translation runtime with the active language
// preloaded (to put inside a <script> in the page head).
func PageScript() string {
	langs, _ := json.Marshal(Languages) // json escapes <, > and & (safe inside <script>)
	state, _ := json.Marshal(State())
	return strings.NewReplacer("%LANGS%", string(langs), "%STATE%", string(state)).Replace(pageScript)
}
