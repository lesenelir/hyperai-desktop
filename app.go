package main

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"

	"hyperai-desktop/locales"
	"hyperai-desktop/ratex"
)

const (
	titleBarHeight = 52  // the height of the hidden title bar's room
	sidebarWidth   = 272 // the sidebar's width at first (layout.go)
	maxWidth       = 780 // the conversation's and the composer's width at most
	// barClear is how far a row of a list that scrolls stays from the
	// list's right, for its wash to clear the scroll bar over it: MyGo's
	// is 6 DIPs wide, 3 in from the edge.
	barClear = 12
	// sidebarTint is how much of the sidebar's own color lies over the
	// desktop's material, where it shows.
	sidebarTint = 0.6
	// rowGap is the room between two rows of such a list, for the washes of
	// one chosen and one under the pointer not to run together.
	rowGap = 2
)

// message is one of a chat's: the user's, or the assistant's reply, which
// streams in while streaming is true.
type message struct {
	ID   int    `json:"id"`
	Role string `json:"role"` // "user" or "assistant"
	Text string `json:"text"`
	// Error is what went wrong, as the provider said it, or errStopped,
	// which shows in the app's language (errorText).
	Error string `json:"error,omitempty"`
	// Elapsed is how many seconds the reply took to write.
	Elapsed float64 `json:"elapsed,omitempty"`
	// Time is when the question was sent, or the reply ended; zero in
	// chats saved before messages kept it.
	Time      time.Time `json:"time,omitzero"`
	streaming bool
	started   time.Time
	preview   string  // the outline's preview of Text, once it is complete
	height    float32 // how tall its row was laid out last
	// A question's lines, as wrapped at wrapWidth; whether it shows whole
	// while it is long; and whether to bring it back into view, collapsed.
	lines     int
	wrapWidth float32
	expanded  bool
	reveal    bool
}

// thread is a chat: its messages, and where its list is scrolled.
type thread struct {
	ID       int        `json:"id"`
	Title    string     `json:"title"`
	Updated  time.Time  `json:"updated"`            // when the user last sent in it, which orders the sidebar
	Model    string     `json:"model,omitempty"`    // the model asked last
	Provider string     `json:"provider,omitempty"` // Model's, "" in chats kept before it
	Draft    string     `json:"draft,omitempty"`    // what its composer held as it was left
	Messages []*message `json:"messages"`
	list     ui.ListState
	// busy is the reply streaming in, nil when none, and cancel stops it:
	// each chat's its own, so that chats stream at once.
	busy   *message
	cancel context.CancelFunc
	// unread tells that a reply ended while the chat was not in view,
	// which the sidebar marks until it is.
	unread bool
	// scroll is where list is scrolled, which the button bringing the end
	// back into view glides.
	scroll ui.ScrollState
	pin    pin
	// boxes are where the last frame laid out the rows, empty for those it
	// did not build, and listBox the list; seek is the row a glide goes
	// to, plus one, 0 for none.
	boxes   []ui.Rect
	listBox ui.Rect
	seek    int
}

// turns returns the conversation for the provider: the messages with text,
// leaving out the reply being written.
func (th *thread) turns() []turn {
	var turns []turn
	for _, m := range th.Messages {
		if m.Text != "" && !m.streaming {
			turns = append(turns, turn{Role: m.Role, Content: m.Text})
		}
	}
	return turns
}

