package main

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

const (
	titleBarHeight = 52  // the height of the hidden title bar's room
	sidebarWidth   = 272 // the sidebar's width while it shows
	maxWidth       = 780 // the conversation's and the composer's width at most
)

// message is one of a chat's: the user's, or the assistant's reply, which
// streams in while streaming is true.
type message struct {
	ID    int    `json:"id"`
	Role  string `json:"role"` // "user" or "assistant"
	Text  string `json:"text"`
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
}

// thread is a chat: its messages, and where its list is scrolled.
type thread struct {
	ID       int        `json:"id"`
	Title    string     `json:"title"`
	Updated  time.Time  `json:"updated"`
	Model    string     `json:"model,omitempty"` // the model asked last
	Messages []*message `json:"messages"`
	list     ui.ListState
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
	win      *mygo.Window // nil in tests, where replies come at once
	prov     provider
	threads  []*thread
	selected string // the ID of the chat shown, "" for a new chat
	next     int    // the last ID given to a chat or a message
	draft    string // what the composer holds
	model    string
	// collapsed hides the sidebar.
	collapsed  bool
	focusDraft bool // gives the composer the focus in the next frame
	// cardHeight is how tall the outline's card was laid out.
	cardHeight float32
	// copied is the message last copied, which shows a check until
	// copiedUntil.
	copied      int
	copiedUntil time.Time
	// lineHeight is how tall a line of text is, measured once.
	lineHeight float32
	pal        *palette // the frame's colors
	md         markdown.Options
	// busy is the reply streaming in, nil when none, and cancel stops it.
	busy   *message
	cancel context.CancelFunc
	// onFinish, when set, runs once as the reply ends (HYPERAI_BENCH).
	onFinish func()
	path     string // where the chats are saved, "" in tests
	saver    saver
	// rev counts the changes to the chats that move them in the sidebar:
	// one made, updated or removed. rows are the sidebar's rows as of
	// rowsRev and rowsDay, so that a frame builds them only when they move.
	rev, rowsRev int
	rowsDay      time.Time
	rows         []chatRow
	chatList     ui.ListState
}

func newApp(p provider) *app {
	return &app{prov: p, model: p.Models()[0]}
}

// current returns the chat shown, nil for a new chat.
func (a *app) current() *thread {
	for _, th := range a.threads {
		if strconv.Itoa(th.ID) == a.selected {
			return th
		}
	}
	return nil
}

// newChat shows an empty chat, which becomes one once its first message is
// sent.
func (a *app) newChat() {
	a.selected, a.focusDraft = "", true
}

