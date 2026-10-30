package main

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// settingsSection is a page of the settings, under its group in the
// sidebar: about says what it holds, under its title, and page builds what
// it holds, nil until it holds something; terms are the names of what it
// holds, which the sidebar's field finds it by too, nil for none.
type settingsSection struct {
	key, group, title string
	icon              *ui.SVG
	about             string
	page              func(a *app, c *ui.Context)
	terms             func(a *app) []settingsTerm
}

// settingsTerm is a thing a settings page holds, as the sidebar's field
// finds it: by its title, which the sidebar then shows under the page, or
// by more of what it is.
type settingsTerm struct {
	title string
	more  []string
}

// matches reports whether the term's title or more holds q, lowered.
func (t settingsTerm) matches(q []rune) bool {
	if holds(t.title, q) {
		return true
	}
	for _, m := range t.more {
		if holds(m, q) {
			return true
		}
	}
	return false
}

// holds reports whether s holds q, lowered, in any case.
func holds(s string, q []rune) bool { return find(lowerRunes([]rune(s)), q) >= 0 }

// settingsMatch is a page the sidebar's field found: hits are the titles of
// what it holds that the query found, none when its own title or group
// did.
type settingsMatch struct {
	settingsSection
	hits []string
}

// settingsSections returns the settings' pages, in the sidebar's order: by
// group, as Codex's; their titles in the app's language.
func settingsSections() []settingsSection {
	personal, models := locales.GroupPersonal(), locales.GroupModels()
	return []settingsSection{
		{"general", personal, locales.PageGeneral(), settingsIcon, locales.PageGeneralAbout(), (*app).generalPage, (*app).generalTerms},
		{"instructions", personal, locales.PageInstructions(), instructionsIcon, locales.PageInstructionsAbout(), (*app).instructionsPage, (*app).instructionsTerms},
		{"appearance", personal, locales.PageAppearance(), sunIcon, locales.PageAppearanceAbout(), (*app).appearancePage, (*app).appearanceTerms},
		{"shortcuts", personal, locales.PageShortcuts(), keyboardIcon, locales.PageShortcutsAbout(), (*app).shortcutsPage, (*app).shortcutTerms},
		{"providers", models, locales.PageProviders(), plugIcon, locales.PageProvidersAbout(), (*app).providersPage, (*app).providerTerms},
		{"model", models, locales.PageModel(), logoIcon, locales.PageModelAbout(), (*app).modelPage, (*app).modelTerms},
		{"about", locales.GroupApp(), locales.PageAbout(), infoIcon, locales.PageAboutAbout(), (*app).aboutPage, (*app).aboutTerms},
	}
}

// section returns the settings' page shown, the first when none is.
func (a *app) section() settingsSection {
	sections := settingsSections()
	for _, s := range sections {
		if s.key == a.settingsPage {
			return s
		}
	}
	return sections[0]
}

// openSettings shows the settings, on the page shown last, with the
// sidebar, which lists their pages.
func (a *app) openSettings() {
	a.settings, a.settingsQuery, a.collapsed, a.searching = true, "", false, false
	// As the system has it now: the user may have changed it there.
	if a.openAtLogin != nil {
		a.atLogin = a.openAtLogin()
	}
}

// showProviders shows the settings' Providers page, which takes the API
// keys.
func (a *app) showProviders() {
	a.openSettings()
	a.settingsPage = "providers"
}

// closeSettings goes back to the chat shown before them.
func (a *app) closeSettings() {
	a.settings, a.focusDraft = false, true
}

// settingsRow builds the row at the sidebar's foot that opens the
// settings.
func (a *app) settingsRow(c *ui.Context) {
	if a.navRow(c, "settings", settingsIcon, locales.Settings(), settingsKeys.String()).Margin(rowGap/2, barClear, 8, 8).Clicked() {
		a.openSettings()
	}
}

