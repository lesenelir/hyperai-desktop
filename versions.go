package main

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// A chat's messages are a tree, as ChatGPT's and Claude's are: each knows
// the one it follows (Parent), 0 for the first, and a question edited or a
// reply asked for again (resend, regenerate) is a version beside the one
// it replaces, which keeps all that followed it. The chat shows one path
// down the tree, Messages, which is what the provider is asked, the search
// finds, the outline marks and Copy as Markdown copies; the other versions
// are kept in Others. A message with versions shows ‹ 2 / 3 ›, which goes
// down another (showVersion); each remembers the one shown after it last
// (Next), so that a version gone back to shows as it was left.
//
// The buttons that change the path change it in OnClick, once the view is
// built, and not as Clicked tells it while the conversation's list builds
// its rows: a path that grows shorter there left the list indexing past
// its end, as it built the rows after the one clicked.

// all returns th's messages of every version: the path shown, then the
// others.
func (th *thread) all() []*message {
	return append(slices.Clone(th.Messages), th.Others...)
}

// link gives the messages of a chat kept before versions the ones they
// follow, each the one before it, and marks the path shown.
func (th *thread) link() {
	for i := 1; i < len(th.Messages); i++ {
		if th.Messages[i].Parent == "" {
			th.Messages[i].Parent = th.Messages[i-1].ID
		}
	}
	th.markPath()
}

// markPath has each message of the path shown remember the one after it.
func (th *thread) markPath() {
	for i := 0; i+1 < len(th.Messages); i++ {
		th.Messages[i].Next = th.Messages[i+1].ID
	}
}