// app is the window's state.
type app struct {
	win      *mygo.Window // nil in tests
	threads  []*thread
	selected string // the ID of the chat shown, "" for a new chat
	next     int    // the last ID given to a chat or a message
	draft    string // what the composer holds, of the chat shown (nav.go)
	newDraft string // what a new chat's composer held as it was left
	model    string // the model asked, of prov
	// post runs f on the main thread, in order, as Window.Update does,
	// for what comes on a goroutine: nil in tests, where replies and the
	// providers' models come at once, but for those that post themselves.
	post func(f func())
	// provs are the providers the composer offers the models of, those
	// with an API key and a model (providers.go), or fallback where none
	// is, which is the demo in tests and HYPERAI_BENCH, and none in the
	// app; prov is the one asked, of model, nil for none, which is the chat's modelFor shows
	// (models.go). keys are the API keys the user gave, by providerKind ID,
	// which keyring keeps (nil in tests); keyError is what reading or
	// keeping them last said, "" where it went well. confs are the
	// providers' other settings, by ID, and defaultModel the model a new
	// chat asks, which confPath keeps ("" in tests); listing tells the
	// providers whose models are being listed, and listErrors why those
	// that could not be were not. The Providers page edits the row editing
	// in editDraft, "" for none.
	provs        []provider
	prov         provider
	fallback     provider
	modelFor     string
	keys         map[string]string
	keyring      keyring
	keyError     string
	confs        map[string]*providerConf
	defaultModel modelRef
	confPath     string
	confSaver    saver
	listing      map[string]bool
	listErrors   map[string]string
	editing      string
	editDraft    string
	needProvider bool // the composer sent while no provider has a key
	// collapsed hides the sidebar, sideWidth wide as it shows; resizing
	// tells that the pointer drags its edge, which pressed it resizeFrom
	// wide and went resizeBy since (layout.go).
	collapsed            bool
	sideWidth            float32
	resizing             bool
	resizeFrom, resizeBy float32
	layoutPath           string // where the layout is kept, "" in tests
	layoutSaver          saver
	focusDraft           bool    // gives the composer the focus in the next frame
	nav                  history // the chats shown before and after the one shown
	// cardHeight is how tall the outline's card was laid out.
	cardHeight float32
	// footer is how tall the composer was laid out, which floats over the
	// conversation's bottom, and field where its field was, scrolled
	// fieldScroll.
	footer      float32
	field       ui.Rect
	fieldScroll ui.ScrollState
	// pill tells whether the composer is a pill this frame (composerPill),
	// as of its box's width and its buttons' when laid out last; measured
	// is the draft whose text is measuredWidth wide.
	pill                 bool
	boxWidth, toolsWidth float32
	measured             string
	measuredWidth        float32
	// copied is the message last copied, which shows a check until
	// copiedUntil.
	copied      int
	copiedUntil time.Time
	pal         *palette // the frame's colors
	md          markdown.Options
	formulas    map[string]*mathBox // math set, by its engine, TeX, size and font (math.go)
	// tex lays math out as KaTeX does (katex.go), once it started: nil
	// until then, and in tests that do not start it. renderMath sets math
	// at all, which General turns off to show its LaTeX.
	tex        *ratex.Engine
	renderMath bool
	// onFinish, when set, runs once as a reply ends (HYPERAI_BENCH).
	onFinish func()
	path     string // where the chats are saved, "" in tests
	saver    saver
	// rev counts the changes to the chats that move them in the sidebar:
	// one made, updated or removed. rows are the sidebar's rows as of
	// rowsRev, rowsDay and rowsLang, the language their groups are titled
	// in, so that a frame builds them only when they move.
	rev, rowsRev int
	rowsDay      time.Time
	rowsLang     string
	rows         []chatRow
	chatList     ui.ListState
	// searching shows the search (search.go), for query on searchTab: hits
	// are the chats it found, as of hitsQuery and hitsRev; searchSel is the
	// row chosen, as of the query searchQuery, and searchHover the one under
	// the pointer, -1 for none.
	searching   bool
	query       string
	searchTab   searchTab
	searchQuery string
	focusSearch bool // gives the search's field the focus in the next frame
	hits        []searchHit
	hitsQuery   string
	hitsRev     int
	searchSel   int
	searchHover int
	searchMoved bool // the keys chose a hit, which comes into view
	// settings shows the settings (settings.go) in place of the chats, on
	// the page settingsPage, which settingsQuery narrows the sidebar to.
	settings      bool
	settingsPage  string
	settingsQuery string
	// The appearance (appearance.go), which its controls bind: the theme,
	// "system", "light" or "dark", which applyTheme gives the app (nil in
	// tests); the size of the conversation's text, of textSizes, which is
	// body DIPs, as mdTheme, the palette mdOf's Markdown, takes it; whether
	// the sidebar lets the desktop's material through; and whether nothing
	// moves, whatever the desktop asks, from motionSettle after it was
	// switched at motionSwitched, but for while motionHeld, the switch
	// could turn. font is the app's font family, ""
	// for the system's, which winTheme, the palette winOf's theme, takes,
	// and textFont the one the conversation was measured in; codeFont is
	// code's, "" for the system's monospaced font; fonts are those
	// installed, which the settings choose of. codeTheme is the key of the
	// code blocks' theme, of codeThemes, as mdTheme has mdCodeTheme.
	theme          string
	applyTheme     func(theme string)
	textSize       int
	body           float32
	mdTheme        *markdown.Theme
	mdOf           *palette
	translucent    bool
	reduceMotion   bool
	motionSwitched time.Time
	motionHeld     bool
	font           string
	textFont       string
	codeFont       string
	codeTheme      string
	mdCodeTheme    string
	fonts          []ui.FontFamily
	winTheme       *ui.Theme
	winOf          *palette
	popUpWidths    map[string]float32 // popUpWidth's, by button, font and what it shows
	// version is the app's, as it was packaged, "" for a development build
	// (about.go).
	version string
	// The General page's (general.go): the language chosen, of languages,
	// "" for the system's, which applyLanguage gives the app (nil in
	// tests); whether the app opens at login, as
	// the system has it, which openAtLogin reads and setOpenAtLogin sets
	// (nil in tests), loginError what setting it last said; the chat shown
	// as the app opens, of launchTitles, the last chat shown being
	// keptChat, as layout.json has it; the key that sends, of sendTitles;
	// and reveal, which shows a file in the file manager (nil in tests).
	language       string
	applyLanguage  func(tag string)
	atLogin        bool
	openAtLogin    func() bool
	setOpenAtLogin func(open bool) error
	loginError     string
	onLaunch       int
	keptChat       string
	sendWith       int
	reveal         func(path string)
	// Whether a reply that ends while the window is in the background
	// shows a notification, which notify shows and focused tells (nil in
	// tests), notifyError why the last could not show; and the chat
	// asked about deleting, all of them where deletingAll, while
	// confirming shows the alert that asks.
	notifyDone  bool
	notify      func(title, body string, clicked func(), failed func(err error))
	focused     func() bool
	notifyError string
	deleting    *thread
	deletingAll bool
	confirming  bool
}