// send sends text in the chat shown, starting one when it is new, and
// streams the reply in.
func (a *app) send(text string) {
	text = strings.TrimSpace(text)
	if text == "" || a.busy != nil {
		return
	}
	th := a.current()
	if th == nil {
		a.next++
		th = &thread{ID: a.next, Title: title(text)}
		a.threads = append(a.threads, th)
		a.selected = strconv.Itoa(th.ID)
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
	th.Updated, th.Model = time.Now(), a.model
	a.rev++
	a.draft = ""
	a.busy = reply
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	// finish ends the reply, on the main thread.
	finish := func(err error) {
		cancel()
		reply.end()
		if err != nil && !errors.Is(err, context.Canceled) {
			reply.Error = err.Error()
		}
		a.busy, a.cancel = nil, nil
		th.Updated = time.Now()
		a.rev++
		a.save()
		if f := a.onFinish; f != nil {
			a.onFinish = nil
			f()
		}
	}
	model := a.model
	if a.win == nil {
		finish(a.prov.Stream(ctx, model, turns, func(s string) { reply.Text += s }))
		return
	}
	// The model streams on a goroutine of its own, and the view shows what
	// came through Window.Update, which runs on the main thread in order:
	// in batches, so that tens of chunks a second commit some 20 times.
	go func() {
		batch := newBatcher(func(s string) {
			a.win.Update(func() {
				if reply.streaming {
					reply.Text += s
				}
			})
		})
		err := a.prov.Stream(ctx, model, turns, batch.add)
		batch.close()
		a.win.Update(func() {
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

// stop stops the reply streaming in, keeping what came.
func (a *app) stop() {
	reply := a.busy
	if reply == nil {
		return
	}
	a.cancel()
	reply.end()
	if reply.Text == "" {
		reply.Error = "Stopped."
	}
	a.busy, a.cancel = nil, nil
	a.save()
}

// remove deletes th, stopping its reply.
func (a *app) remove(th *thread) {
	if a.busy != nil && slices.Contains(th.Messages, a.busy) {
		a.stop()
	}
	a.threads = slices.DeleteFunc(a.threads, func(t *thread) bool { return t == th })
	a.rev++
	if a.selected == strconv.Itoa(th.ID) {
		a.selected = ""
	}
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
	// The colors are hyperai-next's, in the system's appearance.
	a.pal = paletteFor(c.Theme())
	c.SetTheme(a.pal.theme)
	a.md.Theme = a.pal.md
	t := c.Theme()
	if c.Shortcut(ui.Cmd, ui.KeyN) {
		a.newChat()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyS) {
		a.collapsed = !a.collapsed
	}
	if a.busy != nil && c.Shortcut(0, ui.KeyEscape) {
		a.stop()
	}
	// The window's material shows wherever the view draws no background:
	// under the sidebar, where the window has one.
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	bar := c.TitleBar()
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	shown := row.Animate("sidebar", b2f(!a.collapsed), 220*time.Millisecond)
	row.Children(func() {
		// The sidebar slides out to the left as it hides.
		w := sidebarWidth * shown
		ui.Row(c).Width(w).Shrink(0).AlignItems(ui.Stretch).Clip().Children(func() {
			side := ui.Column(c).Width(sidebarWidth).Shrink(0).Margin(0, 0, 0, w-sidebarWidth)
			if !vibrant {
				side.Background(a.pal.fg.Alpha(0.035).Over(t.Background))
			}
			side.Children(func() { a.sidebar(c) })
		})
		if w > 0 {
			ui.Box(c).Width(1).Background(t.Border)
		}
		a.pane(c, 20+(1-shown)*(bar.Left+40))
	})

	// The sidebar's toggle, beside the window controls whether the sidebar
	// shows or not.
	label := "Hide sidebar"
	if a.collapsed {
		label = "Show sidebar"
	}
	toggle := iconButton(c.Key("toggle"), sidebarIcon, label+" ("+shortcutText("⇧⌘S", "Ctrl+Shift+S")+")").
		Absolute().Left(bar.Left + 8).Top((titleBarHeight - 28) / 2)
	if toggle.Clicked() {
		a.collapsed = !a.collapsed
	}
}

// sidebar builds, as Waku's does, New chat over the chats by when they
// were last updated, each with its model and age.
func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Box(c).Height(titleBarHeight).Shrink(0).DragWindow()

	if a.navRow(c, "new", newChatIcon, "New chat", shortcutText("⌘N", "Ctrl+N")).Clicked() {
		a.newChat()
	}
	if len(a.threads) == 0 {
		ui.Text(c, "Your chats show here.").FontSize(13).TextColor(t.TextMuted).Padding(16, 20).Grow(1)
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
	}).Grow(1).Padding(6, 0, 12, 0).Label("Chats")
}

// chatRow is a row of the sidebar: a chat, or the title of a group of them
// when th is nil.
type chatRow struct {
	group string
	th    *thread
}

// chatRows returns the sidebar's rows as of now, grouping the chats by day
// again only when they changed, or the day did.
func (a *app) chatRows(now time.Time) []chatRow {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if a.rows != nil && a.rowsRev == a.rev && a.rowsDay.Equal(day) {
		return a.rows
	}
	rows := make([]chatRow, 0, len(a.threads)+5)
	for _, g := range groupByDay(a.threads, now) {
		rows = append(rows, chatRow{group: g.title})
		for _, th := range g.threads {
			rows = append(rows, chatRow{th: th})
		}
	}
	a.rows, a.rowsRev, a.rowsDay = rows, a.rev, day
	return rows
}

// navRow creates a row of the sidebar's that does something: an icon, its
// label, and the shortcut that does it too.
func (a *app) navRow(c *ui.Context, key string, svg *ui.SVG, label, shortcut string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Margin(0, 8).Padding(7, 12).Gap(10).Radius(8).Justify(ui.Start).Label(label)
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
// and how long ago. The chat shown is on a wash of fg, its title in the
// accent.
func (a *app) threadRow(c *ui.Context, th *thread, now time.Time) {
	t := c.Theme()
	id := strconv.Itoa(th.ID)
	selected := id == a.selected
	row := ui.ButtonBase(c.Key(th.ID)).Margin(0, 8).Padding(8, 12).Radius(8).Justify(ui.Start).Label(th.Title)
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
				title.TextColor(a.pal.ac).FontWeight(500)
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, messageIcon).Size(12, 12).TextColor(t.TextMuted)
				model := th.Model
				if model == "" {
					model = "Chat"
				}
				ui.Text(c, model).FontSize(12).TextColor(t.TextMuted).SingleLine().Grow(1)
				ui.Text(c, ago(th.Updated, now)).FontSize(12).TextColor(t.TextMuted)
			})
		})
	})
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item("Delete chat").Chosen() {
			a.remove(th)
		}
	})
	if row.Clicked() {
		a.selected, a.focusDraft = id, true
	}
}

// ago is how long before now t was, shortly: 5m, 3h, 2d, or the date.
func ago(t, now time.Time) string {
	switch d := now.Sub(t); {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	case d < 30*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	}
	return t.Format("Jan 2")
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
	titles := []string{"Today", "Yesterday", "Previous 7 days", "Previous 30 days", "Older"}
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

// iconButton creates a small square button showing an icon, described by
// label.
func iconButton(c *ui.Context, svg *ui.SVG, label string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Size(30, 28).Radius(6).TextColor(t.TextMuted).Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(t.Text.Alpha(0.06)).TextColor(t.Text)
	}
	b.Children(func() { ui.Icon(c, svg).Size(16, 16) })
	return b
}

// shortcutText is mac on macOS and other elsewhere.
func shortcutText(mac, other string) string {
	if runtime.GOOS == "darwin" {
		return mac
	}
	return other
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
