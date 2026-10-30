package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// The user's instructions are what every model is told, in every chat, as
// its system prompt: about them, and how to reply. They go first in the
// conversation a reply is asked of (withInstructions), a turn of the role
// "system", which OpenAI's API and those like it take as it is, Anthropic's
// takes as its system prompt, and Copilot is told after its own; never in
// the chats kept, so that they apply from the next reply on, in the chats
// there are too. A chat's title is asked for without them. layout.json
// keeps them.

// withInstructions returns turns, the conversation a reply is asked of,
// after the user's instructions, where they wrote any.
func (a *app) withInstructions(turns []turn) []turn {
	s := strings.TrimSpace(a.instructions)
	if s == "" {
		return turns
	}
	return append([]turn{{Role: "system", Content: s}}, turns...)
}

// instructionsTerms returns what the Instructions page holds, as the
// settings' field finds it.
func (a *app) instructionsTerms() []settingsTerm {
	return []settingsTerm{{locales.InstructionsCard(), terms(locales.InstructionsTerms())}}
}

// instructionsPage builds the Instructions page: the field of the user's
// instructions, on a card, as tall as six lines at first and sixteen at
// most, in a hairline, its placeholder an example of them; and under it,
// when they apply. What is typed is kept at once.
func (a *app) instructionsPage(c *ui.Context) {
	t := c.Theme()
	a.settingsCard(c, locales.InstructionsCard(), func() {
		ui.Column(c).Padding(14, 16).Gap(10).AlignItems(ui.Stretch).Children(func() {
			ui.Box(c).Radius(8).Border(1, t.Border).Background(a.pal.composer).Children(func() {
				field := ui.TextAreaBase(c.Key("instructions"), &a.instructions).WidthPercent(100).Lines(6, 16).
					FontSize(14).LineHeight(1.5).Padding(10, 12).Label(locales.InstructionsCard())
				a.placeholder(c, locales.InstructionsPlaceholder(), a.instructions == "" && !field.Composing(), 10, 12, 14, 1.5)
				if field.Changed() {
					a.saveLayout()
				}
			})
			ui.Text(c, locales.InstructionsAbout()).FontSize(12).LineHeight(1.5).TextColor(t.TextMuted)
		})
	})
}
