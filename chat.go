package main

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

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
			a.composer(c, nil)
		} else {
			a.conversation(c, th)
			a.composer(c, th)
		}
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
						ui.Text(c, s.title).FontSize(14).FontWeight(600)
						ui.Text(c, s.prompt).FontSize(13).TextColor(t.TextMuted).MaxLines(2)
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
		room, outgrown = th.measure(q, th.scroll.Y, h, th.listBox, listPadBottom+a.footer)
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
		// is as wide as the pane, so that it scrolls wherever the pointer is,
		// and runs down to its bottom, under the composer, past whose height
		// its end scrolls.
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
				col := ui.Column(c).Grow(1).MaxWidth(maxWidth)
				// The pointer over the message, or beside it, shows its actions.
				hover := col.Hovered()
				col.Children(func() { a.message(c, th.Messages[i], hover, i == n-1) })
			})
		}).Grow(1).Padding(listPadTop, 24, listPadBottom+a.footer, 24).Gap(listGap).TrackScroll(&th.scroll)
		th.listBox = list.Bounds()
		// What shows of the area: above the composer.
		shows := area.Bounds()
		shows.H = max(0, shows.H-a.footer)
		a.outline(c, th, shows)
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
		Absolute().LeftPercent(50).Bottom(a.footer + 16)
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
// assistant's as Markdown across the column, with its actions once done,
// shown while hover, and always for the last reply.
func (a *app) message(c *ui.Context, m *message, hover, last bool) {
	t := c.Theme()
	if m.Role == "user" {
		ui.Row(c).AlignItems(ui.Start).Gap(4).Children(func() {
			ui.Spacer(c)
			// Beside the bubble, centered on its first line, over the
			// bubble's padding: the time, then the button next to the bubble.
			actions := ui.Row(c).Height(actionsHeight).Margin(bubblePadY+bodySize*bodyLineHeight/2-actionsHeight/2, 0, 0, 0).
				Gap(4).AlignItems(ui.Center)
			actions.Opacity(actions.Animate("opacity", shown(hover), revealDuration))
			actions.Children(func() {
				a.messageTime(c, m)
				a.copyButton(c, m)
			})
			ui.Text(c, m.Text).FontSize(bodySize).LineHeight(bodyLineHeight).Selectable().Padding(bubblePadY, 14).Radius(14).Background(a.pal.raised).MaxWidth(560)
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
			ui.Text(c, m.Error).FontSize(14).TextColor(t.Danger).Selectable()
		}
		// Under the reply: the button on the text's edge, the time at the
		// far end.
		if !m.streaming && m.Text != "" {
			actions := ui.Row(c).Margin(-4, 0, 0, -6).AlignItems(ui.Center)
			actions.Opacity(actions.Animate("opacity", shown(hover || last), revealDuration))
			actions.Children(func() {
				a.copyButton(c, m)
				ui.Spacer(c)
				a.messageTime(c, m)
			})
		}
	})
}

const (
	// The conversation's text — questions, replies, and the composer —
	// is 15 DIPs on lines of 1.6, as hyperai-next's (text-[15px],
	// leading-relaxed); the rest of the window keeps the theme's size.
	bodySize       = 15
	bodyLineHeight = 1.6

	bubblePadY    = 9  // a question's bubble's padding over and under its text
	actionsHeight = 28 // a message's actions' height: an iconButton's
)

// revealDuration is how long a message's actions take to fade in or out.
const revealDuration = 150 * time.Millisecond

// shown is the opacity of what shows while show is true.
func shown(show bool) float32 {
	if show {
		return 1
	}
	return 0
}

// copyButton builds the button that copies m's text, which shows a check
// for a moment after.
func (a *app) copyButton(c *ui.Context, m *message) {
	left := a.copiedUntil.Sub(c.Now())
	copied := a.copied == m.ID && left > 0
	svg, label := copyIcon, "Copy"
	if copied {
		svg, label = checkIcon, "Copied"
		c.After(left) // a frame as the check goes
	}
	// A size under the other buttons', as it shows on every message.
	b := iconButtonSized(c, svg, label, 14)
	if copied {
		b.TextColor(c.Theme().Success)
	}
	if b.Clicked() {
		c.Services().WriteClipboard(m.Text)
		a.copied, a.copiedUntil = m.ID, c.Now().Add(copiedDuration)
		c.After(0)
	}
}