// settingsSidebar builds the sidebar of the settings, as Codex's: back to
// the app, a field that narrows the pages by their titles, and the pages
// by group, the one shown on a wash.
func (a *app) settingsSidebar(c *ui.Context) {
	t := c.Theme()
	ui.Box(c).Height(titleBarHeight).Shrink(0).DragWindow()

	back := ui.ButtonBase(c.Key("back to app")).Margin(rowGap/2, barClear, rowGap/2, 8).Padding(7, 12).Gap(10).Radius(8).
		Justify(ui.Start).TextColor(t.TextMuted).Label(locales.SettingsBack())
	if back.Hovered() {
		back.Background(a.pal.fg.Alpha(0.05)).TextColor(t.Text)
	}
	back.Children(func() {
		ui.Icon(c, arrowLeftIcon).Size(16, 16)
		ui.Text(c, locales.SettingsBack()).Grow(1)
	})
	if back.Clicked() {
		a.closeSettings()
	}

	// A field of its own, on the composer's face, as the sidebar is none.
	box := ui.Row(c).Shrink(0).Height(34).Margin(6, barClear, 4, 8).Padding(0, 12).Gap(8).AlignItems(ui.Center).
		Radius(8).Background(a.pal.composer).Border(1, t.Border)
	box.Children(func() {
		ui.Icon(c, searchIcon).Size(14, 14).TextColor(t.TextMuted)
		ui.Box(c).Grow(1).MinWidth(0).Children(func() {
			field := ui.TextInputBase(c.Key("settings query"), &a.settingsQuery).WidthPercent(100).
				Label(locales.SettingsSearch()).FocusRing(false)
			a.placeholder(c, locales.SettingsSearchPlaceholder(), a.settingsQuery == "" && !field.Composing(), 0, 0, t.FontSize, 0)
		})
	})

	sections := a.settingsMatches()
	// The page follows the field: the first it finds, once what was shown
	// is not among them.
	if a.settingsQuery != "" && len(sections) > 0 &&
		!slices.ContainsFunc(sections, func(m settingsMatch) bool { return m.key == a.section().key }) {
		a.settingsPage = sections[0].key
	}
	ui.Scroll(c.Key("settings pages")).Grow(1).MinHeight(0).Padding(0, 0, 12, 0).Label(locales.Settings()).Children(func() {
		if len(sections) == 0 {
			ui.Text(c, locales.SettingsNoMatch()).FontSize(13).TextColor(t.TextMuted).Padding(14, 20)
			return
		}
		shown := a.section().key
		group := ""
		for _, s := range sections {
			if s.group != group {
				group = s.group
				ui.Text(c, group).FontSize(12).TextColor(t.TextMuted).Padding(14, 20, 6, 20)
			}
			on := s.key == shown
			row := ui.ButtonBase(c.Key("settings "+s.key)).Margin(rowGap/2, barClear, rowGap/2, 8).Padding(7, 12).Gap(10).
				Radius(8).Justify(ui.Start).AlignItems(ui.Start).Label(s.title)
			switch {
			case on:
				row.Background(a.pal.fg.Alpha(0.07))
			case row.Hovered():
				row.Background(a.pal.fg.Alpha(0.04))
			}
			row.Children(func() {
				icon := ui.Icon(c, s.icon).Size(16, 16).TextColor(t.TextMuted)
				var label ui.Element
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					label = ui.Text(c, s.title).SingleLine()
					// What the field found in it, when not the page itself.
					if len(s.hits) > 0 {
						ui.Text(c, hitsText(s.hits)).FontSize(12).TextColor(t.TextMuted).SingleLine()
					}
				})
				if on {
					icon.TextColor(t.Text)
					label.FontWeight(500)
				}
			})
			if row.Clicked() {
				a.settingsPage = s.key
			}
		}
	})
}

// settingsMatches returns the settings' pages whose title or group holds
// the query, in any case, or some of what they hold: all for none.
func (a *app) settingsMatches() []settingsMatch {
	q := lowerRunes([]rune(a.settingsQuery))
	var out []settingsMatch
	for _, s := range settingsSections() {
		if len(q) == 0 || holds(s.title, q) || holds(s.group, q) {
			out = append(out, settingsMatch{settingsSection: s})
			continue
		}
		if s.terms == nil {
			continue
		}
		var hits []string
		for _, t := range s.terms(a) {
			if t.matches(q) {
				hits = append(hits, t.title)
			}
		}
		if len(hits) > 0 {
			out = append(out, settingsMatch{s, hits})
		}
	}
	return out
}

// hitsText returns what a page's hits say under it, in the sidebar's
// narrow room: the first, and how many more, which the page lists.
func hitsText(hits []string) string {
	if len(hits) == 1 {
		return hits[0]
	}
	return locales.SettingsHits(hits[0], len(hits)-1)
}

// settingsPane builds the page of the settings shown, beside their
// sidebar: its title in the title bar, and over the page, what it holds.
// titleLeft is the room left of the title, as for the chat's pane.
func (a *app) settingsPane(c *ui.Context, titleLeft float32) {
	t := c.Theme()
	bar := c.TitleBar()
	s := a.section()
	ui.Column(c).Grow(1).MinWidth(0).Background(t.Background).Children(func() {
		ui.Row(c).Height(titleBarHeight).Shrink(0).AlignItems(ui.Center).Padding(0, bar.Right+12, 0, titleLeft).
			DragWindow().Children(func() {
			ui.Text(c, locales.Settings()).FontSize(14).FontWeight(500).SingleLine()
		})
		ui.Scroll(c.Key("settings page")).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).AlignItems(ui.Center).Padding(28, 24).Children(func() {
				ui.Column(c).WidthPercent(100).MaxWidth(settingsWidth).Gap(8).AlignItems(ui.Stretch).Children(func() {
					ui.Text(c, s.title).FontSize(20).FontWeight(500)
					ui.Text(c, s.about).FontSize(14).LineHeight(1.6).TextColor(t.TextMuted)
					// The page's groups, further apart than its lines.
					ui.Column(c).Margin(16, 0, 0, 0).Gap(28).AlignItems(ui.Stretch).Children(func() {
						if s.page != nil {
							s.page(a, c)
						} else {
							ui.Text(c, locales.SettingsEmpty()).FontSize(13).TextColor(t.TextMuted).
								Padding(16).Radius(12).Border(1, t.Border)
						}
					})
				})
			})
		})
	})
}

// settingsWidth is how wide the settings' pages are at most: narrower than
// the conversation, as a form reads.
const settingsWidth = 640