// newApp returns the app, which asks p until a provider has an API key
// (loadKeys): nothing, for nil, until then.
func newApp(p provider) *app {
	a := &app{fallback: p, sideWidth: sidebarWidth, theme: "system",
		textSize: defaultTextSize, body: bodySize, translucent: true, renderMath: true}
	a.setProviders()
	// Math between single dollars too, as models write it within text:
	// $x^2$, but not $5 and $10, which read as money.
	a.md.Extensions = parser.Default | parser.ExtDollarMath
	a.setMath()
	return a
}

// current returns the chat shown, nil for a new chat.
func (a *app) current() *thread { return a.thread(a.selected) }

// thread returns the chat of id, nil for none.
func (a *app) thread(id string) *thread {
	for _, th := range a.threads {
		if strconv.Itoa(th.ID) == id {
			return th
		}
	}
	return nil
}

// newChat shows an empty chat, which becomes one once its first message is
// sent.
func (a *app) newChat() {
	a.open("")
}

// send sends text in the chat shown, starting one when it is new, and
// streams the reply in, while the other chats stream theirs.
func (a *app) send(text string) {
	text = strings.TrimSpace(text)
	th := a.current()
	if text == "" || a.prov == nil || th != nil && th.busy != nil {
		return
	}
	if th == nil {
		a.next++
		th = &thread{ID: a.next, Title: title(text)}
		a.threads = append(a.threads, th)
		// The chat asks the model chosen for it while it was new, and the
		// new chat's draft is what was sent.
		a.selected = strconv.Itoa(th.ID)
		a.modelFor, a.newDraft = a.selected, ""
	}
	a.next++
	th.Messages = append(th.Messages, &message{ID: a.next, Role: "user", Text: text, Time: time.Now()})
	turns := th.turns()
	// The question goes to the top of the conversation, the reply coming
	// in under it.
	th.pinTop(len(th.Messages) - 1)
	a.next++
	reply := &message{ID: a.next, Role: "assistant", streaming: true, started: time.Now()}
	th.Messages = append(th.Messages, reply)
	th.Updated, th.Model, th.Provider = time.Now(), a.model, a.prov.ID()
	a.rev++
	a.draft, th.Draft = "", ""
	ctx, cancel := context.WithCancel(context.Background())
	th.busy, th.cancel = reply, cancel

	// finish ends the reply, on the main thread.
	finish := func(err error) {
		cancel()
		reply.end()
		if err != nil && !errors.Is(err, context.Canceled) {
			reply.Error = err.Error()
		}
		th.busy, th.cancel = nil, nil
		if strconv.Itoa(th.ID) != a.selected || a.settings {
			th.unread = true
		}
		// The chat stays where it is in the sidebar, as of when it was
		// sent in (Updated): a reply that ends after another's moves
		// nothing. The search finds what came.
		a.rev++
		a.save()
		if !errors.Is(err, context.Canceled) {
			a.notifyReply(th, reply)
		}
		if f := a.onFinish; f != nil {
			a.onFinish = nil
			f()
		}
	}
	// The provider and model asked, as of now: showing another chat
	// chooses another for the composer while this one streams.
	prov, model := a.prov, a.model
	if a.post == nil {
		finish(prov.Stream(ctx, model, turns, func(s string) { reply.Text += s }))
		return
	}
	// The model streams on a goroutine of its own, and the view shows what
	// came through Window.Update, which runs on the main thread in order:
	// in batches, so that tens of chunks a second commit some 20 times.
	go func() {
		batch := newBatcher(func(s string) {
			a.post(func() {
				if reply.streaming {
					reply.Text += s
				}
			})
		})
		err := prov.Stream(ctx, model, turns, batch.add)
		batch.close()
		a.post(func() {
			if reply.streaming {
				finish(err)
			}
		})
	}()
}

