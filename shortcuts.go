package main

import (
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// keys are the keys of a shortcut, as their caps show them, the modifiers
// first: mac on macOS, other elsewhere.
type keys struct{ mac, other []string }

// caps returns the keys for this platform.
func (k keys) caps() []string {
	if runtime.GOOS == "darwin" {
		return k.mac
	}
	return k.other
}

// String returns the keys as menus and tooltips write them: run together
// on macOS (⇧⌘S), joined by + elsewhere (Ctrl+Shift+S).
func (k keys) String() string {
	if runtime.GOOS == "darwin" {
		return strings.Join(k.mac, "")
	}
	return strings.Join(k.other, "+")
}

// The app's own shortcuts, which the sidebar, the title bar's tooltips and
// the Keyboard shortcuts page show: the keys the view takes for them, as
// MyGo's Cmd is ⌘ on macOS and Ctrl elsewhere.
var (
	newChatKeys  = keys{[]string{"⌘", "N"}, []string{"Ctrl", "N"}}
	findKeys     = keys{[]string{"⌘", "F"}, []string{"Ctrl", "F"}}
	settingsKeys = keys{[]string{"⌘", ","}, []string{"Ctrl", ","}}
	sidebarKeys  = keys{[]string{"⇧", "⌘", "S"}, []string{"Ctrl", "Shift", "S"}}
	backKeys     = keys{[]string{"⌘", "["}, []string{"Alt", "←"}}
	forwardKeys  = keys{[]string{"⌘", "]"}, []string{"Alt", "→"}}
)

// key is a shortcut of one key, the same everywhere.
func key(k string) keys { return keys{[]string{k}, []string{k}} }

// returnKey is Return, as the key is named on Macs, and Enter elsewhere.
var returnKey = keys{[]string{"Return"}, []string{"Enter"}}

// onMac returns mac on macOS and other elsewhere.
func onMac(mac, other string) string {
	if runtime.GOOS == "darwin" {
		return mac
	}
	return other
}

// shortcutRow is a row of the Keyboard shortcuts page: what the keys do,
// when they do it (about, "" for always), and the keys, any of them.
type shortcutRow struct {
	title, about string
	keys         []keys
}

// shortcutGroups are the shortcuts, by where they work, as the page lists
// them, in the app's language: the composer's as General has its keys.
func (a *app) shortcutGroups() []struct {
	title string
	rows  []shortcutRow
} {
	shiftReturn := keys{append([]string{"⇧"}, returnKey.mac...), append([]string{"Shift"}, returnKey.other...)}
	send, newLine := []keys{returnKey}, []keys{shiftReturn}
	if a.sendWith == sendCmdReturn {
		send, newLine = []keys{cmdReturnKeys}, []keys{returnKey, shiftReturn}
	}
	return []struct {
		title string
		rows  []shortcutRow
	}{
		{locales.GroupApp(), []shortcutRow{
			{locales.NewChat(), "", []keys{newChatKeys}},
			{locales.Search(), locales.KeysSearchAbout(), []keys{findKeys}},
			{locales.KeysSidebar(), "", []keys{sidebarKeys}},
			{locales.Settings(), locales.KeysSettingsAbout(), []keys{settingsKeys}},
			{locales.KeysCloseSettings(), locales.KeysCloseSettingsAbout(), []keys{key("Esc")}},
		}},
		{locales.KeysBackForth(), []shortcutRow{
			{locales.Back(), locales.KeysBackAbout(platform()), []keys{backKeys}},
			{locales.Forward(), locales.KeysForwardAbout(platform()), []keys{forwardKeys}},
		}},
		{locales.Composer(), []shortcutRow{
			{locales.Send(), locales.KeysSendAbout(), send},
			{locales.KeysNewLine(), "", newLine},
			{locales.KeysStop(), locales.KeysStopAbout(), []keys{key("Esc")}},
		}},
		{locales.Search(), []shortcutRow{
			{locales.KeysNextResult(), "", []keys{key("↓"), {[]string{"⌃", "N"}, []string{"Ctrl", "N"}}}},
			{locales.KeysPreviousResult(), "", []keys{key("↑"), {[]string{"⌃", "P"}, []string{"Ctrl", "P"}}}},
			{locales.KeysOpenResult(), "", []keys{returnKey}},
			{locales.KeysPreviousType(), locales.KeysNothingTyped(), []keys{key("←")}},
			{locales.KeysNextType(), locales.KeysNothingTyped(), []keys{key("→")}},
			{locales.KeysAskNew(), locales.KeysAskNewAbout(), []keys{key("Tab")}},
			{locales.Close(), "", []keys{key("Esc")}},
		}},
	}
}

// term returns the row as the settings' field finds it: by what the keys
// do, when, and the keys, as their caps show them and as menus write them.
func (r shortcutRow) term() settingsTerm {
	t := settingsTerm{title: r.title, more: []string{r.about}}
	for _, k := range r.keys {
		t.more = append(t.more, k.String())
		t.more = append(t.more, k.caps()...)
	}
	return t
}

// shortcutTerms returns the shortcuts as the settings' field finds the
// page by them.
func (a *app) shortcutTerms() []settingsTerm {
	var out []settingsTerm
	for _, g := range a.shortcutGroups() {
		for _, r := range g.rows {
			out = append(out, r.term())
		}
	}
	return out
}

// shortcutsPage builds the Keyboard shortcuts page: the shortcuts, by
// where they work, as cards of rows, each its keys on the right, drawn as
// keys. While the settings' field finds some of them, it shows those
// alone, under what it found them by, and a way to the rest.
func (a *app) shortcutsPage(c *ui.Context) {
	t := c.Theme()
	q := lowerRunes([]rune(a.settingsQuery))
	groups := a.shortcutGroups()
	found := false
	if len(q) > 0 {
		for i, g := range groups {
			groups[i].rows = slices.DeleteFunc(slices.Clone(g.rows), func(r shortcutRow) bool { return !r.term().matches(q) })
			found = found || len(groups[i].rows) > 0
		}
	}
	if !found {
		groups = a.shortcutGroups()
	} else {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.KeysMatching(a.settingsQuery)).FontSize(13).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine()
			all := ui.ButtonBase(c.Key("all shortcuts")).Padding(4, 8).Radius(6).TextColor(t.TextMuted).Label(locales.KeysShowAllLabel())
			if all.Hovered() {
				all.TextColor(a.pal.ac)
			}
			all.Children(func() { ui.Text(c, locales.KeysShowAll()).FontSize(13) })
			if all.Clicked() {
				a.settingsQuery = ""
			}
		})
	}
	for _, g := range groups {
		if len(g.rows) == 0 {
			continue
		}
		a.settingsCard(c, g.title, func() {
			for i, r := range g.rows {
				if i > 0 {
					settingDivider(c)
				}
				a.settingRow(c, r.title, r.about, func() { a.shortcutKeys(c, r.keys) })
			}
		})
	}
}

