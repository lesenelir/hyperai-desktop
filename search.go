package main

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
)

// The search finds what the app has, as Claude's does: a card over the
// window, near its top, with the query and the kinds to look through over
// what it found, by section, and the keys it takes under. With no query,
// it lists the chats used last, and what to do.
const (
	maxSearchHits = 12 // the most chats All lists for a query, as Waku's tasks
	maxChatHits   = 50 // the most Chats lists
	recentChats   = 8  // the chats All lists with no query
	searchWidth   = 620
	searchHeight  = 600 // the card's height at most
)

// searchTab is a kind of thing the search looks through.
type searchTab int

const (
	tabAll searchTab = iota
	tabChats
	tabProjects
	tabArtifacts
	tabSettings
)

var searchTabs = [...]string{"All", "Chats", "Projects", "Artifacts", "Settings"}

// searchHit is a chat the search found: and, when a message of it matched
// rather than its title, who said it and the words around the match.
type searchHit struct {
	th                   *thread
	who                  string // "You" or "HyperAI"; "" when the title matched
	before, match, after string
}

// searchItem is a row of the search: a chat (hit), or something to do
// (run), which a check marks as what is on.
type searchItem struct {
	key   string
	icon  *ui.SVG
	tint  bool // the icon in the accent
	title string
	hit   *searchHit
	check bool
	run   func()
}

// searchSection is a titled run of rows.
type searchSection struct {
	title string
	items []searchItem
}

// toggleSearch opens the search, empty, on All, or closes it.
func (a *app) toggleSearch() {
	a.searching = !a.searching
	a.query, a.searchTab, a.searchSel, a.searchHover, a.hits = "", tabAll, 0, -1, nil
	a.focusSearch = a.searching
	if a.searching {
		a.collapsed = false
	}
}

// searchHits returns the chats whose title or messages hold the query, in
// any case, those whose title does first, then by when they were last
// updated; or with no query, the chats by when they were last updated.
// They are found again only when the query or the chats changed.
func (a *app) searchHits() []searchHit {
	q := strings.TrimSpace(a.query)
	if a.hits != nil && a.hitsQuery == q && a.hitsRev == a.rev {
		return a.hits
	}
	threads := slices.Clone(a.threads)
	slices.SortStableFunc(threads, func(x, y *thread) int { return y.Updated.Compare(x.Updated) })
	hits := []searchHit{}
	if q == "" {
		for _, th := range threads[:min(len(threads), maxChatHits)] {
			hits = append(hits, searchHit{th: th})
		}
	} else {
		lq := lowerRunes([]rune(q))
		var titled, said []searchHit
		for _, th := range threads {
			if find(lowerRunes([]rune(th.Title)), lq) >= 0 {
				titled = append(titled, searchHit{th: th})
			} else if h, ok := saidIn(th, lq); ok {
				said = append(said, h)
			}
		}
		hits = append(titled, said...)
		hits = hits[:min(len(hits), maxChatHits)]
	}
	a.hits, a.hitsQuery, a.hitsRev = hits, q, a.rev
	return hits
}

// searchSections returns what the search lists on its tab for its query.
func (a *app) searchSections() []searchSection {
	q := strings.TrimSpace(a.query)
	var out []searchSection
	add := func(title string, items []searchItem) {
		if len(items) > 0 {
			out = append(out, searchSection{title, items})
		}
	}
	chats := func(n int) []searchItem {
		hits := a.searchHits()
		items := make([]searchItem, 0, min(n, len(hits)))
		for i := range hits[:min(n, len(hits))] {
			h := &hits[i]
			id := strconv.Itoa(h.th.ID)
			items = append(items, searchItem{key: "chat " + id, icon: messageIcon, title: h.th.Title, hit: h,
				run: func() { a.open(id) }})
		}
		return items
	}
	switch a.searchTab {
	case tabAll:
		if q == "" {
			add("Recents", chats(recentChats))
		} else {
			add("Chats", chats(maxSearchHits))
			add("Settings", a.settingItems(q))
		}
		add("Actions", []searchItem{{key: "new chat", icon: newChatPlusIcon, title: "New chat", run: a.newChat}})
	case tabChats:
		title := "Chats"
		if q == "" {
			title = "Recents"
		}
		add(title, chats(maxChatHits))
	case tabSettings:
		add("Settings", a.settingItems(q))
	}
	return out
}