// end ends m streaming in, noting when, and how long it took.
func (m *message) end() {
	m.streaming = false
	m.Time = time.Now()
	m.Elapsed = m.Time.Sub(m.started).Seconds()
}

// stop stops the reply streaming in th, if any, keeping what came.
func (a *app) stop(th *thread) {
	if th == nil || th.busy == nil {
		return
	}
	reply := th.busy
	th.cancel()
	reply.end()
	if reply.Text == "" {
		reply.Error = errStopped
	}
	th.busy, th.cancel = nil, nil
	a.save()
}

// shownReply returns the reply streaming in the chat shown, nil for none,
// which the composer's button and Escape stop.
func (a *app) shownReply() *message {
	if th := a.current(); th != nil {
		return th.busy
	}
	return nil
}

// errStopped is the Error of a reply stopped before it said anything, as
// chats.json keeps it, whatever the app's language.
const errStopped = "Stopped."

// errorText returns what went wrong with m, as it shows: a stopped reply's
// in the app's language.
func (m *message) errorText() string {
	if m.Error == errStopped {
		return locales.ReplyStopped()
	}
	return m.Error
}

// askDelete asks whether to delete th, which the alert does once it is
// confirmed; all the chats where th is nil.
func (a *app) askDelete(th *thread) {
	a.deleting, a.deletingAll, a.confirming = th, th == nil, true
}

// deleteAlert builds the alert that asks to delete a chat, or all of
// them, while one is asked, and deletes what it asked about once Delete
// is chosen.
func (a *app) deleteAlert(c *ui.Context) {
	if !a.confirming {
		a.deleting, a.deletingAll = nil, false
		return
	}
	title, about := "", locales.DeleteUndone()
	switch {
	case a.deletingAll:
		title, about = locales.DeleteAllTitle(), locales.DeleteAllAbout(len(a.threads))
	case a.deleting != nil:
		title, about = locales.DeleteChatTitle(a.deleting.Title), locales.DeleteChatAbout()
	}
	if ui.AlertDialog(c, &a.confirming, title, about, locales.Cancel(), locales.Delete()) == 1 {
		if a.deletingAll {
			a.removeAll()
		} else if a.deleting != nil {
			a.remove(a.deleting)
		}
	}
	if !a.confirming {
		a.deleting, a.deletingAll = nil, false
	}
}

