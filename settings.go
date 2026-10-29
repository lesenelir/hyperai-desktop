package main

import (
	"github.com/egoist/mygo/ui"
)

// settingsSection is a page of the settings, under its group in the
// sidebar: about says what it holds, under its title, and page builds what
// it holds, nil until it holds something.
type settingsSection struct {
	key, group, title string
	icon              *ui.SVG
	about             string
	page              func(a *app, c *ui.Context)
}

// settingsSections are the settings' pages, in the sidebar's order: by
// group, as Codex's.
var settingsSections = []settingsSection{
	{"general", "Personal", "General", settingsIcon, "How HyperAI starts and behaves: its language, new chats, and sending with Enter.", nil},
	{"appearance", "Personal", "Appearance", sunIcon, "How HyperAI looks: its theme and font, the conversation's text, and the window.", (*app).appearancePage},
	{"shortcuts", "Personal", "Keyboard shortcuts", keyboardIcon, "The keys that do what HyperAI does, by where they work.", (*app).shortcutsPage},
	{"providers", "Models", "Providers", plugIcon, "Where replies come from: OpenRouter and its API key, and other providers.", nil},
	{"model", "Models", "Default model", logoIcon, "The model a new chat asks, until it is changed in the composer.", nil},
	{"about", "App", "About", infoIcon, "HyperAI's version, and what changed in it.", nil},
}

// section returns the settings' page shown, the first when none is.
func (a *app) section() settingsSection {
	for _, s := range settingsSections {
		if s.key == a.settingsPage {
			return s
		}
	}
	return settingsSections[0]
}

// openSettings shows the settings, on the page shown last, with the
// sidebar, which lists their pages.
func (a *app) openSettings() {
	a.settings, a.settingsQuery, a.collapsed, a.searching = true, "", false, false
}

// closeSettings goes back to the chat shown before them.
func (a *app) closeSettings() {
	a.settings, a.focusDraft = false, true
}

// settingsRow builds the row at the sidebar's foot that opens the
// settings.
func (a *app) settingsRow(c *ui.Context) {
	if a.navRow(c, "settings", settingsIcon, "Settings", settingsKeys.String()).Margin(rowGap/2, barClear, 8, 8).Clicked() {
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
		Justify(ui.Start).TextColor(t.TextMuted).Label("Back to app")
	if back.Hovered() {
		back.Background(a.pal.fg.Alpha(0.05)).TextColor(t.Text)
	}
	back.Children(func() {
		ui.Icon(c, arrowLeftIcon).Size(16, 16)
		ui.Text(c, "Back to app").Grow(1)
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
				Label("Search settings").FocusRing(false)
			a.placeholder(c, "Search settings…", a.settingsQuery == "" && !field.Composing(), 0, 0, t.FontSize, 0)
		})
	})

	sections := a.settingsMatches()
	ui.Scroll(c.Key("settings pages")).Grow(1).MinHeight(0).Padding(0, 0, 12, 0).Label("Settings").Children(func() {
		if len(sections) == 0 {
			ui.Text(c, "No settings match.").FontSize(13).TextColor(t.TextMuted).Padding(14, 20)
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
				Radius(8).Justify(ui.Start).Label(s.title)
			switch {
			case on:
				row.Background(a.pal.fg.Alpha(0.07))
			case row.Hovered():
				row.Background(a.pal.fg.Alpha(0.04))
			}
			row.Children(func() {
				icon := ui.Icon(c, s.icon).Size(16, 16).TextColor(t.TextMuted)
				label := ui.Text(c, s.title).Grow(1).SingleLine()
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
// the query, in any case: all for none.
func (a *app) settingsMatches() []settingsSection {
	q := lowerRunes([]rune(a.settingsQuery))
	if len(q) == 0 {
		return settingsSections
	}
	var out []settingsSection
	for _, s := range settingsSections {
		if find(lowerRunes([]rune(s.title)), q) >= 0 || find(lowerRunes([]rune(s.group)), q) >= 0 {
			out = append(out, s)
		}
	}
	return out
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
			ui.Text(c, "Settings").FontSize(14).FontWeight(500).SingleLine()
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
							ui.Text(c, "Nothing to change here yet.").FontSize(13).TextColor(t.TextMuted).
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