// settingItems returns the settings whose title holds q, in any case, all
// for none: the model to ask, and whether the sidebar shows.
func (a *app) settingItems(q string) []searchItem {
	var items []searchItem
	for _, model := range a.prov.Models() {
		items = append(items, searchItem{key: "model " + model, icon: logoIcon, tint: true, title: "Model: " + model,
			check: model == a.model, run: func() { a.model = model }})
	}
	sidebar := "Hide sidebar"
	if a.collapsed {
		sidebar = "Show sidebar"
	}
	items = append(items, searchItem{key: "sidebar", icon: sidebarIcon, title: sidebar,
		run: func() { a.collapsed = !a.collapsed }})
	if q == "" {
		return items
	}
	lq := lowerRunes([]rune(q))
	return slices.DeleteFunc(items, func(it searchItem) bool { return find(lowerRunes([]rune(it.title)), lq) < 0 })
}

// saidIn finds q, in lower case, in th's messages, as Waku does: in the
// latest of the user's that holds it, or else of the replies.
func saidIn(th *thread, q []rune) (searchHit, bool) {
	var reply searchHit
	found := false
	for i := len(th.Messages) - 1; i >= 0; i-- {
		m := th.Messages[i]
		text := []rune(m.Text)
		at := find(lowerRunes(text), q)
		if at < 0 {
			continue
		}
		h := snippet(th, text, at, len(q))
		if m.Role == "user" {
			h.who = "You"
			return h, true
		}
		if !found {
			h.who = "HyperAI"
			reply, found = h, true
		}
	}
	return reply, found
}

// snippet returns the hit of th at text[at:at+n] with some of the words
// around it, on a line.
func snippet(th *thread, text []rune, at, n int) searchHit {
	const lead, trail = 40, 160
	start, end := max(0, at-lead), min(len(text), at+n+trail)
	h := searchHit{th: th, before: oneLine(text[start:at]), match: oneLine(text[at : at+n]), after: oneLine(text[at+n : end])}
	if start > 0 {
		h.before = "…" + strings.TrimLeft(h.before, " ")
	}
	if end < len(text) {
		h.after += "…"
	}
	return h
}

// markup is the Markdown a snippet leaves out, which a reply's text has.
var markup = strings.NewReplacer("```", "", "`", "", "**", "", "__", "")

// oneLine returns s with each run of white space a space, and without
// Markdown's marks.
func oneLine(s []rune) string {
	return markup.Replace(spaces(s))
}

// spaces returns s with each run of white space a space.
func spaces(s []rune) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	if space {
		b.WriteByte(' ')
	}
	return b.String()
}

// lowerRunes returns s in lower case, a rune for a rune.
func lowerRunes(s []rune) []rune {
	out := make([]rune, len(s))
	for i, r := range s {
		out[i] = unicode.ToLower(r)
	}
	return out
}

// find returns where q first is in s, by runes, -1 for nowhere.
func find(s, q []rune) int {
	for i := 0; i+len(q) <= len(s); i++ {
		if slices.Equal(s[i:i+len(q)], q) {
			return i
		}
	}
	return -1
}

// runSearch does it, closing the search.
func (a *app) runSearch(it searchItem) {
	a.searching = false
	it.run()
}

// compose closes the search for a new chat, the query in its composer.
func (a *app) compose() {
	q := strings.TrimSpace(a.query)
	a.searching = false
	a.newChat()
	if q != "" {
		a.draft = q
	}
}

// moveSearch moves the chosen row of n by d, around the ends.
func (a *app) moveSearch(d, n int) {
	if n == 0 {
		return
	}
	a.searchSel = ((a.searchSel+d)%n + n) % n
	a.searchMoved = true
}

// setSearchTab shows the tab, its first row chosen.
func (a *app) setSearchTab(tab searchTab) {
	n := searchTab(len(searchTabs))
	a.searchTab = (tab%n + n) % n
	a.searchSel, a.searchMoved = 0, true
}