// removeAll deletes every chat, stopping their replies.
func (a *app) removeAll() {
	for _, th := range a.threads {
		a.stop(th)
	}
	if a.selected != "" {
		a.draft = a.newDraft
	}
	a.threads, a.selected = nil, ""
	a.nav = history{}
	a.rev++
	a.save()
}

// remove deletes th, stopping its reply.
func (a *app) remove(th *thread) {
	a.stop(th)
	a.threads = slices.DeleteFunc(a.threads, func(t *thread) bool { return t == th })
	a.rev++
	id := strconv.Itoa(th.ID)
	if a.selected == id {
		// Its draft goes with it.
		a.selected, a.draft = "", a.newDraft
	}
	a.nav.remove(id, a.selected)
	a.save()
}

// title makes a chat's title of its first message.
func title(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if r := []rune(text); len(r) > 48 {
		return string(r[:48]) + "…"
	}
	return text
}

func (a *app) view(c *ui.Context) {
	a.followChat()
	// The chat in view is read.
	if th := a.current(); th != nil && !a.settings {
		th.unread = false
	}
	// The colors are hyperai-next's, in the system's appearance.
	a.pal = paletteFor(c.Theme())
	c.SetTheme(a.windowTheme())
	a.setText(textSizes[a.textSize], a.font)
	a.md.Theme = a.markdownTheme()
	a.toasts(c)
	held := a.motionHeld
	a.motionHeld = false // until the switch says so again
	if a.reduceMotion && !held && c.Now().Sub(a.motionSwitched) >= motionSettle {
		c.DisableAnimations()
	}
	t := c.Theme()
	if c.Shortcut(ui.Cmd, ui.KeyN) {
		a.newChat()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyS) {
		a.collapsed = !a.collapsed
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		a.toggleSearch()
	}
	if c.Shortcut(ui.Cmd, ui.KeyComma) {
		if a.settings {
			a.closeSettings()
		} else {
			a.openSettings()
		}
	}
	a.historyKeys(c)
	// The chat shown, kept for the next start (General's On launch).
	if a.selected != a.keptChat {
		a.keptChat = a.selected
		a.saveLayout()
	}
	// Once a frame, for the composer and the room the conversation leaves it.
	th := a.current()
	a.pill = a.composerPill(c, th == nil || len(th.Messages) == 0)
	switch {
	case a.confirming:
		// Escape is the alert's, which it cancels.
	case a.settings && !a.searching && c.Shortcut(0, ui.KeyEscape):
		a.closeSettings()
	case a.shownReply() != nil && c.Shortcut(0, ui.KeyEscape):
		a.stop(a.current())
	}
	// The window's material shows wherever the view draws no background:
	// under the sidebar, where the window has one.
	vibrant := c.Vibrancy() && a.translucent
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	bar := c.TitleBar()
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	shown := row.Animate("sidebar", b2f(!a.collapsed), 220*time.Millisecond)
	winW, _ := c.Size()
	lo, hi := sidebarBounds(winW, bar.Left)
	sideW := max(lo, min(a.sideWidth, hi))
	row.Children(func() {
		// The sidebar slides out to the left as it hides.
		w := sideW * shown
		ui.Row(c).Width(w).Shrink(0).AlignItems(ui.Stretch).Clip().Children(func() {
			side := ui.Column(c).Width(sideW).Shrink(0).Margin(0, 0, 0, w-sideW)
			// Over the material, its own color, warm as the canvas beside it,
			// lets the material through but tints it: the desktop's grey,
			// cool by itself, sat apart from the canvas.
			fill := a.pal.fg.Alpha(0.035).Over(t.Background)
			if vibrant {
				fill = fill.Alpha(sidebarTint)
			}
			side.Background(fill)
			side.Children(func() { a.sidebar(c) })
		})
		if w > 0 {
			ui.Box(c).Width(1).Background(t.Border)
		}
		// Clear of the toggle, Back and Forward as the sidebar hides.
		if left := 20 + (1-shown)*(bar.Left+108); a.settings {
			a.settingsPane(c, left)
		} else {
			a.pane(c, left)
		}
		if shown == 1 {
			a.resizer(c, w, lo, hi)
		}
	})

	// The sidebar's toggle, beside the window controls whether the sidebar
	// shows or not.
	label := locales.HideSidebar()
	if a.collapsed {
		label = locales.ShowSidebar()
	}
	toggle := iconButtonSized(c.Key("toggle"), sidebarIcon, locales.WithKeys(label, sidebarKeys.String()), titleIconSize).
		Absolute().Left(bar.Left + 8).Top((titleBarHeight - 28) / 2)
	if toggle.Clicked() {
		a.collapsed = !a.collapsed
	}
	a.historyButtons(c, bar.Left+8+30+6)
	a.searchPalette(c)
	// Over all of it, the alert asking to delete.
	a.deleteAlert(c)
}

