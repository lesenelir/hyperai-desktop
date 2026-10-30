package main

import (
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"hyperai-desktop/locales"
)

// themes are the appearances the settings choose from, as
// mygo.ThemeSource names them: the system's, or always light or dark.
var themes = [...]string{"system", "light", "dark"}

// themeTitles name themes, in the app's language.
func themeTitles() []string {
	return []string{locales.ThemeSystem(), locales.ThemeLight(), locales.ThemeDark()}
}

// textSizes are the sizes the settings choose from for the conversation's
// text: hyperai-next's, and one smaller and one larger, a step apart that
// shows.
var textSizes = [...]float32{14, bodySize, 17}

// textSizeTitles name textSizes, in the app's language.
func textSizeTitles() []string {
	return []string{locales.TextSizeSmall(), locales.TextSizeDefault(), locales.TextSizeLarge()}
}

const defaultTextSize = 1 // bodySize, of textSizes

// motionSettle is how long the app keeps moving after Reduce motion is
// switched: for the switch's knob, which slides for 140 ms as every
// switch's, to slide too, rather than jump as the motion it turns off.
// It moves while the pointer is on the switch, or the focus shows on it,
// too (app.motionHeld).
const motionSettle = 250 * time.Millisecond

// appearanceTerms returns what the Appearance page holds, as the settings'
// field finds it: its rows' titles, and what they offer.
func (a *app) appearanceTerms() []settingsTerm {
	codes := terms(locales.CodeThemeTerms())
	for _, ct := range codeThemes {
		codes = append(codes, ct.name())
	}
	return []settingsTerm{
		{locales.Theme(), append(terms(locales.ThemeTerms()), themeTitles()...)},
		{locales.Font(), terms(locales.FontTerms())},
		{locales.CodeFont(), terms(locales.CodeFontTerms())},
		{locales.CodeTheme(), codes},
		{locales.TextSize(), textSizeTitles()},
		{locales.TranslucentSidebar(), terms(locales.TranslucentSidebarTerms())},
		{locales.ReduceMotion(), terms(locales.ReduceMotionTerms())},
	}
}

// setTheme shows the app in theme, as the app does from then on.
func (a *app) setTheme(theme string) {
	if theme == a.theme {
		return
	}
	a.theme = theme
	if a.applyTheme != nil {
		a.applyTheme(theme)
	}
	a.saveLayout()
}

// setFont makes *font, the app's font or code's, the family ("" for the
// system's), as the settings chose.
func (a *app) setFont(font *string, family string) {
	if family == *font {
		return
	}
	*font = family
	a.saveLayout()
}

// fontFamilies returns the font families installed, for the settings to
// choose from: asked of the system once, as Appearance first shows, whose
// font buttons are as wide as the names.
func (a *app) fontFamilies() []ui.FontFamily {
	if a.fonts == nil {
		a.fonts = ui.FontFamilies()
		if a.fonts == nil {
			a.fonts = []ui.FontFamily{}
		}
	}
	return a.fonts
}

// setText makes the conversation's text size DIPs, and the app's font
// family, as the settings chose: its questions and the draft are measured
// again.
func (a *app) setText(size float32, family string) {
	if size == a.body && family == a.textFont {
		return
	}
	a.body, a.textFont, a.measured = size, family, ""
	for _, th := range a.threads {
		for _, m := range th.all() {
			m.wrapWidth = 0
		}
	}
}

