package main

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// suggestions are what an empty chat offers to ask.
var suggestions = []struct{ title, prompt string }{
	{"Write a function", "Write an LRU cache in Go that is safe for concurrent use."},
	{"Plan a change", "Plan how to add streaming replies to a chat app."},
	{"Explain a concept", "Explain how Go's scheduler runs goroutines."},
	{"Review my setup", "How do I connect this app to a real model?"},
}

// pane builds the chat shown: its title bar, its messages or the welcome,
// and the composer. titleLeft is the room left of the title, which grows
// to clear the window controls as the sidebar hides.
func (a *app) pane(c *ui.Context, titleLeft float32) {
	t := c.Theme()
	bar := c.TitleBar()
	th := a.current()
	ui.Column(c).Grow(1).MinWidth(0).Background(t.Background).Children(func() {
		ui.Row(c).Height(titleBarHeight).Shrink(0).AlignItems(ui.Center).Padding(0, bar.Right+12, 0, titleLeft).Children(func() {
			name := "New chat"
			if th != nil {
				name = th.Title
			}
			// The title drags the window; the menu beside it does not.
			ui.Row(c).Grow(1).MinWidth(0).Height(titleBarHeight).AlignItems(ui.Center).DragWindow().Children(func() {
				ui.Text(c, name).FontSize(14).FontWeight(500).SingleLine()
			})
			if th != nil {
				iconButton(c, moreIcon, "More").Menu(func(m *ui.Menu) {
					if m.Item("Copy as Markdown").Chosen() {
						c.Services().WriteClipboard(transcript(th))
						c.Toast("Copied")
					}
					m.Separator()
					if m.Item("Delete chat").Chosen() {
						a.remove(th)
					}
				})
			}
		})
		if th == nil || len(th.Messages) == 0 {
			a.welcome(c)
		} else {
			a.conversation(c, th)
		}
		a.composer(c)
	})
}

// welcome builds an empty chat: a greeting, and suggestions to ask.
func (a *app) welcome(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(8).Padding(24).Children(func() {
		ui.Icon(c, logoIcon).Size(34, 34).TextColor(a.pal.ac)
		ui.Text(c, "What should we work on?").FontSize(26).FontWeight(600).Margin(14, 0, 0, 0)
		ui.Text(c, "Ask anything, or start with one of these.").TextColor(t.TextMuted)
		ui.Row(c).Wrap().Gap(10).Justify(ui.Center).MaxWidth(maxWidth).Margin(22, 0, 0, 0).Children(func() {
			for _, s := range suggestions {
				// Cards rest flat on a hairline, and lift on hover.
				card := ui.ButtonBase(c.Key(s.title)).Width(300).Padding(12, 14).Radius(12).
					Background(a.pal.composer).Border(1, t.Border).Justify(ui.Start).Label(s.title)
				if card.Hovered() {
					card.Shadow(0, 8, 24, -6, a.pal.shadow(0.18))
				}
				card.Children(func() {
					ui.Column(c).Gap(4).MinWidth(0).Children(func() {
						ui.Text(c, s.title).FontSize(13).FontWeight(600)
						ui.Text(c, s.prompt).FontSize(12).TextColor(t.TextMuted).MaxLines(2)
					})
				})
				if card.Clicked() {
					a.send(s.prompt)
				}
			}
		})
	})
}

// conversation builds th's messages down the middle, the question last
// sent at the top while its reply fits under it (pin), and following the
// end otherwise until the user scrolls up.
func (a *app) conversation(c *ui.Context, th *thread) {
	th.settle()
	n := len(th.Messages)
	q := th.pin.row(th.Messages)
	room, outgrown := float32(0), false
	if q >= 0 {
		_, h := c.Size()
		room, outgrown = th.measure(q, th.scroll.Y, h, th.listBox)
		if outgrown {
			// The reply no longer fits under the question: follow its end.
			th.pin, q = pin{}, -1
			th.list.ScrollToEnd()
		}
	}
	rows := n
	if room > 0 {
		rows++ // the room after the last row
	}
	th.list.Key = func(i int) any {
		if i == n {
			return "room"
		}
		return th.Messages[i].ID
	}
	// It jumps to the end rather than gliding: a reply grows all the time
	// it streams, so a glide never ends, and draws every refresh for the
	// whole reply (docs/performance.md).
	th.list.FollowEnd = q < 0
	area := ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
	area.Children(func() {
		th.boxes = slices.Grow(th.boxes[:0], n)[:n]
		clear(th.boxes)
		// A list per chat, so that each keeps where it is scrolled. The list
		// is as wide as the pane, so that it scrolls wherever the pointer is.
		list := ui.List(c.Key(th.ID), &th.list, rows, func(i int) {
			if i == n {
				ui.Box(c).Height(room)
				return
			}
			row := ui.Row(c).Justify(ui.Center)
			if b := row.Bounds(); !empty(b) {
				th.boxes[i], th.Messages[i].height = b, b.H
			}
			row.Children(func() {
				ui.Column(c).Grow(1).MaxWidth(maxWidth).Children(func() { a.message(c, th.Messages[i]) })
			})
		}).Grow(1).Padding(listPadTop, 24, listPadBottom, 24).Gap(listGap).TrackScroll(&th.scroll)
		th.listBox = list.Bounds()
		a.outline(c, th, area.Bounds())
		// Not while the question is held at the top, which is the end as far
		// as the user sees, nor as the list goes to its end as the reply
		// outgrows it; gone at once as it is clicked, as the list glides.
		if !th.list.AtEnd() && !th.scroll.Scrolling() && !outgrown && !(q >= 0 && th.pin.held(th.scroll.Y)) {
			a.scrollToEnd(c, th)
		}
	})
}