// sidebar builds, as Waku's does, New chat and Search over the chats by
// when they were last updated, each with its model and age, and Settings
// at its foot; or the settings' own, while they show.
func (a *app) sidebar(c *ui.Context) {
	if a.settings {
		a.settingsSidebar(c)
		return
	}
	t := c.Theme()
	ui.Box(c).Height(titleBarHeight).Shrink(0).DragWindow()

	if a.navRow(c, "new", newChatIcon, locales.NewChat(), newChatKeys.String()).Clicked() {
		a.newChat()
	}
	if a.navRow(c, "search", searchIcon, locales.Search(), findKeys.String()).Clicked() {
		a.toggleSearch()
	}
	if len(a.threads) == 0 {
		ui.Text(c, locales.SidebarEmpty()).FontSize(13).TextColor(t.TextMuted).Padding(16, 20).Grow(1)
		a.settingsRow(c)
		return
	}
	// A list, so that a frame builds only the rows that show: every frame of
	// a streaming reply builds the sidebar too.
	now := time.Now()
	rows := a.chatRows(now)
	a.chatList.Key = func(i int) any {
		if th := rows[i].th; th != nil {
			return th.ID
		}
		return "group " + rows[i].group
	}
	ui.List(c.Key("chats"), &a.chatList, len(rows), func(i int) {
		if th := rows[i].th; th != nil {
			a.threadRow(c, th, now)
		} else {
			ui.Text(c, rows[i].group).FontSize(12).TextColor(t.TextMuted).Padding(14, 20, 6, 20)
		}
	}).Grow(1).MinHeight(0).Padding(6, 0, 12, 0).Label(locales.SidebarChats())
	a.settingsRow(c)
}

// chatRow is a row of the sidebar: a chat, or the title of a group of them
// when th is nil.
type chatRow struct {
	group string
	th    *thread
}

// chatRows returns the sidebar's rows as of now, grouping the chats by day
// again only when they changed, or the day or the language did.
func (a *app) chatRows(now time.Time) []chatRow {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	lang := i18n.Locale()
	if a.rows != nil && a.rowsRev == a.rev && a.rowsDay.Equal(day) && a.rowsLang == lang {
		return a.rows
	}
	rows := make([]chatRow, 0, len(a.threads)+5)
	for _, g := range groupByDay(a.threads, now) {
		rows = append(rows, chatRow{group: g.title})
		for _, th := range g.threads {
			rows = append(rows, chatRow{th: th})
		}
	}
	a.rows, a.rowsRev, a.rowsDay, a.rowsLang = rows, a.rev, day, lang
	return rows
}