// appearancePage builds the Appearance page: the theme, chosen of three
// small windows drawn in it, as macOS's; the text, its font and the
// conversation's size, over a question and a reply in them, since the
// settings hide the conversation; and the window, a switch a row.
func (a *app) appearancePage(c *ui.Context) {
	settingsGroup(c, locales.Theme(), func() {
		ui.Row(c).Gap(12).Children(func() {
			titles := themeTitles()
			for i, theme := range themes {
				a.themeCard(c, theme, titles[i])
			}
		})
	})
	a.settingsCard(c, locales.Text(), func() {
		a.settingRow(c, locales.Font(), locales.FontAbout(), func() {
			a.fontButton(c, locales.Font(), &a.font, false)
		})
		settingDivider(c)
		a.settingRow(c, locales.CodeFont(), locales.CodeFontAbout(), func() {
			a.fontButton(c, locales.CodeFont(), &a.codeFont, true)
		})
		settingDivider(c)
		a.settingRow(c, locales.CodeTheme(), locales.CodeThemeAbout(), func() {
			a.codeThemeButton(c)
		})
		settingDivider(c)
		a.settingRow(c, locales.TextSize(), locales.TextSizeAbout(), func() {
			if ui.Segmented(c.Key("text size"), &a.textSize, textSizeTitles()...).Label(locales.TextSize()).Changed() {
				a.saveLayout()
			}
		})
		settingDivider(c)
		// A question and its reply, as the conversation shows them.
		ui.Column(c).Padding(16).Gap(10).AlignItems(ui.Stretch).Label(locales.Preview()).Children(func() {
			ui.Row(c).Children(func() {
				ui.Spacer(c)
				ui.Text(c, locales.PreviewQuestion()).FontSize(a.body).LineHeight(bodyLineHeight).
					Padding(bubblePadY, bubblePadX).Radius(14).Background(a.pal.raised)
			})
			markdown.View(c, locales.PreviewReply()+"\n\n"+previewCode, &a.md)
		})
	})
	a.settingsCard(c, locales.Window(), func() {
		// The material is the desktop's to give: macOS's and Windows 11's.
		material := c.Vibrancy()
		about := locales.TranslucentSidebarAbout()
		if !material {
			about = locales.TranslucentSidebarNone()
		}
		a.settingRow(c, locales.TranslucentSidebar(), about, func() {
			// Off, and still, where there is none.
			on, none := &a.translucent, false
			if !material {
				on = &none
			}
			if ui.Switch(c.Key("translucent sidebar"), on).Label(locales.TranslucentSidebar()).Disabled(!material).Changed() {
				a.saveLayout()
			}
		})
		settingDivider(c)
		a.settingRow(c, locales.ReduceMotion(), locales.ReduceMotionAbout(), func() {
			// On, and still, while the system reduces it.
			system := c.Preferences().ReduceMotion
			on, always := &a.reduceMotion, true
			if system {
				on = &always
			}
			sw := ui.Switch(c.Key("reduce motion"), on).Label(locales.ReduceMotion()).Disabled(system)
			if sw.Changed() {
				a.motionSwitched = c.Now()
				a.saveLayout()
			}
			// The app moves while the switch could turn: a frame it turns
			// in, with motion reduced, could not move its knob after.
			a.motionHeld = sw.Hovered() || sw.FocusVisible()
		})
	})
}

// themeCard builds the card that chooses theme, titled title: a small
// window in its colors over its name, ringed in the accent while chosen.
func (a *app) themeCard(c *ui.Context, theme, title string) {
	t := c.Theme()
	on := a.theme == theme
	card := ui.ButtonBase(c.Key("theme " + theme)).Column().Grow(1).Basis(0).MinWidth(0).Gap(8).
		AlignItems(ui.Stretch).Label(title)
	hover := card.Hovered()
	card.Children(func() {
		frame := ui.Row(c).Height(96).Radius(10).Clip().AlignItems(ui.Stretch).Border(1, t.Border)
		switch {
		case on:
			frame.Border(2, a.pal.ac)
		case hover:
			frame.Border(1, a.pal.fg.Alpha(0.3))
		}
		frame.Children(func() {
			switch theme {
			case "light":
				themePreview(c, lightPalette, true)
			case "dark":
				themePreview(c, darkPalette, true)
			default:
				// Half and half, as the system turns it.
				themePreview(c, lightPalette, true)
				themePreview(c, darkPalette, false)
			}
		})
		label := ui.Text(c, title).FontSize(13).TextColor(t.TextMuted).AlignSelf(ui.Center)
		if on || hover {
			label.TextColor(t.Text)
		}
		if on {
			label.FontWeight(500)
		}
	})
	if card.Clicked() {
		a.setTheme(theme)
	}
}

// previewCode is the code of the Text card's preview reply, under its
// prose (locales.PreviewReply), which has code in a line: to show both
// fonts.
const previewCode = "```go\nfunc counter() func() int {\n\tn := 0\n\treturn func() int { n++; return n }\n}\n```"