// children returns the messages that follow the message id, 0 for the
// first, oldest first.
func (th *thread) children(id ID) []*message {
	var out []*message
	for _, m := range th.all() {
		if m.Parent == id {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(x, y *message) int { return cmp.Compare(x.ID, y.ID) })
	return out
}

// versions returns the versions of m, itself among them, oldest first.
func (th *thread) versions(m *message) []*message { return th.children(m.Parent) }

// descend returns the path down from m to its end: m, then the one shown
// after it last, else the latest.
func (th *thread) descend(m *message) []*message {
	path := []*message{m}
	for {
		kids := th.children(m.ID)
		if len(kids) == 0 {
			return path
		}
		next := kids[len(kids)-1]
		if i := slices.IndexFunc(kids, func(k *message) bool { return k.ID == m.Next }); i >= 0 {
			next = kids[i]
		}
		path, m = append(path, next), next
	}
}

// cut ends th's path before its message at i, which, with those after it,
// goes to the other versions.
func (th *thread) cut(i int) {
	th.Others = append(th.Others, th.Messages[i:]...)
	th.Messages = slices.Clip(th.Messages[:i])
}

// showVersion shows v in place of th's message at i, a version of it, and
// the path down from it.
func (th *thread) showVersion(i int, v *message) {
	path := th.descend(v)
	th.cut(i)
	th.Others = slices.DeleteFunc(th.Others, func(m *message) bool { return slices.Contains(path, m) })
	th.Messages = append(th.Messages, path...)
	th.markPath()
}

// canChange reports whether th's conversation can change now: no reply
// streams in it.
func canChange(th *thread) bool { return th != nil && th.busy == nil }

// regenerate asks again for th's reply at i, a new version beside it, of
// the model the composer asks; the one it replaces, and what followed it,
// is kept. Where no provider can be asked, the composer says so.
func (a *app) regenerate(th *thread, i int) {
	if !canChange(th) || i < 1 || i >= len(th.Messages) || th.Messages[i].Role != "assistant" {
		return
	}
	if a.prov == nil {
		a.needProvider = true
		return
	}
	a.endEdit()
	th.cut(i)
	a.ask(th)
}

// resend sends text in place of th's question at i, a new version of it
// with its files, and asks for its reply; the question it replaces, and
// what followed it, is kept. Nothing for text that is empty and no files.
func (a *app) resend(th *thread, i int, text string) {
	text = strings.TrimSpace(text)
	if !canChange(th) || i >= len(th.Messages) || th.Messages[i].Role != "user" {
		return
	}
	old := th.Messages[i]
	if text == "" && len(old.Files) == 0 {
		return
	}
	if a.prov == nil {
		a.needProvider = true
		return
	}
	a.endEdit()
	th.cut(i)
	th.Messages = append(th.Messages, &message{ID: newID(), Parent: old.Parent, Role: "user", Text: text, Files: old.Files, Time: time.Now()})
	a.ask(th)
}

// showVersion shows v in place of th's message at i, as its switcher's
// buttons do, and the conversation as it went from it, which the search
// finds from then on and the chats keep.
func (a *app) showVersion(th *thread, i int, v *message) {
	if !canChange(th) {
		return
	}
	a.endEdit()
	th.showVersion(i, v)
	th.pin = pin{}
	a.rev++
	a.save()
}

// versionSwitcher builds, where th's message at i has versions, the
// buttons that show the one before it and after, between which of how many
// it is: ‹ 2 / 3 ›, faded where there is none, and while a reply streams.
func (a *app) versionSwitcher(c *ui.Context, th *thread, i int) ui.Element {
	m := th.Messages[i]
	vs := th.versions(m)
	if len(vs) < 2 {
		return ui.Element{}
	}
	k := slices.Index(vs, m)
	busy := !canChange(th)
	row := ui.Row(c.Key("versions")).AlignItems(ui.Center).Gap(barGap).Label(locales.VersionOf(k+1, len(vs)))
	row.Children(func() {
		step := func(key string, svg *ui.SVG, label string, to int) {
			can := to >= 0 && to < len(vs) && !busy
			b := iconButtonSized(c.Key(key), svg, label, switchIcon).Size(switchWidth, actionSize).Disabled(!can)
			if !can {
				b.Opacity(0.35)
			}
			if can {
				b.OnClick(func() { a.showVersion(th, i, vs[to]) })
			}
		}
		step("previous", chevronLeftIcon, locales.PreviousVersion(), k-1)
		ui.Text(c, strconv.Itoa(k+1)+" / "+strconv.Itoa(len(vs))).FontSize(12).Padding(0, 2).
			TextColor(c.Theme().TextMuted).FontFeatures("tnum").SingleLine()
		step("next", chevronRightIcon, locales.NextVersion(), k+1)
	})
	return row
}

// A message's actions — copy, edit, regenerate and the switcher's arrows —
// are icon buttons actionSize tall, alike in face and wash, barGap apart
// in a row, so that the space between two icons is about the same
// throughout: the actions' actionSize square, their icons actionIcon; the
// chevrons switchIcon, as small as the count's text, in buttons
// switchWidth wide, their wash close round them. actionInset and
// switchInset are how far an icon is in from its button's edge, which a
// row of them goes out by to put the icon on the message's.
const (
	actionSize  = 24
	actionIcon  = 14
	switchWidth = 18
	switchIcon  = 12
	barGap      = 2
	actionInset = (actionSize - actionIcon) / 2
	switchInset = (switchWidth - switchIcon) / 2
)

// actionButton builds an action button of a message: svg, size DIPs,
// described by label.
func actionButton(c *ui.Context, svg *ui.SVG, label string, size float32) ui.Element {
	return iconButtonSized(c, svg, label, size).Size(actionSize, actionSize)
}

// barInset returns how far the row of th's message at i's actions goes
// out past the message, for its outer icon to be on its edge: the
// switcher's chevron, where it has versions, else an action's icon.
func barInset(th *thread, i int) float32 {
	if len(th.versions(th.Messages[i])) > 1 {
		return switchInset
	}
	return actionInset
}

// regenerateButton builds the button under th's reply at i that asks for
// it again (regenerate), faded while a reply streams in the chat.
func (a *app) regenerateButton(c *ui.Context, th *thread, i int) {
	b := actionButton(c.Key("regenerate"), rotateIcon, locales.Regenerate(), actionIcon).Disabled(!canChange(th))
	if !canChange(th) {
		b.Opacity(0.35)
	}
	b.OnClick(func() { a.regenerate(th, i) })
}

// editButton builds the button beside th's question m that turns its
// bubble into a field, to send it again changed (editQuestion).
func (a *app) editButton(c *ui.Context, th *thread, m *message) {
	b := actionButton(c.Key("edit"), pencilIcon, locales.EditMessage(), actionIcon).Disabled(!canChange(th))
	if !canChange(th) {
		b.Opacity(0.35)
	}
	b.OnClick(func() { a.editingMsg, a.editText, a.editStart = m.ID, m.Text, true })
}

// endEdit ends the editing of a question, keeping nothing of it.
func (a *app) endEdit() { a.editingMsg, a.editText = "", "" }

// editQuestion builds th's question at i as it is edited: its bubble across
// the column, the field in it holding its text, the caret at its end, and
// Cancel and Send under it. The key that sends sends it (app.sends), as
// the composer's does, a new version of it (resend); Escape cancels.
func (a *app) editQuestion(c *ui.Context, th *thread, i int) {
	m := th.Messages[i]
	ready := strings.TrimSpace(a.editText) != "" || len(m.Files) > 0
	ui.Column(c).Padding(bubblePadY, bubblePadX).Radius(14).Background(a.pal.raised).Gap(10).AlignItems(ui.Stretch).
		Children(func() {
			field := ui.TextAreaBase(c.Key("edit "+string(m.ID)), &a.editText).WidthPercent(100).Lines(1, 12).
				FontSize(a.body).LineHeight(bodyLineHeight).Label(locales.EditMessage()).AutoFocus().
				OnTextKey(func(k ui.TextKey) bool {
					switch {
					case k.Key == ui.KeyEscape:
						a.endEdit()
					case a.sends(k.Key, k.Mods):
						if strings.TrimSpace(k.Text()) != "" || len(m.Files) > 0 {
							a.resend(th, i, k.Text())
						}
					default:
						return false
					}
					return true
				})
			if a.editStart {
				n := utf8.RuneCountInString(a.editText)
				field.Focus().SetTextSelection(n, n)
				a.editStart = false
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				a.textButton(c, "cancel edit", locales.Cancel(), false, false, a.endEdit)
				a.textButton(c, "send edit", locales.Send(), true, !ready, func() { a.resend(th, i, a.editText) })
			})
		})
}

// textButton builds a button that says label, keyed key: the accent's
// where primary, as the composer's send button is, else the composer's
// face in a hairline; faded where disabled. A click runs clicked, once the
// view is built.
func (a *app) textButton(c *ui.Context, key, label string, primary, disabled bool, clicked func()) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Padding(5, 12).Radius(8).Label(label).Disabled(disabled)
	switch {
	case primary && disabled:
		b.Background(a.pal.fg.Alpha(0.08)).TextColor(a.pal.fg.Alpha(0.35))
	case primary:
		b.Background(a.pal.ac).TextColor(a.pal.bg)
	default:
		b.Background(a.pal.composer).Border(1, t.Border).TextColor(t.Text)
		if b.Hovered() {
			b.Background(a.pal.fg.Alpha(0.04).Over(a.pal.composer))
		}
	}
	b.Children(func() { ui.Text(c, label).FontSize(13).FontWeight(500).SingleLine() })
	b.OnClick(clicked)
}