// scrollToEnd builds the button that brings th's end back into view, which
// shows while the user has scrolled away from it, above the composer, as
// hyperai-next's agent does: round, on bg, lifted by a shadow, and a
// little more under the pointer. A click glides the list to its end, as a
// browser's smooth scroll does, where it follows the reply again.
func (a *app) scrollToEnd(c *ui.Context, th *thread) {
	const size = 32
	t := c.Theme()
	b := ui.ButtonBase(c.Key("scroll to end")).Size(size, size).Radius(size/2).
		Background(a.pal.composer).Border(1, t.Border).TextColor(t.Text).
		Label("Scroll to end").Tooltip("Scroll to end").
		Absolute().LeftPercent(50).Bottom(16)
	if b.Hovered() {
		b.Translate(-size/2, -4).Shadow(0, 8, 16, -4, a.pal.shadow(0.22))
	} else {
		b.Translate(-size/2, 0).Shadow(0, 4, 6, -1, a.pal.shadow(0.16))
	}
	b.Children(func() { ui.Icon(c, arrowDownIcon).Size(16, 16) })
	if b.Clicked() {
		th.scroll.ScrollTo(0, math.MaxFloat32, ui.Smooth)
	}
}

// message builds a message: the user's in a bubble on the right, the
// assistant's as Markdown across the column, with its actions once done.
func (a *app) message(c *ui.Context, m *message) {
	t := c.Theme()
	if m.Role == "user" {
		ui.Row(c).Children(func() {
			ui.Spacer(c)
			ui.Text(c, m.Text).Selectable().Padding(9, 14).Radius(14).Background(a.pal.raised).MaxWidth(560)
		})
		return
	}
	ui.Column(c).Gap(10).AlignItems(ui.Stretch).Children(func() {
		if m.streaming || m.Elapsed > 0 {
			worked(c, m)
		}
		switch {
		case m.streaming && m.Text == "":
		case m.streaming:
			markdown.Stream(c, m.Text, &a.md)
		case m.Text != "":
			markdown.View(c, m.Text, &a.md)
		}
		if m.Error != "" {
			ui.Text(c, m.Error).FontSize(13).TextColor(t.Danger).Selectable()
		}
		if !m.streaming && m.Text != "" {
			ui.Row(c).Margin(-4, 0, 0, -6).Children(func() {
				if iconButton(c, copyIcon, "Copy").Clicked() {
					c.Services().WriteClipboard(m.Text)
					c.Toast("Copied")
				}
			})
		}
	})
}

// worked builds the rule over a reply, as Waku's: while it is written,
// that it is, and then how long it took.
func worked(c *ui.Context, m *message) {
	t := c.Theme()
	rule := func() { ui.Box(c).Height(1).Grow(1).Background(t.Border) }
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		rule()
		if m.streaming {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, "Thinking…").FontSize(12).TextColor(t.TextMuted)
		} else {
			ui.Text(c, "Worked for "+seconds(m.Elapsed)).FontSize(12).TextColor(t.TextMuted)
		}
		rule()
	})
}

// seconds is a duration of s seconds, shortly: 4s, 2m 5s.
func seconds(s float64) string {
	n := max(1, int(s+0.5))
	if n < 60 {
		return strconv.Itoa(n) + "s"
	}
	return strconv.Itoa(n/60) + "m " + strconv.Itoa(n%60) + "s"
}

