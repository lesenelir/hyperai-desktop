package main

import "github.com/egoist/mygo/ui"

// The conversation list's room around its rows, which the pin measures
// with; the composer's height adds to its bottom.
const (
	listPadTop    = 20
	listPadBottom = 24
	listGap       = 24
)

// pin holds the user's latest question at the top of the conversation while
// its reply comes in under it, as hyperai-next's agent does. The list
// scrolls the question to its top, and stops following its end, so that it
// keeps it there; room after the last row lets it scroll that far, and
// shrinks as the reply fills it. Once the reply outgrows the room, the pin
// lets go and the list follows the reply's end again.
type pin struct {
	id   ID      // the question's message ID, "" for none
	room float32 // the room after the last row; -1 until measured
	y    float32 // where the list is scrolled with the question at its top; -1 until measured
}

// pinTop pins th's message i, a question just sent, to the top.
func (th *thread) pinTop(i int) {
	th.pin = pin{id: th.Messages[i].ID, room: -1, y: -1}
	th.list.ScrollTo(i, ui.Start)
}

// row returns the row of the question pinned in msgs, -1 for none.
func (p *pin) row(msgs []*message) int {
	if p.id == "" {
		return -1
	}
	for i, m := range msgs {
		if m.ID == p.id {
			return i
		}
	}
	p.id = "" // the question is gone
	return -1
}

// held reports whether the list shows the question at the top where the
// pin put it, scrolled y: the end of the list, as far as the user sees.
func (p *pin) held(y float32) bool {
	return p.id != "" && (p.y < 0 || y >= p.y-1 && y <= p.y+1)
}

// measure returns the room the list leaves after its last row this frame,
// and whether the reply outgrew it, from what the last frame laid out: the
// list's height, less what the question q and the rows after it take. The
// list is scrolled y and is in box, empty before it was laid out, with
// padding bottom under its last row; the window is h tall.
func (th *thread) measure(q int, y, h float32, box ui.Rect, bottom float32) (room float32, outgrown bool) {
	p := &th.pin
	n := len(th.Messages)
	question, last := th.box(q), th.box(n-1)
	switch {
	case empty(question) && p.room < 0:
		// Not laid out yet: the list's height is room enough to scroll the
		// question to the top, which the next frame measures.
		if !empty(box) {
			h = box.H
		}
		return h, false
	case !p.held(y) || empty(question):
		// Scrolled away from the question: the room stays as it was, which
		// the rows out of sight cannot tell.
		return max(p.room, 0), false
	case empty(last):
		// The last row is below the list's bottom.
		return 0, true
	}
	below := last.Y + last.H - question.Y
	p.room = box.H - listPadTop - below - listGap - bottom
	if p.room <= 0 {
		return 0, true
	}
	if p.y < 0 {
		p.y = y
	}
	return p.room, false
}

// glideTo scrolls the list smoothly until row i is at its top, as a
// browser's smooth scroll does: by how far the row is from there, from the
// rows the last frame laid out, and the heights of those laid out before
// (estimated for rows never laid out). Where that was off, the list puts
// the row at the top as the glide ends (settle).
func (th *thread) glideTo(i int) {
	ref := -1
	if !empty(th.box(i)) {
		ref = i
	} else {
		for j, b := range th.boxes {
			if !empty(b) {
				ref = j
				break
			}
		}
	}
	if ref < 0 {
		th.list.ScrollTo(i, ui.Start)
		return
	}
	height := func(j int) float32 {
		if h := th.Messages[j].height; h > 0 {
			return h
		}
		return 120 // never laid out: about a short message
	}
	d := th.boxes[ref].Y - th.listBox.Y - listPadTop
	for j := i; j < ref; j++ {
		d -= height(j) + listGap
	}
	for j := ref; j < i; j++ {
		d += height(j) + listGap
	}
	th.scroll.ScrollTo(0, max(0, th.scroll.Y+d), ui.Smooth)
	th.seek = i + 1
}

// settle puts the row a glide went to at the top once the glide ends, if it
// is not there.
func (th *thread) settle() {
	if th.seek == 0 || th.scroll.Scrolling() {
		return
	}
	i := th.seek - 1
	th.seek = 0
	if i >= len(th.Messages) {
		return
	}
	if b := th.box(i); empty(b) || b.Y-th.listBox.Y-listPadTop > 0.5 || b.Y-th.listBox.Y-listPadTop < -0.5 {
		th.list.ScrollTo(i, ui.Start)
	}
}

// box returns where the last frame laid out row i, empty for a row it did
// not build.
func (th *thread) box(i int) ui.Rect {
	if i < 0 || i >= len(th.boxes) {
		return ui.Rect{}
	}
	return th.boxes[i]
}

func empty(r ui.Rect) bool { return r.W == 0 && r.H == 0 }
