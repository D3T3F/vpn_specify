//go:build windows

package i18n

import "golang.org/x/sys/windows"

const muiLanguageName = 0x8 // MUI_LANGUAGE_NAME: "pt-BR" instead of an LCID

// systemLocales returns the user's preferred Windows display languages.
func systemLocales() []string {
	langs, err := windows.GetUserPreferredUILanguages(muiLanguageName)
	if err != nil {
		return nil
	}
	return langs
}
