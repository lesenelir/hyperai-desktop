package main

import (
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// usage is what a reply took of its model, as its provider counted it: the
// tokens of the conversation it was asked, Cached of them read from the
// provider's prompt cache, and those it wrote, its reasoning among them.
// The providers give it as their streams end (Stream), where they count
// it: Anthropic's API, those that take OpenAI's (stream_options), and
// Copilot's.
type usage struct {
	Input  int64 `json:"input"`
	Cached int64 `json:"cached,omitempty"`
	Output int64 `json:"output"`
}

// usageRow is a row of a reply's usage: what it says, and how much.
type usageRow struct{ label, value string }

// usageRows returns m's usage as its card shows it, in the app's language:
// its tokens, where its provider counted them; how long its first token
// took, and the whole of it; and how fast it came, once it began.
func usageRows(m *message) []usageRow {
	lang := i18n.Locale()
	tokens := func(n int64) string { return i18n.FormatNumber(lang, n, i18n.NumberOptions{Integer: true}) }
	tenths := func(f float64) string {
		return i18n.FormatNumber(lang, f, i18n.NumberOptions{MinFractionDigits: 1, MaxFractionDigits: 1})
	}
	var rows []usageRow
	if u := m.Usage; u != nil {
		rows = append(rows,
			usageRow{locales.UsageInput(), tokens(u.Input)},
			usageRow{locales.UsageCached(), tokens(u.Cached)},
			usageRow{locales.UsageOutput(), tokens(u.Output)})
	}
	if m.FirstToken > 0 {
		rows = append(rows, usageRow{locales.UsageFirstToken(), locales.UsageSeconds(tenths(m.FirstToken))})
	}
	if m.Elapsed > 0 {
		rows = append(rows, usageRow{locales.UsageDuration(), locales.UsageSeconds(tenths(m.Elapsed))})
	}
	// Of the time it wrote, past its first token.
	if writing := m.Elapsed - m.FirstToken; m.Usage != nil && m.Usage.Output > 0 && m.FirstToken > 0 && writing > 0 {
		rows = append(rows, usageRow{locales.UsageSpeed(), locales.UsageTokensPerSecond(tenths(float64(m.Usage.Output) / writing))})
	}
	return rows
}

// usageButton builds the button under reply m that shows its usage, in a
// card under it, as a popover: the rows of usageRows, what each says on the
// left, muted, and how much on the right, in tabular figures. One card
// shows at a time; clicking away or Escape closes it.
func (a *app) usageButton(c *ui.Context, m *message) {
	rows := usageRows(m)
	if len(rows) == 0 {
		return
	}
	b := actionButton(c.Key("usage"), usageIcon, locales.UsageButton(), actionIcon)
	b.OnClick(func() { a.usageFor, a.usageOpen = m.ID, a.usageFor != m.ID || !a.usageOpen })
	if a.usageFor != m.ID || !a.usageOpen {
		return
	}
	t := c.Theme()
	ui.PopoverBase(c, b, &a.usageOpen, func(panel ui.Element) {
		panel.MinWidth(240).Padding(10, 14).Radius(12).Gap(6).Background(a.pal.composer).Border(1, t.Border).
			Shadow(0, 10, 24, -6, a.pal.shadow(0.2)).TextColor(t.Text).Label(locales.UsageButton())
		panel.Children(func() {
			for _, r := range rows {
				ui.Row(c).Gap(24).AlignItems(ui.Center).Children(func() {
					ui.Text(c, r.label).FontSize(13).TextColor(t.TextMuted).SingleLine()
					ui.Spacer(c)
					ui.Text(c, r.value).FontSize(13).FontFeatures("tnum").SingleLine()
				})
			}
		})
	})
}