// searchPalette builds the search while it is open: over the window, which
// it dims a little, near its top, in the app's own look, as the composer
// and the sidebar have it. The keys work as the query is typed: Up and Down
// (or Ctrl+P and Ctrl+N) choose a row, and Enter does it; Left and Right
// change the tab while there is no query to move in; Tab writes the query
// in a new chat; Escape, the close button and a click beside the card
// close it.
func (a *app) searchPalette(c *ui.Context) {
	ui.DialogBase(c, &a.searching, func(back, panel ui.Element) {
		t := c.Theme()
		_, h := c.Size()
		top := max(48, min(h*0.09, 72))
		shade := float32(0.14)
		if t.Dark {
			shade = 0.3
		}
		back.Background(ui.Hex("#000000").Alpha(shade)).Justify(ui.Start).Padding(top, 24, 24, 24)
		panel.WidthPercent(100).MaxWidth(searchWidth).MaxHeight(min(h-top-24, searchHeight)).Radius(16).
			Background(a.pal.composer).Border(1, t.Border).Shadow(0, 16, 40, -12, a.pal.shadow(0.28)).
			Clip().Label("Search")

		// A change of the query chooses the first row.
		if q := strings.TrimSpace(a.query); q != a.searchQuery {
			a.searchQuery, a.searchSel, a.searchMoved = q, 0, true
		}
		sections := a.searchSections()
		var rows []searchItem
		for _, s := range sections {
			rows = append(rows, s.items...)
		}
		a.searchSel = min(a.searchSel, max(0, len(rows)-1))

		ui.Column(c).Shrink(0).Padding(8, 8, 10, 8).Gap(6).Children(func() {
			ui.Row(c).Height(40).Padding(0, 0, 0, 12).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, searchIcon).Size(16, 16).TextColor(t.TextMuted)
				field := ui.TextInputBase(c.Key("query"), &a.query).Grow(1).FontSize(14).
					Placeholder("Search").Label("Search").FocusRing(false).
					OnTextKey(func(k ui.TextKey) bool {
						empty := k.Text() == ""
						switch {
						case k.Key == ui.KeyDown && k.Mods == 0, k.Key == ui.KeyN && k.Mods == ui.Ctrl:
							a.moveSearch(1, len(rows))
						case k.Key == ui.KeyUp && k.Mods == 0, k.Key == ui.KeyP && k.Mods == ui.Ctrl:
							a.moveSearch(-1, len(rows))
						case k.Key == ui.KeyEnter && k.Mods == 0:
							if a.searchSel < len(rows) {
								a.runSearch(rows[a.searchSel])
							}
						case k.Key == ui.KeyTab && k.Mods == 0:
							a.compose()
						case k.Key == ui.KeyLeft && k.Mods == 0 && empty:
							a.setSearchTab(a.searchTab - 1)
						case k.Key == ui.KeyRight && k.Mods == 0 && empty:
							a.setSearchTab(a.searchTab + 1)
						default:
							return false
						}
						return true
					})
				if a.focusSearch {
					field.Focus()
					a.focusSearch = false
				}
				if iconButton(c.Key("close"), closeIcon, "Close").Clicked() {
					a.searching = false
				}
			})
			// The kinds, as the sidebar's rows: muted, and on a wash chosen.
			ui.Row(c).Gap(2).Padding(0, 4).Children(func() {
				for i, name := range searchTabs {
					on := searchTab(i) == a.searchTab
					tab := ui.ButtonBase(c.Key("tab "+name)).Padding(5, 10).Radius(8).Label(name)
					switch {
					case on:
						tab.Background(a.pal.fg.Alpha(0.07))
					case tab.Hovered():
						tab.Background(a.pal.fg.Alpha(0.04))
					}
					tab.Children(func() {
						label := ui.Text(c, name)
						if on {
							label.FontWeight(500)
						} else {
							label.TextColor(t.TextMuted)
						}
					})
					if tab.Clicked() {
						a.setSearchTab(searchTab(i))
						a.focusSearch = true
					}
				}
			})
		})
		ui.Box(c).Height(1).Shrink(0).Background(t.Border)

		if len(rows) == 0 {
			a.searchEmpty(c)
		} else {
			ui.Scroll(c.Key("rows")).Grow(1).Shrink(1).MinHeight(0).Padding(2, 0, 8, 0).Children(func() {
				hovered, i := -1, 0
				for _, s := range sections {
					// As the sidebar titles its days.
					ui.Text(c, s.title).FontSize(12).TextColor(t.TextMuted).Padding(12, 20, 4, 20)
					for _, it := range s.items {
						if a.searchRow(c, it, i == a.searchSel) {
							hovered = i
						}
						i++
					}
				}
				// The pointer chooses the row it comes to; resting, it
				// leaves the keys to choose.
				if hovered != a.searchHover {
					if hovered >= 0 {
						a.searchSel = hovered
					}
					a.searchHover = hovered
				}
			})
		}
		a.searchKeys(c)
	})
}

