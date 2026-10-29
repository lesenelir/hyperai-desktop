package main

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// themes are the appearances the settings choose from, as
// mygo.ThemeSource names them: the system's, or always light or dark.
var (
	themes      = [...]string{"system", "light", "dark"}
	themeTitles = [...]string{"System", "Light", "Dark"}
)

// textSizes are the sizes the settings choose from for the conversation's
// text: hyperai-next's, and one smaller and one larger, a step apart that
// shows.
var (
	textSizes      = [...]float32{14, bodySize, 17}
	textSizeTitles = [...]string{"Small", "Default", "Large"}
)

const defaultTextSize = 1 // bodySize, of textSizes

// motionSettle is how long the app keeps moving after Reduce motion is
// switched: for the switch's knob, which slides for 140 ms as every
// switch's, to slide too, rather than jump as the motion it turns off.
// It moves while the pointer is on the switch, or the focus shows on it,
// too (app.motionHeld).
const motionSettle = 250 * time.Millisecond

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
// choose from: asked of the system once, as a menu of them first opens.
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
		for _, m := range th.Messages {
			m.wrapWidth = 0
		}
	}
}

// appearancePage builds the Appearance page: the theme, chosen of three
// small windows drawn in it, as macOS's; the text, its font and the
// conversation's size, over a question and a reply in them, since the
// settings hide the conversation; and the window, a switch a row.
func (a *app) appearancePage(c *ui.Context) {
	settingsGroup(c, "Theme", func() {
		ui.Row(c).Gap(12).Children(func() {
			for i, theme := range themes {
				a.themeCard(c, theme, themeTitles[i])
			}
		})
	})
	a.settingsCard(c, "Text", func() {
		a.settingRow(c, "Font", "All of the window's text, but code.", func() {
			a.fontButton(c, "Font", &a.font, false)
		})
		settingDivider(c)
		a.settingRow(c, "Code font", "Code blocks and code in a line, in replies.", func() {
			a.fontButton(c, "Code font", &a.codeFont, true)
		})
		settingDivider(c)
		a.settingRow(c, "Code theme", "The colors of code, in the app's light or dark as the app is.", func() {
			a.codeThemeButton(c)
		})
		settingDivider(c)
		a.settingRow(c, "Text size", "Questions, replies and the composer. The rest of the window keeps its size.", func() {
			if ui.Segmented(c.Key("text size"), &a.textSize, textSizeTitles[:]...).Label("Text size").Changed() {
				a.saveLayout()
			}
		})
		settingDivider(c)
		// A question and its reply, as the conversation shows them.
		ui.Column(c).Padding(16).Gap(10).AlignItems(ui.Stretch).Label("Preview").Children(func() {
			ui.Row(c).Children(func() {
				ui.Spacer(c)
				ui.Text(c, "Can you explain closures in Go?").FontSize(a.body).LineHeight(bodyLineHeight).
					Padding(bubblePadY, bubblePadX).Radius(14).Background(a.pal.raised)
			})
			markdown.View(c, previewReply, &a.md)
		})
	})
	a.settingsCard(c, "Window", func() {
		// The material is the desktop's to give: macOS's and Windows 11's.
		material := c.Vibrancy()
		about := "Let the desktop show through the sidebar."
		if !material {
			about = "This system draws no material behind windows."
		}
		a.settingRow(c, "Translucent sidebar", about, func() {
			// Off, and still, where there is none.
			on, none := &a.translucent, false
			if !material {
				on = &none
			}
			if ui.Switch(c.Key("translucent sidebar"), on).Label("Translucent sidebar").Disabled(!material).Changed() {
				a.saveLayout()
			}
		})
		settingDivider(c)
		a.settingRow(c, "Reduce motion", "Show changes at once rather than fading or gliding. On whenever the system asks for less motion.", func() {
			// On, and still, while the system reduces it.
			system := c.Preferences().ReduceMotion
			on, always := &a.reduceMotion, true
			if system {
				on = &always
			}
			sw := ui.Switch(c.Key("reduce motion"), on).Label("Reduce motion").Disabled(system)
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

// previewReply is the reply of the Text card's preview: prose, code in a
// line, and a block of it, to show both fonts.
const previewReply = "A closure is a function that keeps the variables around it alive, as `n` here, so it can read and change them after they go out of scope:\n\n" +
	"```go\nfunc counter() func() int {\n\tn := 0\n\treturn func() int { n++; return n }\n}\n```"

// fontButton builds the pop-up button, labeled label, that chooses *font:
// System, then every family installed, the one chosen checked, as macOS's
// font settings do. For code, the monospaced families come first and the
// rest in a submenu.
func (a *app) fontButton(c *ui.Context, label string, font *string, code bool) {
	name := *font
	if name == "" {
		name = "System"
	}
	a.popUpButton(c, label, name, func(m *ui.Menu) {
		item := func(m *ui.Menu, label, family string) {
			if m.Item(label).Checked(*font == family).Chosen() {
				a.setFont(font, family)
			}
		}
		item(m, "System", "")
		m.Separator()
		for _, f := range a.fontFamilies() {
			if !code || f.Monospaced {
				item(m, f.Name, f.Name)
			}
		}
		if code {
			m.Submenu("Other Fonts", func(m *ui.Menu) {
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
// Default, then the others, the one chosen checked.
func (a *app) codeThemeButton(c *ui.Context) {
	shown := codeThemeNamed(a.codeTheme)
	a.popUpButton(c, "Code theme", shown.title, func(m *ui.Menu) {
		for i, ct := range codeThemes {
			if i == 1 {
				m.Separator() // after Default
			}
			if m.Item(ct.title).Checked(ct.key == a.codeTheme).Chosen() && ct.key != a.codeTheme {
				a.codeTheme = ct.key
				a.saveLayout()
			}
		}
	})
}

// popUpButton builds a pop-up button labeled label, as macOS's: what is
// chosen, shown, over an arrow, which opens the system's menu that build
// fills with the choices.
func (a *app) popUpButton(c *ui.Context, label, shown string, build func(m *ui.Menu)) {
	t := c.Theme()
	btn := ui.ButtonBase(c.Key(label)).Width(200).Padding(5, 8, 5, 10).Gap(6).Radius(8).Justify(ui.Start).
		Background(a.pal.composer).Border(1, t.Border).Label(label)
	if btn.Hovered() {
		btn.Background(a.pal.fg.Alpha(0.04).Over(a.pal.composer))
	}
	btn.Children(func() {
		ui.Text(c, shown).Grow(1).MinWidth(0).SingleLine()
		ui.Icon(c, chevronIcon).Size(14, 14).TextColor(t.TextMuted)
	})
	btn.Menu(build)
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
// more of that, on the left; on the right, the control that sets it.
func (a *app) settingRow(c *ui.Context, title, about string, control func()) {
	t := c.Theme()
	ui.Row(c).Padding(12, 16).Gap(24).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(14)
			ui.Text(c, about).FontSize(12).LineHeight(1.5).TextColor(t.TextMuted)
		})
		ui.Row(c).Shrink(0).Children(control)
	})
}

// settingDivider builds the hairline between two rows of a settings card,
// in from its left as the rows' text.
func settingDivider(c *ui.Context) {
	ui.Box(c).Height(1).Margin(0, 0, 0, 16).Background(c.Theme().Border)
}