// composer builds the field to write in, with the model to ask and the
// button that sends, or stops the reply.
func (a *app) composer(c *ui.Context) {
	t := c.Theme()
	ready := strings.TrimSpace(a.draft) != ""
	ui.Column(c).Shrink(0).AlignItems(ui.Center).Padding(0, 24, 14, 24).Gap(8).Children(func() {
		box := ui.Column(c).WidthPercent(100).MaxWidth(maxWidth).Radius(16).Border(1, t.Border).
			Background(a.pal.composer).Shadow(0, 4, 16, -4, a.pal.shadow(0.12))
		box.Children(func() {
			// Enter sends, Shift+Enter breaks the line.
			draft := ui.TextAreaBase(c.Key("draft"), &a.draft).Lines(1, 10).Padding(14, 16, 6, 16).
				Placeholder("Ask anything…").Label("Message").AutoFocus().
				OnTextKey(func(k ui.TextKey) bool {
					if k.Key != ui.KeyEnter || k.Mods != 0 {
						return false
					}
					if a.busy == nil && strings.TrimSpace(k.Text()) != "" {
						a.send(k.Text())
						k.SetText("")
					}
					return true
				})
			// Back to writing after choosing a chat.
			if a.focusDraft {
				draft.Focus()
				a.focusDraft = false
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(4, 8, 8, 10).Children(func() {
				a.modelPicker(c)
				ui.Spacer(c)
				label, svg := "Send", arrowUpIcon
				if a.busy != nil {
					label, svg = "Stop", stopIcon
				}
				send := ui.ButtonBase(c.Key("send")).Size(30, 30).Radius(15).Label(label).Tooltip(label).
					Disabled(a.busy == nil && !ready)
				// Sending is the one primary action: the accent, with a hint
				// of it under; stopping is fg, and nothing to send a wash.
				switch {
				case a.busy != nil:
					send.Background(a.pal.fg).TextColor(a.pal.bg)
				case ready:
					send.Background(a.pal.ac).TextColor(a.pal.bg).Shadow(0, 3, 8, -2, a.pal.ac.Alpha(0.35))
				default:
					send.Background(a.pal.fg.Alpha(0.08)).TextColor(a.pal.fg.Alpha(0.35))
				}
				send.Children(func() { ui.Icon(c, svg).Size(16, 16) })
				if send.Clicked() {
					if a.busy != nil {
						a.stop()
					} else {
						a.send(a.draft)
					}
				}
			})
		})
		a.status(c)
	})
}

// status builds the row under the composer: where replies come from, and
// how to send.
func (a *app) status(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).WidthPercent(100).MaxWidth(maxWidth).Gap(8).AlignItems(ui.Center).Padding(0, 8).Children(func() {
		dot, name := t.Success, a.prov.Name()
		if _, ok := a.prov.(*demo); ok {
			dot, name = t.Warning, "Demo · set OPENROUTER_API_KEY for real replies"
		}
		ui.Box(c).Size(7, 7).Radius(3.5).Background(dot)
		ui.Text(c, name).FontSize(12).TextColor(t.TextMuted).SingleLine().Shrink(1)
		ui.Spacer(c)
		ui.Text(c, shortcutText("⏎ send  ·  ⇧⏎ new line", "Enter to send · Shift+Enter for a new line")).
			FontSize(12).TextColor(t.TextMuted).SingleLine()
	})
}

// modelPicker builds the button that chooses the model to ask.
func (a *app) modelPicker(c *ui.Context) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("model")).Padding(5, 8).Gap(6).Radius(8).TextColor(t.TextMuted).Label("Model")
	if b.Hovered() {
		b.Background(a.pal.fg.Alpha(0.06)).TextColor(t.Text)
	}
	b.Children(func() {
		ui.Icon(c, logoIcon).Size(13, 13).TextColor(a.pal.ac)
		ui.Text(c, a.model).FontSize(13)
		ui.Icon(c, chevronIcon).Size(12, 12)
	})
	b.Menu(func(m *ui.Menu) {
		for _, model := range a.prov.Models() {
			if m.Item(model).Checked(model == a.model).Chosen() {
				a.model = model
			}
		}
	})
}

// transcript returns th as Markdown.
func transcript(th *thread) string {
	var b strings.Builder
	b.WriteString("# " + th.Title + "\n\n")
	for _, m := range th.Messages {
		who := "**Assistant**"
		if m.Role == "user" {
			who = "**You**"
		}
		b.WriteString(who + "\n\n" + m.Text + "\n\n")
	}
	return b.String()
}