// searchRow builds the row of it, chosen or not, as a row of the sidebar's,
// and reports whether the pointer is over it: its icon and title, a chat's
// age, Enter's mark when chosen, and for a chat a message of which
// matched, the words around the match.
func (a *app) searchRow(c *ui.Context, it searchItem, chosen bool) bool {
	t := c.Theme()
	row := ui.ButtonBase(c.Key(it.key)).Margin(0, barClear, 0, 8).Padding(7, 12).Gap(10).Radius(8).
		Justify(ui.Start).AlignItems(ui.Center).Label(it.title)
	if chosen {
		row.Background(a.pal.fg.Alpha(0.06))
		if a.searchMoved {
			row.ScrollIntoView()
			a.searchMoved = false
		}
	}
	row.Children(func() {
		icon := ui.Icon(c, it.icon).Size(16, 16).Shrink(0)
		if it.tint {
			icon.TextColor(a.pal.ac)
		} else {
			icon.TextColor(t.TextMuted)
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, it.title).SingleLine()
			if h := it.hit; h != nil && h.who != "" {
				ui.RichText(c).FontSize(12).TextColor(t.TextMuted).SingleLine().Children(func() {
					ui.Text(c, h.who+": ").FontWeight(500)
					ui.Text(c, h.before)
					ui.Text(c, h.match).FontWeight(500).TextColor(t.Text)
					ui.Text(c, h.after)
				})
			}
		})
		if h := it.hit; h != nil {
			ui.Text(c, ago(h.th.Updated, time.Now())).FontSize(12).TextColor(t.TextMuted).Shrink(0)
		}
		if it.check {
			ui.Icon(c, checkIcon).Size(14, 14).Shrink(0).TextColor(t.TextMuted)
		}
		if chosen {
			ui.Icon(c, enterIcon).Size(14, 14).Shrink(0).TextColor(t.TextMuted)
		}
	})
	if row.Clicked() {
		a.runSearch(it)
	}
	return row.Hovered()
}

// searchEmpty builds what the search shows when it lists nothing: there
// being none of the tab's kind, or none matching.
func (a *app) searchEmpty(c *ui.Context) {
	t := c.Theme()
	icon, title, hint := searchIcon, "No results", "Try other words, or another tab"
	switch {
	case a.searchTab == tabProjects:
		icon, title, hint = folderIcon, "No projects yet", "Projects will show here"
	case a.searchTab == tabArtifacts:
		icon, title, hint = artifactIcon, "No artifacts yet", "Artifacts will show here"
	case strings.TrimSpace(a.query) != "":
		title = "No results for “" + strings.TrimSpace(a.query) + "”"
	}
	ui.Column(c).Height(160).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Gap(4).Padding(0, 24).Children(func() {
		ui.Icon(c, icon).Size(18, 18).TextColor(t.TextMuted).Margin(0, 0, 6, 0)
		ui.Text(c, title).FontWeight(500).SingleLine()
		ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted)
	})
}

// searchKeys builds the row of the keys the search takes, under it, in the
// muted 12 of the app's shortcuts, each key on a wash.
func (a *app) searchKeys(c *ui.Context) {
	t := c.Theme()
	ui.Box(c).Height(1).Shrink(0).Background(t.Border)
	key := func(label string, svg *ui.SVG) {
		k := ui.Row(c).Height(18).MinWidth(18).Padding(0, 5).Radius(4).Background(a.pal.fg.Alpha(0.06)).
			Justify(ui.Center).AlignItems(ui.Center)
		k.Children(func() {
			if svg != nil {
				ui.Icon(c, svg).Size(11, 11).TextColor(t.TextMuted)
			} else {
				ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
			}
		})
	}
	hint := func(label string, keys func()) {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			keys()
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
		})
	}
	ui.Row(c).Shrink(0).Padding(9, 16).Gap(16).AlignItems(ui.Center).Children(func() {
		hint("Close", func() { key("Esc", nil) })
		hint("Change type", func() {
			ui.Row(c).Gap(2).Children(func() {
				key("", arrowLeftIcon)
				key("", arrowRightIcon)
			})
		})
		ui.Spacer(c)
		hint("Compose", func() { key("Tab", nil) })
	})
}
