package main

import (
	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// Where the About page's links go: HyperAI's website, and its project of
// AI for Science research on GitHub.
const (
	websiteURL = "https://hyper.ai"
	ai4sURL    = "https://github.com/hyperai/awesome-ai4s"
	ai4sTitle  = "Awesome AI for Science" // a name, the same in every language
)

// aboutTerms returns what the About page holds, as the settings' field
// finds it.
func (a *app) aboutTerms() []settingsTerm {
	return []settingsTerm{
		{locales.AboutVersionTitle(), terms(locales.AboutVersionTerms())},
		{locales.AboutCommunity(), terms(locales.AboutCommunityTerms())},
		{locales.AboutWebsite(), []string{"hyper.ai"}},
		{ai4sTitle, []string{"GitHub", "AI4S"}},
	}
}

// aboutPage builds the About page: HyperAI and its version, over what
// HyperAI is, as hyperai-next's About page says it, and where to read
// more.
func (a *app) aboutPage(c *ui.Context) {
	t := c.Theme()
	// HyperAI itself, on a card of its own: its mark, in the accent as over
	// an empty chat, beside its name and version.
	ui.Row(c).Padding(16).Gap(14).AlignItems(ui.Center).Radius(12).Background(a.pal.composer).
		Border(1, t.Border).Children(func() {
		ui.Icon(c, markIcon).Size(40, 40).TextColor(a.pal.ac).Label("HyperAI")
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, "HyperAI").FontSize(16).FontWeight(500)
			version := locales.AboutDevBuild()
			if a.version != "" {
				version = locales.AboutVersion(a.version)
			}
			ui.Text(c, version).FontSize(12).TextColor(t.TextMuted).Selectable()
		})
	})
	a.settingsCard(c, locales.AboutCommunity(), func() {
		ui.Column(c).Padding(14, 16).Gap(12).AlignItems(ui.Stretch).Children(func() {
			for _, p := range []string{locales.AboutIntro(), locales.AboutContent(), locales.AboutResources()} {
				ui.Text(c, p).FontSize(14).LineHeight(1.6).Selectable()
			}
		})
	})
	a.settingsCard(c, locales.AboutLinks(), func() {
		a.settingRow(c, locales.AboutWebsite(), "hyper.ai", func() {
			if a.settingButton(c, "open website", locales.AboutOpen(), false) {
				c.Services().OpenURL(websiteURL)
			}
		})
		settingDivider(c)
		a.settingRow(c, ai4sTitle, locales.AboutAI4S(), func() {
			if a.settingButton(c, "open ai4s", locales.AboutOpen(), false) {
				c.Services().OpenURL(ai4sURL)
			}
		})
	})
}