// fontButton builds the pop-up button, labeled label, that chooses *font:
// System, then every family installed, the one chosen checked, as macOS's
// font settings do. For code, the monospaced families come first and the
// rest in a submenu.
func (a *app) fontButton(c *ui.Context, label string, font *string, code bool) {
	system, others := locales.FontSystem(), locales.FontOthers()
	name := *font
	if name == "" {
		name = system
	}
	// The menu's titles: the families, but for code those not monospaced,
	// in their submenu.
	titles := func() []string {
		out := []string{system}
		for _, f := range a.fontFamilies() {
			if !code || f.Monospaced {
				out = append(out, f.Name)
			}
		}
		if code {
			out = append(out, others)
		}
		return out
	}
	a.popUpButton(c, label, name, titles, func(m *ui.Menu) {
		item := func(m *ui.Menu, label, family string) {
			if m.Item(label).Checked(*font == family).Chosen() {
				a.setFont(font, family)
			}
		}
		item(m, system, "")
		m.Separator()
		for _, f := range a.fontFamilies() {
			if !code || f.Monospaced {
				item(m, f.Name, f.Name)
			}
		}
		if code {
			m.Submenu(others, func(m *ui.Menu) {
				for _, f := range a.fontFamilies() {
					if !f.Monospaced {
						item(m, f.Name, f.Name)
					}
				}
			})
		}
	})
}

// codeThemeButton builds the pop-up button that chooses the code theme:
// its groups apart, the one chosen checked.
func (a *app) codeThemeButton(c *ui.Context) {
	shown := codeThemeNamed(a.codeTheme)
	titles := func() []string {
		var out []string
		for _, ct := range codeThemes {
			out = append(out, ct.name())
		}
		return out
	}
	a.popUpButton(c, locales.CodeTheme(), shown.name(), titles, func(m *ui.Menu) {
		for i, ct := range codeThemes {
			if i > 0 && ct.group != codeThemes[i-1].group {
				m.Separator()
			}
			if m.Item(ct.name()).Checked(ct.key == a.codeTheme).Chosen() && ct.key != a.codeTheme {
				a.codeTheme = ct.key
				a.saveLayout()
			}
		}
	})
}

// popUpButton builds a pop-up button labeled label, as macOS's: what is
// chosen, shown, over an arrow, which opens the system's menu that build
// fills with the choices, titled titles. It is as wide as the widest of
// them, as macOS's are, so that the menu opens about as wide as it: the
// system's menu is as wide as its titles, and a button wider than them
// had it open short of it.
//
// The menu opens at the bottom of the element that opens it, touching it:
// that element is popUpGap taller than the button over and under it,
// clear, so that the menu opens popUpGap under what shows, as macOS's
// does; as much less margin keeps the row as tall as the button makes it.
func (a *app) popUpButton(c *ui.Context, label, shown string, titles func() []string, build func(m *ui.Menu)) {
	t := c.Theme()
	opener := ui.ButtonBase(c.Key(label)).Padding(popUpGap, 0).Margin(-popUpGap, 0).FocusRing(false).Label(label)
	hover, focus := opener.Hovered(), opener.FocusVisible()
	opener.Children(func() {
		btn := ui.Row(c).Width(a.popUpWidth(c, label, shown, titles)).Padding(5, 8, 5, 10).Gap(6).Radius(8).
			AlignItems(ui.Center).Background(a.pal.composer).Border(1, t.Border)
		if hover {
			btn.Background(a.pal.fg.Alpha(0.04).Over(a.pal.composer))
		}
		if focus {
			// The ring the opener does not draw, round what shows.
			btn.Border(2, t.Focus)
		}
		btn.Children(func() {
			ui.Text(c, shown).Grow(1).MinWidth(0).SingleLine()
			ui.Icon(c, chevronIcon).Size(14, 14).TextColor(t.TextMuted)
		})
	})
	opener.Menu(build)
}

// Pop-up buttons are popUpPad wider than their widest title: their
// padding, gap, arrow and border. They are popUpMax wide at most, the
// longest titles cut short; a menu of fonts is wider than that, as it
// opens past the button. Their menus open popUpGap under them.
const (
	popUpPad = 10 + 6 + 14 + 8 + 2
	popUpMax = 200
	popUpGap = 6
)