// copiedDuration is how long a copy button shows its check.
const copiedDuration = 1500 * time.Millisecond

// messageTime builds when m was, shortly, with the long form in its
// tooltip; nothing for a message that does not know.
func (a *app) messageTime(c *ui.Context, m *message) {
	if m.Time.IsZero() {
		return
	}
	short, full := formatTime(m.Time, c.Now())
	ui.Text(c, short).FontSize(12).TextColor(c.Theme().TextMuted).FontFeatures("tnum").SingleLine().Tooltip(full)
}

// formatTime returns t shortly — the clock today, the day too before
// that, and the year outside this one — and in full, as hyperai-next's
// messages show it; now is the time now, in the zone t shows in.
func formatTime(t, now time.Time) (short, full string) {
	t = t.In(now.Location())
	switch {
	case t.Year() != now.Year():
		short = t.Format("Jan 2, 2006, 3:04 PM")
	case t.YearDay() != now.YearDay():
		short = t.Format("Jan 2, 3:04 PM")
	default:
		short = t.Format("3:04 PM")
	}
	return short, t.Format("January 2, 2006 at 3:04 PM")
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
//
// Over th's conversation (nil for none), it floats over the pane's bottom,
// as hyperai-next's sticky footer does: the conversation runs down to the
// window's bottom, its scrollbar with it, and scrolls under the composer,
// which hides it on bg, faded out above. It stays the same element either
// way, so that it keeps what is typed and the focus as a chat starts.
func (a *app) composer(c *ui.Context, th *thread) {
	ready := strings.TrimSpace(a.draft) != ""
	outer := ui.Column(c).Shrink(0).AlignItems(ui.Center).Padding(0, 24)
	if th != nil {
		// Only the column takes the pointer: beside it, the pointer reaches
		// the conversation and its scrollbar.
		outer.Absolute().Left(0).Right(0).Bottom(0).PassThrough()
	}
	// The conversation leaves the room it takes at its end; measured the
	// frame before, so the first frame it changes asks for another.
	if h := outer.Bounds().H; h != a.footer {
		a.footer = h
		c.After(0)
	}
	outer.Children(func() { a.composerColumn(c, th, ready) })
}

// composerColumn builds the composer's column: the field and its buttons,
// and the status under them.
func (a *app) composerColumn(c *ui.Context, th *thread, ready bool) {
	t := c.Theme()
	col := ui.Column(c).WidthPercent(100).MaxWidth(maxWidth).Gap(8).Padding(0, 0, 14, 0)
	if th != nil {
		col.Padding(8, 0, 14, 0).Background(t.Background).HandleInput(func(ev ui.InputEvent) bool {
			// Scrolling over it scrolls the conversation, as over
			// hyperai-next's footer, which is in the conversation.
			if ev.Kind != ui.InputScroll || ev.DY == 0 {
				return false
			}
			th.scroll.Y = max(0, min(th.scroll.Y+ev.DY, th.scroll.MaxY))
			return true
		})
	}
	col.Children(func() {
		if th != nil {
			ui.Box(c).Absolute().Left(0).Right(0).Top(-16).Height(16).PassThrough().
				LinearGradient(ui.LinearGradient{From: t.Background.Alpha(0), To: t.Background, Angle: 180})
		}
		box := ui.Column(c).WidthPercent(100).MaxWidth(maxWidth).Radius(16).Border(1, t.Border).
			Background(a.pal.composer).Shadow(0, 4, 16, -4, a.pal.shadow(0.12))
		box.Children(func() {
			// Enter sends, Shift+Enter breaks the line.
			draft := ui.TextAreaBase(c.Key("draft"), &a.draft).Lines(1, 10).FontSize(bodySize).LineHeight(bodyLineHeight).Padding(14, 16, 6, 16).
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