// navRow creates a row of the sidebar's that does something: an icon, its
// label, and the shortcut that does it too.
func (a *app) navRow(c *ui.Context, key string, svg *ui.SVG, label, shortcut string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Margin(rowGap/2, barClear, rowGap/2, 8).Padding(7, 12).Gap(10).Radius(8).Justify(ui.Start).Label(label)
	if b.Hovered() {
		b.Background(a.pal.fg.Alpha(0.05))
	}
	b.Children(func() {
		ui.Icon(c, svg).Size(16, 16).TextColor(t.TextMuted)
		ui.Text(c, label).Grow(1)
		ui.Text(c, shortcut).FontSize(12).TextColor(t.TextMuted)
	})
	return b
}

// threadRow builds th's row: its title, and under it the model it asked
// and how long ago, or a spinner as a reply streams in, the mark of one
// that ended out of view before it. The chat shown is on a wash of fg, its title at 500,
// as the settings' pages are: the wash says it, without the accent too.
func (a *app) threadRow(c *ui.Context, th *thread, now time.Time) {
	t := c.Theme()
	id := strconv.Itoa(th.ID)
	selected := id == a.selected
	row := ui.ButtonBase(c.Key(th.ID)).Margin(rowGap/2, barClear, rowGap/2, 8).Padding(8, 12).Radius(8).Justify(ui.Start).Label(th.Title)
	switch {
	case selected:
		row.Background(a.pal.fg.Alpha(0.07))
	case row.Hovered():
		row.Background(a.pal.fg.Alpha(0.04))
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			title := ui.Text(c, th.Title).SingleLine()
			if selected {
				title.FontWeight(500)
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, messageIcon).Size(12, 12).TextColor(t.TextMuted)
				model := th.Model
				if model == "" {
					model = locales.SidebarModelNone()
				}
				ui.Text(c, model).FontSize(12).TextColor(t.TextMuted).SingleLine().Grow(1)
				// A reply streaming in, in place of the age; one that ended
				// out of view, marked before it.
				if th.busy != nil {
					ui.Spinner(c).Size(12, 12).Label(locales.SidebarReplying())
					return
				}
				if th.unread {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(a.pal.ac).Label(locales.SidebarUnread())
				}
				ui.Text(c, ago(th.Updated, now)).FontSize(12).TextColor(t.TextMuted)
			})
		})
	})
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.DeleteChatItem()).Chosen() {
			a.askDelete(th)
		}
	})
	if row.Clicked() {
		a.open(id)
	}
}

// ago is how long before now t was, shortly, in the app's language: 5m,
// 3h, 2d, or the date.
func ago(t, now time.Time) string {
	switch d := now.Sub(t); {
	case d < time.Minute:
		return locales.AgoNow()
	case d < time.Hour:
		return locales.AgoMinutes(int(d.Minutes()))
	case d < 24*time.Hour:
		return locales.AgoHours(int(d.Hours()))
	case d < 30*24*time.Hour:
		return locales.AgoDays(int(d.Hours() / 24))
	}
	return i18n.FormatDate(i18n.Locale(), t, i18n.DateOptions{Skeleton: "MMMd"})
}

// group is chats updated on the same day, or within the same span of days.
type group struct {
	title   string
	threads []*thread
}

// groupByDay groups threads as of now, the latest first.
func groupByDay(threads []*thread, now time.Time) []group {
	sorted := slices.Clone(threads)
	slices.SortStableFunc(sorted, func(x, y *thread) int { return y.Updated.Compare(x.Updated) })
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	// Each span holds the chats updated since its start, and not since the
	// start of the span before it.
	titles := []string{locales.GroupToday(), locales.GroupYesterday(), locales.GroupWeek(), locales.GroupMonth(), locales.GroupOlder()}
	starts := []time.Time{today, today.AddDate(0, 0, -1), today.AddDate(0, 0, -7), today.AddDate(0, 0, -30), {}}
	var groups []group
	for _, th := range sorted {
		i := slices.IndexFunc(starts, func(s time.Time) bool { return !th.Updated.Before(s) })
		if n := len(groups); n == 0 || groups[n-1].title != titles[i] {
			groups = append(groups, group{title: titles[i]})
		}
		groups[len(groups)-1].threads = append(groups[len(groups)-1].threads, th)
	}
	return groups
}

