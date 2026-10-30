package main

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"hyperai-desktop/locales"
)

// suggestions are what an empty chat offers to ask, each with the icon of
// its kind, in the app's language.
var suggestions = []struct {
	icon   *ui.SVG
	prompt func() string
}{
	{codeIcon, locales.SuggestionCache},
	{planIcon, locales.SuggestionPlan},
	{ideaIcon, locales.SuggestionScheduler},
}

// pane builds the chat shown: its title bar, its messages or the welcome,
// and the composer. titleLeft is the room left of the title, which grows
// to clear the window controls as the sidebar hides.
func (a *app) pane(c *ui.Context, titleLeft float32) {
	t := c.Theme()
	bar := c.TitleBar()
	th := a.current()
	// A project's page, where no chat shows (projects.go).
	var page *project
	if th == nil {
		page = a.shownProject()
	}
	pane := ui.Column(c).Grow(1).MinWidth(0).Background(t.Background)
	// Files dropped anywhere on the chat are attached to its draft, as
	// Codex's thread takes them; the composer shows they would be while
	// they are over it.
	a.attach(pane.DroppedFiles())
	a.fileOver = pane.FileDragOver()
	pane.Children(func() {
		ui.Row(c).Height(titleBarHeight).Shrink(0).AlignItems(ui.Center).Padding(0, bar.Right+12, 0, titleLeft).Children(func() {
			name := locales.NewChat()
			if th != nil {
				name = th.Title
				// Of a project: its name first, which goes to its page.
				if p := a.projectOf(th); p != nil {
					a.projectCrumb(c, p)
				}
			}
			if page != nil {
				name = page.Name
			}
			if th != nil && a.renaming == strconv.Itoa(th.ID) && !a.renameInSidebar || page != nil && a.renaming == a.selected {
				// Renamed in place, out of the room that drags the window.
				ui.Row(c).Grow(1).MinWidth(0).Height(titleBarHeight).AlignItems(ui.Center).Children(func() {
					a.renameField(c, 14, 500).MaxWidth(480)
					ui.Spacer(c)
				})
			} else {
				// The title drags the window; the menu beside it does not.
				ui.Row(c).Grow(1).MinWidth(0).Height(titleBarHeight).AlignItems(ui.Center).DragWindow().Children(func() {
					ui.Text(c, name).FontSize(14).FontWeight(500).SingleLine().MinWidth(0)
					// A title asked for anew, as the user asked.
					if th != nil && th.retitling {
						ui.Spinner(c).Size(14, 14).Shrink(0).Margin(0, 0, 0, 8).Label(locales.TitleRegenerating())
					}
				})
			}
			if th != nil {
				iconButton(c, moreIcon, locales.More()).Menu(func(m *ui.Menu) { a.chatMenu(c, m, th, false) })
			}
			if page != nil {
				iconButton(c, moreIcon, locales.More()).Menu(func(m *ui.Menu) { a.projectMenu(m, page) })
			}
		})
		if page != nil {
			// As an empty chat, the composer third, so that it keeps the
			// focus and what is typed as the chat starts.
			a.projectHeader(c, page)
			a.composer(c, nil)
			a.projectBody(c, page)
			return
		}
		if th == nil || len(th.Messages) == 0 {
			// The composer is the third element either way, so that it keeps
			// the focus and what is typed as the chat starts.
			a.greeting(c)
			a.composer(c, nil)
			a.prompts(c)
		} else {
			a.conversation(c, th)
			a.composer(c, th)
		}
	})
}

// greeting builds the top of an empty chat, over the composer, which the
// empty chat centers as hyperai-next's agent page does: the HyperAI mark,
// in the accent, over a question, as a chat app asks what is on your mind,
// of what HyperAI is for: AI, its resources and its compute.
func (a *app) greeting(c *ui.Context) {
	ui.Column(c).Grow(1).MinHeight(0).Justify(ui.End).AlignItems(ui.Center).Padding(24, 24, 28, 24).Gap(16).
		Children(func() {
			ui.Icon(c, markIcon).Size(48, 48).TextColor(a.pal.ac).Label("HyperAI")
			// Regular: at this size, 500 read heavy.
			ui.Text(c, locales.Greeting()).FontSize(26)
		})
}

