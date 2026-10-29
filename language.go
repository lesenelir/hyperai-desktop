package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// languages are the languages HyperAI is written in, its own first, as the
// catalogs in locales/ have them: each tag, and its name in itself, as the
// General page's menu lists them, and as macOS's do ("Français", where
// CLDR's autonym is lower case).
var languages = []struct{ tag, name string }{
	{"en", "English"},
	{"zh-Hans", "简体中文"},
	{"ja", "日本語"},
	{"ko", "한국어"},
	{"fr", "Français"},
	{"de", "Deutsch"},
}

// languageName returns the name of the language of tag in itself, System
// for "", which follows the system's languages.
func languageName(tag string) string {
	for _, l := range languages {
		if l.tag == tag {
			return l.name
		}
	}
	return locales.LanguageSystem()
}

// setLanguage shows the app in the language of tag, "" for the system's,
// as it does from then on: applyLanguage keeps it for the next launches
// too (mygo.App.SetLocale), and layout.json keeps what the menu shows
// chosen.
func (a *app) setLanguage(tag string) {
	if tag == a.language {
		return
	}
	a.language = tag
	if a.applyLanguage != nil {
		a.applyLanguage(tag)
	}
	a.saveLayout()
}

// languageButton builds the pop-up button that chooses the app's language,
// as the fonts' do: System, which follows the system's languages, a
// separator, then each language in itself, the one chosen checked.
func (a *app) languageButton(c *ui.Context) {
	titles := func() []string {
		out := []string{locales.LanguageSystem()}
		for _, l := range languages {
			out = append(out, l.name)
		}
		return out
	}
	a.popUpButton(c, locales.Language(), languageName(a.language), titles, func(m *ui.Menu) {
		if m.Item(locales.LanguageSystem()).Checked(a.language == "").Chosen() {
			a.setLanguage("")
		}
		m.Separator()
		for _, l := range languages {
			if m.Item(l.name).Checked(a.language == l.tag).Chosen() {
				a.setLanguage(l.tag)
			}
		}
	})
}

// platform names this platform as the catalogs' messages choose by it:
// "mac" on macOS, "other" elsewhere.
func platform() string { return onMac("mac", "other") }

// terms returns the words of a catalog's list of them, as the settings'
// field finds a row by: separated by commas, or by the ideographic ones
// Chinese and Japanese write.
func terms(list string) []string {
	var out []string
	for _, t := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == '，' || r == '、' }) {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}