// placeholder builds s over the empty field built just before it, at top,
// left in their parent, in its size and line height (none for the font's):
// in fg at 40%, as hyperai-next's inputs show theirs (docs/design.md),
// where MyGo's own takes the muted text's 70%, which reads as text. It
// shows while show: the field empty, and no input method composing in it.
func (a *app) placeholder(c *ui.Context, s string, show bool, top, left, size, lineHeight float32) {
	if !show {
		return
	}
	p := ui.Text(c, s).Absolute().Top(top).Left(left).FontSize(size).TextColor(a.pal.fg.Alpha(0.4)).
		SingleLine().PassThrough()
	if lineHeight > 0 {
		p.LineHeight(lineHeight)
	}
}

// toasts builds the window's toasts as the app's cards look, rather than
// turned over as the theme's tooltips are: the composer's face, a hairline
// round it and its shadow, the action in the accent. They rise a little as
// they come, and fade as they go.
func (a *app) toasts(c *ui.Context) {
	t := c.Theme()
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(0, 24, 24, 24).AlignItems(ui.Center).Gap(8)
		for _, ts := range toasts {
			toast := ui.ToastBase(c, ts)
			toast.Root.Row().AlignItems(ui.Center).Gap(16).Padding(10, 10, 10, 16).Radius(12).
				Background(a.pal.composer).Border(1, t.Border).Shadow(0, 8, 24, -6, a.pal.shadow(0.18)).
				TextColor(t.Text).Transition(toastTransition)
			toast.Root.Children(func() {
				if ts.Description == "" {
					ui.Text(c, ts.Title).FontSize(13)
				} else {
					ui.Column(c).Shrink(1).Gap(2).Children(func() {
						ui.Text(c, ts.Title).FontSize(13).FontWeight(500)
						ui.Text(c, ts.Description).FontSize(12).LineHeight(1.5).TextColor(t.TextMuted)
					})
				}
				if ts.Action == "" {
					return
				}
				b := toast.ActionButton().Padding(5, 10).Radius(8).TextColor(a.pal.ac).Label(ts.Action)
				if b.Hovered() {
					b.Background(a.pal.ac.Alpha(0.08))
				}
				b.Children(func() { ui.Text(c, ts.Action).FontSize(13).FontWeight(500).SingleLine() })
			})
		}
	})
}

// tooltip shows s by e as the pointer rests on it, as Element.Tooltip
// does, but as the app's toasts look rather than turned over: the cards'
// face, a hairline round it and their shadow. It returns e.
func tooltip(c *ui.Context, e ui.Element, s string) ui.Element {
	t, p := c.Theme(), paletteFor(c.Theme())
	ui.TooltipBase(c, e, func(tip ui.Element) {
		tip.MaxWidth(320).Padding(5, 8).Radius(6).Background(p.composer).Border(1, t.Border).
			TextColor(t.Text).FontSize(12).Shadow(0, 4, 12, -4, p.shadow(0.16))
		tip.Children(func() { ui.Text(c, s) })
		tip.AttachTo(e, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 6, 0)
	})
	return e
}

// toastTransition brings a toast in from a little under its place, and
// fades it out where it is.
var toastTransition = ui.ElementTransition{Enter: &ui.Motion{Y: 8}, Exit: &ui.Motion{}}

// iconButton creates a small square button showing an icon, described by
// label.
func iconButton(c *ui.Context, svg *ui.SVG, label string) ui.Element {
	return iconButtonSized(c, svg, label, 16)
}

// iconButtonSized is iconButton with an icon size DIPs square.
func iconButtonSized(c *ui.Context, svg *ui.SVG, label string, size float32) ui.Element {
	t := c.Theme()
	b := tooltip(c, ui.ButtonBase(c).Size(30, 28).Radius(6).TextColor(t.TextMuted).Label(label), label)
	if b.Hovered() {
		b.Background(t.Text.Alpha(0.06)).TextColor(t.Text)
	}
	b.Children(func() { ui.Icon(c, svg).Size(size, size) })
	return b
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