// prompts builds the bottom of an empty chat, under the composer: what to
// ask, a row each, which a click writes in the composer to send as it is
// or after a change; and at the window's foot, a word on the replies.
func (a *app) prompts(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1.2).MinHeight(0).AlignItems(ui.Center).Padding(0, 24).Children(func() {
		ui.Column(c).WidthPercent(100).MaxWidth(maxWidth).Padding(4, 0, 0, 0).Children(func() {
			for i, s := range suggestions {
				prompt := s.prompt()
				// Its icon under the attach button's of the composer, as
				// hyperai-next's under its text: no card, no wash; the pointer
				// brings the text up the ladder, and the icon to the accent.
				row := ui.ButtonBase(c.Key("prompt "+strconv.Itoa(i))).Padding(7, attachLeft+(sendSize-16)/2+1).Gap(12).Radius(8).
					Justify(ui.Start).AlignItems(ui.Center).Label(prompt)
				on := row.Hovered()
				row.Children(func() {
					icon := ui.Icon(c, s.icon).Size(16, 16).Shrink(0).TextColor(t.TextMuted)
					text := ui.Text(c, prompt).FontSize(14).TextColor(t.TextMuted)
					if on {
						icon.TextColor(a.pal.ac)
						text.TextColor(t.Text)
					}
				})
				if row.Clicked() {
					a.draft, a.focusDraft = prompt, true
				}
			}
		})
		ui.Spacer(c)
		if a.prov != nil {
			ui.Text(c, locales.NoteAccuracy()).FontSize(12).TextColor(t.TextMuted).Padding(24, 0, 14, 0)
			return
		}
		// No provider has a key: the note goes to the page that takes one.
		link := ui.ButtonBase(c.Key("add a key")).Label(locales.NoteAddKey()).TextColor(a.pal.ac).Padding(24, 0, 14, 0)
		link.Children(func() {
			text := ui.Text(c, locales.NoteAddKey()).FontSize(12)
			if link.Hovered() {
				text.Underline()
			}
		})
		if link.Clicked() {
			a.showProviders()
		}
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
	// A row after the messages: the room that holds the question at the
	// top, or room for the composer, which floats over the list's end.
	rows := n + 1
	th.list.Key = func(i int) any {
		switch {
		case i < n:
			return th.Messages[i].ID
		case q >= 0:
			return "room"
		}
		return "composer"
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
		// and runs down to its bottom, under the composer, past which its
		// last row scrolls its end; the gap before that row is the room under
		// the messages.
		list := ui.List(c.Key(th.ID), &th.list, rows, func(i int) {
			if i == n {
				if q >= 0 {
					// As far as the list's bottom, under the composer too, so
					// that the question stays as the composer changes.
					ui.Box(c).Height(room + listPadBottom + a.footer)
				} else {
					a.composerRoom(c)
				}
				return
			}
			if m := th.Messages[i]; m.reveal {
				// A question just collapsed, from far down it: its top was
				// above the view.
				m.reveal = false
				th.list.ScrollIntoView(i)
			}
			row := ui.Row(c).Justify(ui.Center)
			if b := row.Bounds(); !empty(b) {
				th.boxes[i], th.Messages[i].height = b, b.H
			}
			row.Children(func() {
				col := ui.Column(c).Grow(1).MaxWidth(maxWidth)
				// The pointer over the message, or beside it, shows its actions.
				hover := col.Hovered()
				col.Children(func() { a.message(c, th, i, hover, i == n-1) })
			})
		}).Grow(1).Padding(listPadTop, 24, 0, 24).Gap(listGap).TrackScroll(&th.scroll)
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
		Label(locales.ScrollToEnd()).Absolute().LeftPercent(50).Bottom(a.footer + 16)
	tooltip(c, b, locales.ScrollToEnd())
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
func (a *app) message(c *ui.Context, th *thread, i int, hover, last bool) {
	t := c.Theme()
	m := th.Messages[i]
	if m.Role == "user" {
		if len(m.Files) > 0 {
			a.sentFiles(c, m)
		}
		if a.editingMsg == m.ID {
			a.editQuestion(c, th, i)
			return
		}
		if m.Text == "" {
			ui.Row(c).Justify(ui.End).Children(func() { a.questionActions(c, th, i, hover) })
			a.questionVersions(c, th, i)
			return
		}
		row := ui.Row(c).AlignItems(ui.Start).Gap(6)
		width := row.Bounds().W
		row.Children(func() {
			ui.Spacer(c)
			// Beside the bubble, centered on its first line, over the
			// bubble's padding.
			actions := a.questionActions(c, th, i, hover).Margin(bubblePadY+a.body*bodyLineHeight/2-actionSize/2, 0, 0, 0)
			// The row's room for the bubble, as of the last frame: past the
			// spacer's gap and the actions'.
			a.question(c, m, width-actions.Bounds().W-6)
		})
		a.questionVersions(c, th, i)
		return
	}
	// A little further from its question than the list's gap, which the
	// rule over it sits too close to; still closer than to the next question,
	// with its actions between.
	reply := ui.Column(c).Gap(10).AlignItems(ui.Stretch).Padding(replyGap, 0, 0, 0)
	// A reply built anew while it streams, its chat shown again or it
	// scrolled back into view, shows what came at once, and reveals only
	// what comes next: its reveal went with the element, which started
	// over from the reply's first word.
	anew := false
	ui.Local(reply, shownKey{}, func() bool { anew = true; return true })
	reply.Children(func() {
		if m.streaming || m.Elapsed > 0 {
			worked(c, m)
		}
		if strings.TrimSpace(m.Reasoning) != "" {
			a.thoughts(c, m)
		}
		switch {
		case m.streaming && m.Text == "":
		case m.streaming && anew:
			// For this view only: Options holds a lock, and is not copied.
			reveal := a.md.Reveal
			a.md.Reveal = &revealAtOnce
			markdown.Stream(c, m.Text, &a.md)
			a.md.Reveal = reveal
		case m.streaming:
			markdown.Stream(c, m.Text, &a.md)
		case m.Text != "":
			markdown.View(c, m.Text, &a.md)
		}
		if m.Error != "" {
			ui.Text(c, m.errorText()).FontSize(14).TextColor(t.Danger).Selectable()
		}
		// Under the reply, once it ended, its words or what went wrong:
		// its versions, always, then its buttons on the text's edge, and
		// the time at the far end, under the pointer and on the last reply.
		if !m.streaming && (m.Text != "" || m.Error != "") {
			actions := ui.Row(c).Margin(-4, 0, 0, -barInset(th, i)).Gap(barGap).AlignItems(ui.Center)
			reveal := actions.Animate("opacity", shown(hover || last), revealDuration)
			actions.Children(func() {
				a.versionSwitcher(c, th, i)
				ui.Row(c).AlignItems(ui.Center).Gap(barGap).Opacity(reveal).Children(func() {
					if m.Text != "" {
						a.copyButton(c, m)
					}
					a.regenerateButton(c, th, i)
					a.usageButton(c, m)
				})
				ui.Spacer(c)
				ui.Row(c).Opacity(reveal).Children(func() { a.messageTime(c, m) })
			})
		}
	})
}

// thoughts builds what the model reasoned before reply m: as it comes,
// until the reply's text does, and then folded under how long it took,
// which unfolds it, muted, by a rule down its side.
func (a *app) thoughts(c *ui.Context, m *message) {
	t := c.Theme()
	if !m.streaming || m.Text != "" {
		label := locales.ThoughtFor(seconds(m.Reasoned))
		ui.Row(c).Children(func() {
			btn := ui.ButtonBase(c).Gap(4).AlignItems(ui.Center).TextColor(t.TextMuted).Label(label).Expanded(m.thoughtsOpen)
			if btn.Hovered() {
				btn.TextColor(t.Text)
			}
			btn.Children(func() {
				ui.Text(c, label).FontSize(13)
				chevron := ui.Icon(c, chevronIcon).Size(14, 14)
				if !m.thoughtsOpen {
					chevron.Rotate(-90)
				}
			})
			if btn.Clicked() {
				m.thoughtsOpen = !m.thoughtsOpen
			}
		})
		if !m.thoughtsOpen {
			return
		}
	}
	ui.Row(c).Gap(12).AlignItems(ui.Stretch).Children(func() {
		ui.Box(c).Width(2).Shrink(0).Radius(1).Background(t.Border)
		ui.Text(c, strings.TrimSpace(m.Reasoning)).Grow(1).MinWidth(0).FontSize(13).LineHeight(1.6).
			TextColor(t.TextMuted).Selectable()
	})
}

// shownKey keys the state a reply's element keeps, which tells it was
// built before (message).
type shownKey struct{}

// revealAtOnce shows a stream's text as it is, still streaming: what a
// reply shown anew had come.
var revealAtOnce = markdown.Reveal{Unit: markdown.AtOnce}

// question builds the user's message m in its bubble, as wide as room at
// most. A long one, past clampLines+slackLines lines, shows clampLines of
// them, fading out, over a button that shows it whole and collapses it
// again, as hyperai-next's agent does.
func (a *app) question(c *ui.Context, m *message, room float32) {
	t := c.Theme()
	width := float32(bubbleMaxWidth - 2*bubblePadX)
	switch {
	case room > 0:
		width = min(width, room-2*bubblePadX)
	case m.wrapWidth > 0:
		width = m.wrapWidth
	}
	if room <= 0 {
		c.AnimationFrame() // a frame laid out, for the room
	}
	if width != m.wrapWidth {
		// Every line is as tall, whatever the script: the font's.
		_, h := c.MeasureText(width, ui.Span{Text: m.Text, Size: a.body})
		_, line := c.MeasureText(0, ui.Span{Text: "x", Size: a.body})
		m.lines, m.wrapWidth = int(h/line+0.5), width
	}
	long := m.lines > clampLines+slackLines
	collapsed := long && !m.expanded
	bubble := ui.Column(c).MinWidth(0).MaxWidth(bubbleMaxWidth).Padding(bubblePadY, bubblePadX).Radius(14).
		Background(a.pal.raised).AlignItems(ui.Stretch).Gap(4)
	bubble.Children(func() {
		h := clampLines * a.body * bodyLineHeight
		clip := ui.Column(c)
		if collapsed {
			clip.Height(h).ClipY()
		}
		clip.Children(func() {
			ui.Text(c, m.Text).FontSize(a.body).LineHeight(bodyLineHeight).Selectable()
			if collapsed {
				// Its last lines fade into the bubble, from 70% down.
				ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(h*0.3).PassThrough().
					Gradient(a.pal.raised.Alpha(0), a.pal.raised, 180)
			}
		})
		if !long {
			return
		}
		// Muted, open or not, as hyperai-next's; the accent under the pointer,
		// as a link.
		label, svg := locales.ShowMore(), chevronIcon
		if m.expanded {
			label, svg = locales.ShowLess(), chevronUpIcon
		}
		b := ui.ButtonBase(c.Key("show more")).AlignSelf(ui.Start).Padding(2, 0).Gap(4).TextColor(t.TextMuted).Label(label)
		if b.Hovered() {
			b.TextColor(a.pal.ac)
		}
		b.Children(func() {
			ui.Text(c, label).FontSize(13)
			ui.Icon(c, svg).Size(14, 14)
		})
		if b.Clicked() {
			m.expanded = !m.expanded
			m.reveal = !m.expanded
		}
	})
}

const (
	// A long question shows clampLines lines collapsed. One at most
	// slackLines past them shows whole: collapsing it would hide less than
	// the button takes.
	clampLines = 6
	slackLines = 2
)

const (
	// The conversation's text — questions, replies, and the composer —
	// is 15 DIPs on lines of 1.6 at first, as hyperai-next's (text-[15px],
	// leading-relaxed), and as large as the settings make it (app.body);
	// the rest of the window keeps the theme's size.
	bodySize       = 15
	bodyLineHeight = 1.6

	replyGap       = 8   // a reply's room over it, past the list's gap
	bubblePadY     = 9   // a question's bubble's padding over and under its text
	bubblePadX     = 14  // and to its sides
	bubbleMaxWidth = 560 // a question's bubble's width at most
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

// questionActions builds the row of th's question at i's actions, beside
// it on the left, under the pointer: its time and its buttons.
func (a *app) questionActions(c *ui.Context, th *thread, i int, hover bool) ui.Element {
	m := th.Messages[i]
	actions := ui.Row(c).Height(actionSize).Gap(barGap).AlignItems(ui.Center)
	actions.Opacity(actions.Animate("opacity", shown(hover), revealDuration))
	actions.Children(func() {
		ui.Row(c).Padding(0, 4, 0, 0).Children(func() { a.messageTime(c, m) })
		if m.Text != "" {
			a.editButton(c, th, m)
			a.copyButton(c, m)
		}
	})
	return actions
}

// questionVersions builds, under th's question at i, where it has
// versions, its switcher, always, on the right, its last chevron on the
// bubble's edge.
func (a *app) questionVersions(c *ui.Context, th *thread, i int) {
	ui.Row(c).Justify(ui.End).Children(func() {
		a.versionSwitcher(c, th, i).Margin(2, -switchInset, 0, 0)
	})
}

// copyButton builds the button that copies m's text, which shows a check
// for a moment after.
func (a *app) copyButton(c *ui.Context, m *message) {
	left := a.copiedUntil.Sub(c.Now())
	copied := a.copied == m.ID && left > 0
	svg, label := copyIcon, locales.Copy()
	if copied {
		svg, label = checkIcon, locales.Copied()
		c.After(left) // a frame as the check goes
	}
	b := actionButton(c, svg, label, actionIcon)
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
	tooltip(c, ui.Text(c, short).FontSize(12).TextColor(c.Theme().TextMuted).FontFeatures("tnum").SingleLine().Description(full), full)
}

// formatTime returns t shortly — the clock today, the day too before
// that, and the year outside this one — and in full, as hyperai-next's
// messages show it, in the app's language: "Oct 28, 11:05 PM" in English,
// "10月28日 23:05" in Chinese. now is the time now, in the zone t shows in.
func formatTime(t, now time.Time) (short, full string) {
	t = t.In(now.Location())
	lang := i18n.Locale()
	skeleton := "jm"
	switch {
	case t.Year() != now.Year():
		skeleton = "yMMMdjm"
	case t.YearDay() != now.YearDay():
		skeleton = "MMMdjm"
	}
	short = i18n.FormatDate(lang, t, i18n.DateOptions{Skeleton: skeleton})
	return short, i18n.FormatDate(lang, t, i18n.DateOptions{Date: i18n.LongStyle, Time: i18n.ShortStyle})
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
			ui.Text(c, locales.Thinking()).FontSize(12).TextColor(t.TextMuted)
		} else {
			ui.Text(c, locales.WorkedFor(seconds(m.Elapsed))).FontSize(12).TextColor(t.TextMuted)
		}
		rule()
	})
}

// seconds is a duration of s seconds, shortly, in the app's language: 4s,
// 2m 5s.
func seconds(s float64) string {
	n := max(1, int(s+0.5))
	if n < 60 {
		return locales.DurationSeconds(n)
	}
	return locales.DurationMinutes(n/60, n%60)
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
	ready := strings.TrimSpace(a.draft) != "" || len(a.files) > 0
	a.attachToasts(c)
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

// On an empty chat, the composer is a pill while the draft is a line that
// fits beside its buttons, as ChatGPT's is, and opens into a box as the
// draft breaks a line or outgrows it, the field across the box over a row
// of the buttons; over a conversation, it is that box. Its
// heights are these, so that the conversation's end knows them in the
// frame the composer takes them (composerRoom).
const (
	composerRadius = 24 // a pill's, one line tall; the box keeps it
	fieldPadX      = 12 // the field's text in from the box's left, as Codex's
	// attachLeft is the attach button in from the box's left, as far as
	// the send button is from its right: round in the box's round corner,
	// both are as far from it all round (composerRadius - sendSize/2).
	attachLeft   = composerRadius - sendSize/2
	pillPadLeft  = attachLeft + sendSize + 4 // the text in from a pill's left, past the attach button
	pillPadY     = 12                        // the field's padding over and under its line, as a pill
	boxPadTop    = 16                        // and as a box: room for the fade (fieldFadeGap)
	boxPadBottom = 6
	// filesPadTop is the field's padding over its text under the files
	// attached, which keep the room over them: closer, as Codex's text is
	// to its images, and room for a fade still.
	filesPadTop    = fieldFadeGap + 2
	toolbarHeight  = 44 // the row of the buttons under the field of a box
	sendSize       = 32
	floatPadTop    = 8  // the room over the composer floating over a conversation
	composerPadEnd = 16 // and under it
)

// The fade over the field's top lies in the room over its text, and only
// there: text at rest under it is never faded, however far the field is
// scrolled. The box's color over the room's first fieldFadeGap, which text
// scrolled up never shows in, then a fade to clear by the room's end.
const fieldFadeGap = 6

// fieldFade returns how strong the fade over the field's text is: none at
// its top, and full once it scrolled as far as the room over the text, so
// that it comes in with the scrolling rather than at once.
func (a *app) fieldFade() float32 {
	return max(0, min(a.fieldScroll.Y/a.fieldTop(), 1))
}

// fieldTop returns the field's padding over its text, as a box.
func (a *app) fieldTop() float32 {
	if len(a.files) > 0 {
		return filesPadTop
	}
	return boxPadTop
}

// composerPill reports whether the composer shows as a pill, as of the box's
// width and its buttons' when it was laid out last: on an empty chat
// (welcome), while the draft is one line that fits beside them, and has no
// files, which show over it.
func (a *app) composerPill(c *ui.Context, welcome bool) bool {
	if !welcome || len(a.files) > 0 || strings.Contains(a.draft, "\n") {
		return false
	}
	if a.draft == "" || a.boxWidth == 0 {
		return true
	}
	if a.draft != a.measured {
		a.measured = a.draft
		a.measuredWidth, _ = c.MeasureText(0, ui.Span{Text: a.draft, Size: a.body})
	}
	return a.measuredWidth <= a.pillWidth()
}

// pillWidth and boxWidthText are how wide the field's text is as a pill,
// beside the buttons, and as a box, across it, less the caret's room.
func (a *app) pillWidth() float32    { return a.boxWidth - 2 - pillPadLeft - 8 - a.toolsWidth - 4 }
func (a *app) boxTextWidth() float32 { return a.boxWidth - 2 - 2*fieldPadX }

// composerRoom builds, at the end of a conversation, room as tall as the
// composer floating over it, in the layout of the frame the composer is
// laid out in: the field's text laid out again, as the field lays it out,
// and the rest as tall as the composer's heights make it as a pill or as a
// box. So the end of the conversation moves with the field as it grows or
// shrinks, and as it opens or closes, rather than a frame after, which
// showed an empty band as tall as the lines just deleted.
func (a *app) composerRoom(c *ui.Context) {
	text := a.draft
	if text == "" || strings.HasSuffix(text, "\n") {
		text += "\u200b" // the line the caret is on, empty
	}
	rest, w := floatPadTop+composerPadEnd+2+a.fieldTop()+boxPadBottom+toolbarHeight, a.boxTextWidth()
	if a.pill {
		rest, w = floatPadTop+composerPadEnd+2+2*pillPadY, a.pillWidth()
	}
	if len(a.files) > 0 {
		rest += chipsPadTop + chipSize
	}
	ui.Column(c).Padding(0, 0, rest, 0).Children(func() {
		ui.Text(c, text).Width(max(0, w)).FontSize(a.body).LineHeight(bodyLineHeight).
			MaxLines(10).Opacity(0)
	})
}

// composerColumn builds the composer's column: the field and its buttons.
func (a *app) composerColumn(c *ui.Context, th *thread, ready bool) {
	t := c.Theme()
	col := ui.Column(c).WidthPercent(100).MaxWidth(maxWidth).Padding(0, 0, composerPadEnd, 0)
	if th != nil {
		box := col.Bounds()
		col.Padding(floatPadTop, 0, composerPadEnd, 0).Background(t.Background).HandleInput(func(ev ui.InputEvent) bool {
			// Scrolling over it scrolls the conversation, as over
			// hyperai-next's footer, which is in the conversation; but over
			// the field with more text than it shows, the field, to its
			// ends and no further, as a browser's text area does.
			if ev.Kind != ui.InputScroll || ev.DY == 0 {
				return false
			}
			x, y := box.X+ev.X, box.Y+ev.Y
			f := a.field
			if a.fieldScroll.MaxY > 0 && x >= f.X && x < f.X+f.W && y >= f.Y && y < f.Y+f.H {
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
		// Its line is drawn over what is in it (the last child), which the
		// fade over the field would otherwise draw over, where the corners
		// round; its own is clear, for the room it takes.
		box := ui.Column(c).WidthPercent(100).MaxWidth(maxWidth).Radius(composerRadius).Border(1, ui.Transparent).
			Background(a.pal.composer).Shadow(0, 4, 16, -4, a.pal.shadow(0.12))
		if b := box.Bounds(); !empty(b) {
			a.boxWidth = b.W
		}
		pill := a.pill
		box.Children(func() {
			// The files attached, over the field, as Codex shows them.
			a.draftFiles(c)
			// The field's row, the field first in it either way, so that it
			// keeps its place in the tree, and with it the focus and the
			// caret, as the composer opens; as a pill, the buttons after it.
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				top, right, bottom, left := a.fieldTop(), float32(fieldPadX), float32(boxPadBottom), float32(fieldPadX)
				if pill {
					top, right, bottom, left = pillPadY, 8, pillPadY, pillPadLeft
				}
				// The field, and over it its placeholder and the fade at its
				// top.
				svc := c.Services()
				ui.Box(c).Grow(1).MinWidth(0).Children(func() {
					// Return sends, or Cmd+Return as the settings choose
					// (app.sends); the editor breaks the line for the rest.
					draft := ui.TextAreaBase(c.Key("draft"), &a.draft).WidthPercent(100).Lines(1, 10).
						FontSize(a.body).LineHeight(bodyLineHeight).Padding(top, right, bottom, left).
						TrackScroll(&a.fieldScroll).
						Label(locales.ComposerLabel()).AutoFocus().
						// Files and images pasted are attached (attach.go).
						OnPaste(func(text string) bool { return a.paste(svc, text) }).
						OnTextKey(func(k ui.TextKey) bool {
							if !a.sends(k.Key, k.Mods) {
								return false
							}
							switch {
							case a.shownReply() != nil || strings.TrimSpace(k.Text()) == "" && len(a.files) == 0:
							case a.prov == nil:
								// Out of the frame: the toast shows in the next.
								a.needProvider = true
							default:
								a.send(k.Text())
								k.SetText("")
							}
							return true
						})
					a.placeholder(c, locales.ComposerPlaceholder(), a.draft == "" && !draft.Composing(), top, left, a.body, bodyLineHeight)
					if b := draft.Bounds(); !empty(b) {
						a.field = b
					}
					// Text scrolled up fades out as it goes into the room over
					// it, as the conversation does over the composer, and is
					// gone short of the box's edge, which keeps its room: the
					// box's color over the room's first fieldFadeGap, then a
					// fade to clear by the room's end, where text rests. It
					// keeps off the box's rounded corner, left of the text,
					// and off the scroll bar.
					if k := a.fieldFade(); k > 0 {
						// Two pieces, the solid one and the fade: a gradient's
						// Start and Stops drew as a plain fade, edge to edge.
						solid := a.pal.composer.Alpha(k)
						ui.Box(c).Absolute().Top(0).Left(left).Right(right + barClear).
							Height(fieldFadeGap).PassThrough().Background(solid)
						ui.Box(c).Absolute().Top(fieldFadeGap).Left(left).Right(right + barClear).
							Height(top - fieldFadeGap).PassThrough().LinearGradient(ui.LinearGradient{
							From: solid, To: a.pal.composer.Alpha(0), Angle: 180,
						})
					}
					// Back to writing after choosing a chat, or a prompt: at the
					// end of what is there, where the app's text leaves the
					// caret at its start.
					if a.focusDraft {
						n := utf8.RuneCountInString(a.draft)
						draft.Focus().SetTextSelection(n, n)
						a.focusDraft = false
					}
				})
				if pill {
					if b := a.composerTools(c, ready).Padding(0, 8, 0, 0).Bounds(); !empty(b) {
						a.toolsWidth = b.W
					}
					// Left of the field's text, as ChatGPT's pill has it; after
					// the field in the tree, which keeps its place.
					a.attachButton(c).Absolute().Left(attachLeft).Top(pillPadY + (a.body*bodyLineHeight-sendSize)/2)
				}
			})
			if !pill {
				// As Codex's: attaching on the left, the model and sending on
				// the right.
				ui.Row(c).Height(toolbarHeight).Shrink(0).AlignItems(ui.Center).Children(func() {
					a.attachButton(c).Margin(0, 0, 0, attachLeft)
					ui.Spacer(c)
					a.composerTools(c, ready).Padding(0, 8, 0, 0)
				})
			}
			ui.Box(c).Absolute().Top(-1).Left(-1).Right(-1).Bottom(-1).Radius(composerRadius).
				Border(1, t.Border).PassThrough()
			if a.fileOver {
				a.dropOverlay(c)
			}
		})
	})
}

// composerTools builds the composer's buttons, in a row: the model to ask,
// and the button that sends, or stops the reply.
func (a *app) composerTools(c *ui.Context, ready bool) ui.Element {
	row := ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center)
	busy := a.shownReply() != nil
	row.Children(func() {
		a.modelPicker(c)
		label, svg := locales.Send(), arrowUpIcon
		if busy {
			label, svg = locales.Stop(), stopIcon
		}
		send := tooltip(c, ui.ButtonBase(c.Key("send")).Size(sendSize, sendSize).Radius(sendSize/2).Label(label), label).
			Disabled(!busy && !ready)
		// Sending is the one primary action: the accent, with a hint of it
		// under; stopping is fg, and nothing to send a wash.
		switch {
		case busy:
			send.Background(a.pal.fg).TextColor(a.pal.bg)
		case ready:
			send.Background(a.pal.ac).TextColor(a.pal.bg).Shadow(0, 3, 8, -2, a.pal.ac.Alpha(0.35))
		default:
			send.Background(a.pal.fg.Alpha(0.08)).TextColor(a.pal.fg.Alpha(0.35))
		}
		send.Children(func() { ui.Icon(c, svg).Size(16, 16) })
		if a.needProvider {
			a.needProvider = false
			a.askForProvider(c)
		}
		if send.Clicked() {
			switch {
			case busy:
				a.stop(a.current())
			case a.prov == nil:
				a.askForProvider(c)
			default:
				a.send(a.draft)
			}
		}
	})
	return row
}

// askForProvider says, as the user sends while no provider can be asked,
// that one needs a key, or a local server on, first, with the button that
// goes to the page that sets them up; the draft stays. Sending again shows the toast anew,
// rather than another.
func (a *app) askForProvider(c *ui.Context) {
	c.AddToast(ui.Toast{ID: "no provider", Title: locales.NoProviderTitle(), Description: locales.NoProviderAbout(),
		Action: locales.ModelAddProvider(), OnAction: a.showProviders})
}

// modelPicker builds the button that chooses the model to ask, or, where
// no provider can be asked, that goes to the page that sets one up.
func (a *app) modelPicker(c *ui.Context) {
	t := c.Theme()
	if a.prov == nil {
		b := ui.ButtonBase(c.Key("add a provider")).Padding(5, 8).Gap(6).Radius(8).TextColor(a.pal.ac).
			Label(locales.ModelAddProvider())
		if b.Hovered() {
			b.Background(a.pal.fg.Alpha(0.06))
		}
		b.Children(func() {
			ui.Icon(c, plugIcon).Size(13, 13)
			ui.Text(c, locales.ModelAddProvider()).FontSize(13)
		})
		if b.Clicked() {
			a.showProviders()
		}
		return
	}
	b := ui.ButtonBase(c.Key("model")).Padding(5, 8).Gap(6).Radius(8).TextColor(t.TextMuted).Label(locales.Model())
	if b.Hovered() {
		b.Background(a.pal.fg.Alpha(0.06)).TextColor(t.Text)
	}
	b.Children(func() {
		ui.Icon(c, logoIcon).Size(13, 13).TextColor(a.pal.ac)
		ui.Text(c, a.model).FontSize(13)
		ui.Icon(c, chevronIcon).Size(12, 12)
	})
	b.Menu(func(m *ui.Menu) {
		a.modelMenu(m, func(p provider, model string) bool { return p == a.prov && model == a.model }, a.pick)
	})
}

// transcript returns th as Markdown, who said each message in the app's
// language.
func transcript(th *thread) string {
	var b strings.Builder
	b.WriteString("# " + th.Title + "\n\n")
	for _, m := range th.Messages {
		who := locales.TranscriptAssistant()
		if m.Role == "user" {
			who = locales.TranscriptYou()
		}
		b.WriteString("**" + who + "**\n\n" + m.Text + "\n\n")
	}
	return b.String()
}
