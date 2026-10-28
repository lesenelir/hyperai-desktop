package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// maxOutlineTurns is the most turns the outline shows: 20 ticks of 14 DIPs
// fit any window, so ticks never shrink; past that it hides.
const maxOutlineTurns = 20

// outlineTurn is a question of a chat and its reply, as the outline shows
// them: row is the question's row in the conversation.
type outlineTurn struct {
	row           int
	prompt, reply string
}

// outlineTurns groups msgs into turns: a question opens one, and the reply
// after it answers it.
func outlineTurns(msgs []*message) []outlineTurn {
	var turns []outlineTurn
	for i, m := range msgs {
		if m.Role == "user" {
			turns = append(turns, outlineTurn{row: i, prompt: m.previewText(), reply: "No reply"})
			continue
		}
		if len(turns) == 0 {
			continue
		}
		t := &turns[len(turns)-1]
		switch {
		case m.streaming:
			t.reply = "Generating…"
		case m.Text != "":
			t.reply = m.previewText()
		case m.Error == "Stopped.":
			t.reply = "Stopped"
		case m.Error != "":
			t.reply = "Failed"
		}
	}
	return turns
}

// previewText returns m's text as one line, whitespace collapsed, enough
// for three: made once, as the outline asks every frame.
func (m *message) previewText() string {
	if m.preview == "" && !m.streaming {
		text := strings.Join(strings.Fields(m.Text), " ")
		if r := []rune(text); len(r) > 280 {
			text = string(r[:280])
		}
		m.preview = text
	}
	return m.preview
}

// tickWidth is the length of a tick distance ticks from the one under the
// pointer (-1 for none): the wave, longest under the pointer and easing to
// the resting length over two ticks either side, as hyperai-next's outline
// (and Waku's rail) does. The turn at the top of the conversation rests a
// little longer.
func tickWidth(distance int, current bool) float32 {
	switch distance {
	case 0:
		return 32
	case 1:
		return 24
	case 2:
		return 16
	}
	if current {
		return 16
	}
	return 12
}

// outline builds, on the left of the conversation in area, a tick per turn,
// as hyperai-next's agent does: the tick of the turn at the top of the
// conversation is dark, the pointer raises a wave and a card with the
// question and its reply, and a click glides to the question.
// It shows for 2 to maxOutlineTurns turns.
func (a *app) outline(c *ui.Context, th *thread, area ui.Rect) {
	turns := outlineTurns(th.Messages)
	if len(turns) < 2 || len(turns) > maxOutlineTurns {
		return
	}
	// The turn at the top is the last to start at or above the first row
	// that shows.
	first, _ := th.list.Visible()
	current := 0
	for i, tu := range turns {
		if tu.row <= first {
			current = i
		}
	}

	hovered := -1
	var hoveredBox ui.Rect
	bars := make([]ui.Element, len(turns))
	ui.Column(c.Key("outline")).Absolute().Left(12).Top(16).Bottom(16).Width(40).
		Justify(ui.Center).AlignItems(ui.Start).Label("Conversation outline").
		Children(func() {
			for i, tu := range turns {
				b := ui.ButtonBase(c.Key(tu.row)).Height(14).WidthPercent(100).Justify(ui.Start).Radius(4).
					Label("Message " + strconv.Itoa(i+1) + ": " + tu.prompt)
				if b.Hovered() {
					hovered, hoveredBox = i, b.Bounds()
				}
				b.Children(func() { bars[i] = ui.Box(c).Height(2).Radius(1) })
				if b.Clicked() {
					th.glideTo(tu.row)
				}
			}
		})
	// The ticks take their lengths once the one under the pointer is known.
	for i, bar := range bars {
		distance := -1
		if hovered >= 0 {
			distance = max(i-hovered, hovered-i)
		}
		w := bar.Animate("width", tickWidth(distance, i == current), 200*time.Millisecond)
		color := a.pal.fg.Alpha(0.2)
		if distance == 0 || i == current {
			color = a.pal.fg
		}
		bar.Width(w).Background(color)
	}
	if hovered >= 0 {
		a.outlineCard(c, turns[hovered], hoveredBox, area)
	}
}

// outlineCard builds the card of turn beside its tick, which is at box in
// area: the question on a line, and three of its reply.
func (a *app) outlineCard(c *ui.Context, turn outlineTurn, box, area ui.Rect) {
	t := c.Theme()
	// Centred on the tick, and kept 8 DIPs inside the area; measured the
	// frame before, so the first frame it shows asks for another.
	top := box.Y + box.H/2 - area.Y - a.cardHeight/2
	top = max(8, min(top, area.H-a.cardHeight-8))
	card := ui.Column(c.Key("outline card")).Absolute().Left(12+40+12).Top(top).Width(288).
		Padding(12).Gap(4).Radius(12).Background(a.pal.composer).Border(1, t.Border).
		Shadow(0, 10, 24, -6, a.pal.shadow(0.2)).PassThrough()
	card.Children(func() {
		prompt := turn.prompt
		if prompt == "" {
			prompt = "Message"
		}
		ui.Text(c, prompt).FontWeight(600).SingleLine()
		ui.Text(c, turn.reply).FontSize(12.5).LineHeight(1.6).TextColor(t.TextMuted).MaxLines(3)
	})
	if h := card.Bounds().H; h != a.cardHeight {
		a.cardHeight = h
		c.After(0)
	}
}