// shortcutKeys builds the keys of a shortcut, any of alternatives: the
// keys of each, pressed together, as caps side by side, "or" between them.
func (a *app) shortcutKeys(c *ui.Context, alternatives []keys) {
	t := c.Theme()
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		for i, k := range alternatives {
			if i > 0 {
				ui.Text(c, locales.KeysOr()).FontSize(12).TextColor(t.TextMuted)
			}
			ui.Row(c).Gap(3).Children(func() {
				for _, cap := range k.caps() {
					a.keyCap(c, cap)
				}
			})
		}
	})
}

// keyCap builds a key, drawn as a key: its label, 11 in the muted text, on
// a wash, as the search's footer has them; an arrow drawn, as the system
// font's arrows sit low.
func (a *app) keyCap(c *ui.Context, label string) {
	t := c.Theme()
	arrows := map[string]*ui.SVG{"←": arrowLeftIcon, "→": arrowRightIcon, "↑": arrowUpIcon, "↓": arrowDownIcon}
	k := ui.Row(c).Height(18).MinWidth(18).Padding(0, 5).Radius(4).Background(a.pal.fg.Alpha(0.06)).
		Justify(ui.Center).AlignItems(ui.Center)
	k.Children(func() {
		if svg := arrows[label]; svg != nil {
			ui.Icon(c, svg).Size(11, 11).TextColor(t.TextMuted)
		} else {
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
		}
	})
}