// popUpWidth returns how wide the pop-up button labeled label is, shown
// showing: as its widest title, measured as the system's menu draws them,
// in the system font at the window's size, and as wide as shown in the
// app's font, which it draws in. The width is kept for the titles until
// they, the app's font or its language change, as measuring hundreds of
// fonts' names each frame would cost.
func (a *app) popUpWidth(c *ui.Context, label, shown string, titles func() []string) float32 {
	if a.popUpWidths == nil {
		a.popUpWidths = map[string]float32{}
	}
	key := label + "\x00" + a.font + "\x00" + shown + "\x00" + i18n.Locale()
	w, ok := a.popUpWidths[key]
	if !ok {
		size := c.Theme().FontSize
		for _, title := range titles() {
			tw, _ := c.MeasureText(0, ui.Span{Text: title, Font: "system-ui", Size: size})
			w = max(w, tw)
		}
		sw, _ := c.MeasureText(0, ui.Span{Text: shown, Size: size})
		w = min(max(w, sw)+popUpPad, popUpMax)
		a.popUpWidths[key] = w
	}
	return w
}

// themePreview draws HyperAI small in p's colors: the sidebar, when side,
// with a chat chosen; a question in its bubble and lines of a reply; and
// the composer, with its send button in the accent.
func themePreview(c *ui.Context, p *palette, side bool) {
	canvas := p.theme.Background
	line := func(w float32, color ui.Color) {
		ui.Box(c).Height(4).WidthPercent(w).Radius(2).Background(color)
	}
	ui.Row(c).Grow(1).Basis(0).AlignItems(ui.Stretch).Background(canvas).Children(func() {
		if side {
			ui.Column(c).WidthPercent(30).Shrink(0).Padding(14, 6).Gap(6).Background(p.fg.Alpha(0.035).Over(canvas)).
				Children(func() {
					line(70, p.fg.Alpha(0.25))
					ui.Box(c).Height(9).Radius(3).Background(p.fg.Alpha(0.07)).Children(func() {
						ui.Box(c).Height(3).WidthPercent(60).Margin(3, 0, 0, 4).Radius(2).Background(p.ac)
					})
					line(80, p.fg.Alpha(0.18))
					line(55, p.fg.Alpha(0.18))
				})
		}
		ui.Column(c).Grow(1).MinWidth(0).Padding(12, 10, 8, 10).Gap(6).AlignItems(ui.Stretch).Children(func() {
			ui.Row(c).Justify(ui.End).Children(func() {
				ui.Box(c).Height(10).WidthPercent(45).Radius(5).Background(p.raised)
			})
			line(90, p.fg.Alpha(0.22))
			line(70, p.fg.Alpha(0.22))
			ui.Spacer(c)
			ui.Row(c).Height(16).Radius(8).Padding(0, 3).Justify(ui.End).AlignItems(ui.Center).
				Background(p.composer).Border(1, p.theme.Border).Children(func() {
				ui.Box(c).Size(10, 10).Radius(5).Background(p.ac)
			})
		})
	})
}

// settingsGroup builds a titled group of a settings page.
func settingsGroup(c *ui.Context, title string, children func()) {
	ui.Column(c).Gap(8).AlignItems(ui.Stretch).Children(func() {
		ui.Text(c, title).FontSize(12).TextColor(c.Theme().TextMuted).Padding(0, 4)
		children()
	})
}

// settingsCard builds a titled group of rows on a card, as macOS's
// settings: the composer's face, which stands off the canvas, a hairline
// round it, and between its rows.
func (a *app) settingsCard(c *ui.Context, title string, rows func()) {
	settingsGroup(c, title, func() {
		ui.Column(c).Radius(12).Background(a.pal.composer).Border(1, c.Theme().Border).Clip().
			AlignItems(ui.Stretch).Children(rows)
	})
}

// settingRow builds a row of a settings card: what it sets, and under it
// more of that (none for ""), on the left; on the right, the control that
// sets it.
func (a *app) settingRow(c *ui.Context, title, about string, control func()) {
	t := c.Theme()
	ui.Row(c).Padding(12, 16).Gap(24).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(14)
			if about != "" {
				ui.Text(c, about).FontSize(12).LineHeight(1.5).TextColor(t.TextMuted)
			}
		})
		ui.Row(c).Shrink(0).Children(control)
	})
}

// settingDivider builds the hairline between two rows of a settings card,
// in from its left as the rows' text.
func settingDivider(c *ui.Context) {
	ui.Box(c).Height(1).Margin(0, 0, 0, 16).Background(c.Theme().Border)
}
