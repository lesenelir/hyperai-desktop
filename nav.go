package main

import (
	"runtime"
	"slices"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// history is the chats shown before the one shown, which Back returns to,
// and those after it, which Forward goes to again, as Waku's
// SessionNavigation keeps its tasks. An entry is what app.selected holds:
// a chat's ID, "" for a new chat.
type history struct {
	back, forward []string
}

// visit goes from the chat cur to next: cur is the one to go back to, and
// what was forward of it is gone, as in a browser.
func (h *history) visit(cur, next string) {
	if cur == next {
		return
	}
	h.back = append(h.back, cur)
	h.forward = h.forward[:0]
}

// goBack returns the chat to go back to from cur, which forward then holds;
// false for none.
func (h *history) goBack(cur string) (string, bool) {
	return step(&h.back, &h.forward, cur)
}

// goForward returns the chat to go forward to from cur, which back then
// holds; false for none.
func (h *history) goForward(cur string) (string, bool) {
	return step(&h.forward, &h.back, cur)
}

// step takes the last of from, and puts cur on to.
func step(from, to *[]string, cur string) (string, bool) {
	n := len(*from)
	if n == 0 {
		return "", false
	}
	next := (*from)[n-1]
	*from = (*from)[:n-1]
	*to = append(*to, cur)
	return next, true
}

// remove forgets the chat id, now that the chat cur shows: and with it the
// steps it leaves that go nowhere, to the chat beside them or to cur.
func (h *history) remove(id, cur string) {
	prune := func(s []string) []string {
		s = slices.DeleteFunc(s, func(e string) bool { return e == id })
		s = slices.Compact(s)
		for len(s) > 0 && s[len(s)-1] == cur {
			s = s[:len(s)-1]
		}
		return s
	}
	h.back, h.forward = prune(h.back), prune(h.forward)
}

// open shows the chat id ("" for a new one), which Back then returns from.
func (a *app) open(id string) {
	a.nav.visit(a.selected, id)
	a.show(id)
	a.focusDraft, a.settings = true, false
}

// goBack shows the chat shown before, if any.
func (a *app) goBack() {
	if id, ok := a.nav.goBack(a.selected); ok {
		a.show(id)
		a.focusDraft, a.settings = true, false
	}
}

// goForward shows again the chat gone back from, if any.
func (a *app) goForward() {
	if id, ok := a.nav.goForward(a.selected); ok {
		a.show(id)
		a.focusDraft, a.settings = true, false
	}
}

// show makes id the chat shown ("" for a new one), with the draft its
// composer held: what the composer holds stays with the chat it was
// written in, at once, so that what is written next goes to the chat
// shown, as the search's query does.
func (a *app) show(id string) {
	if id == a.selected {
		return
	}
	a.keepDraft()
	a.selected = id
	a.draft = a.newDraft
	if th := a.current(); th != nil {
		a.draft = th.Draft
	}
	// From its first line.
	a.fieldScroll = ui.ScrollState{}
}

// keepDraft keeps what the composer holds with the chat shown, saving the
// chats where a chat's changed: a new chat's is not kept past the app.
func (a *app) keepDraft() {
	th := a.current()
	if th == nil {
		a.newDraft = a.draft
		return
	}
	if th.Draft != a.draft {
		th.Draft = a.draft
		a.save()
	}
}

// historyKeys goes back and forward on the keys a browser does, as MyGo's
// Router takes them: Cmd+[ and Cmd+] on macOS, Alt+Left and Alt+Right
// elsewhere, and everywhere the side buttons of a mouse and, on macOS, the
// swipes between pages.
func (a *app) historyKeys(c *ui.Context) {
	back, forward := c.Shortcut(0, ui.KeyBack), c.Shortcut(0, ui.KeyForward)
	if runtime.GOOS == "darwin" {
		back = c.Shortcut(ui.Super, ui.KeyBracketLeft) || back
		forward = c.Shortcut(ui.Super, ui.KeyBracketRight) || forward
	} else {
		back = c.Shortcut(ui.Alt, ui.KeyLeft) || back
		forward = c.Shortcut(ui.Alt, ui.KeyRight) || forward
	}
	if back {
		a.goBack()
	}
	if forward {
		a.goForward()
	}
}

// historyButtons builds Back and Forward beside the sidebar's toggle, at x
// in the title bar, as Waku has them: dimmed where there is nowhere to go.
func (a *app) historyButtons(c *ui.Context, x float32) {
	for i, b := range []struct {
		svg      *ui.SVG
		label    string
		shortcut string
		can      bool
		do       func()
	}{
		{arrowLeftIcon, locales.Back(), backKeys.String(), len(a.nav.back) > 0, a.goBack},
		{arrowRightIcon, locales.Forward(), forwardKeys.String(), len(a.nav.forward) > 0, a.goForward},
	} {
		btn := iconButtonSized(c.Key(i), b.svg, locales.WithKeys(b.label, b.shortcut), titleIconSize).
			Absolute().Left(x + float32(i)*(30+2)).Top((titleBarHeight - 28) / 2).Disabled(!b.can)
		if !b.can {
			btn.Opacity(0.35)
		}
		if btn.Clicked() {
			b.do()
		}
	}
}
