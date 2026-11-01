package main

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// sidebar returns the sidebar's rows, as their groups' titles and the
// chats' titles.
func sidebar(a *app) []string {
	var out []string
	for _, r := range a.chatRows(time.Now()) {
		if r.th != nil {
			out = append(out, r.th.Title)
		} else {
			out = append(out, "["+r.group+"]")
		}
	}
	return out
}

// A chat pinned from its menu goes to the sidebar's top, in Pinned, the
// latest pinned first, and out of its day; unpinned, back to its day. The
// chats keep it.
func TestPinChat(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.loadFrom(dir)
	t.Cleanup(func() { a.logs.wait() })
	tt := ui.NewTester(a.view, 1120, 760)
	for _, q := range []string{"First", "Second", "Third"} {
		a.newChat()
		a.send(q)
	}
	tt.Frame()
	today, chats := "["+locales.GroupToday()+"]", "["+locales.SidebarChats()+"]"
	if got := sidebar(a); !slices.Equal(got, []string{chats, today, "Third", "Second", "First"}) {
		t.Fatalf("rows %q", got)
	}

	// From the sidebar's menu: the first, then the second.
	for _, title := range []string{"First", "Second"} {
		if err := tt.RightClick(title); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(locales.PinChat()); err != nil {
			t.Fatalf("%v; menu %q", err, tt.Menu())
		}
		tt.Advance(time.Second)
	}
	pinned := "[" + locales.GroupPinned() + "]"
	if got := sidebar(a); !slices.Equal(got, []string{pinned, "Second", "First", chats, today, "Third"}) {
		t.Errorf("pinned: rows %q", got)
	}
	if !tt.HasText(locales.GroupPinned()) {
		t.Errorf("no Pinned group in %q", tt.Texts())
	}

	// The title bar's menu says Unpin for the pinned chat shown.
	a.open(string(a.threads[0].ID)) // First
	tt.Frame()
	if err := tt.Click(locales.More()); err != nil {
		t.Fatal(err)
	}
	if got := tt.Menu(); !slices.Contains(got, locales.UnpinChat()) || slices.Contains(got, locales.PinChat()) {
		t.Fatalf("menu %q", got)
	}
	if err := tt.ChooseMenuItem(locales.UnpinChat()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := sidebar(a); !slices.Equal(got, []string{pinned, "Second", chats, today, "Third", "First"}) {
		t.Errorf("unpinned: rows %q", got)
	}

	// Kept with the chats.
	a.logs.wait()
	again := newApp(&counter{})
	again.loadFrom(dir)
	if got := sidebar(again); !slices.Equal(got, []string{pinned, "Second", chats, today, "Third", "First"}) {
		t.Errorf("kept: rows %q", got)
	}
}

// Pinning a chat glides the sidebar to its top, where the chat went, out
// of view where the list kept its place; unpinning leaves the sidebar
// where it is.
func TestPinScrollsToTop(t *testing.T) {
	a := newApp(&counter{})
	tt := ui.NewTester(a.view, 1120, 760)
	for i := range 25 {
		a.newChat()
		a.send(fmt.Sprintf("Chat %02d", i+1))
	}
	tt.Frame()
	a.chatScroll.ScrollTo(0, 600, ui.Animation{})
	tt.Advance(time.Second)
	if a.chatScroll.Y < 100 {
		t.Fatalf("the sidebar did not scroll: %v of %v", a.chatScroll.Y, a.chatScroll.MaxY)
	}
	r, ok := tt.Find("Chat 10")
	if !ok || r.Y < 0 || r.Y > 760 {
		t.Fatalf("Chat 10 out of view at %v", r)
	}
	if err := tt.RightClick("Chat 10"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.PinChat()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	// A glide, which settles within a DIP of its end.
	if a.chatScroll.Y > 1 {
		t.Errorf("pinned: the sidebar at %v", a.chatScroll.Y)
	}
	p, _ := tt.Find(locales.GroupPinned())
	r, _ = tt.Find("Chat 10")
	if !tt.HasText(locales.GroupPinned()) || p.Y < 0 || r.Y < p.Y || r.Y > p.Y+60 {
		t.Errorf("pinned: its group at %v, the chat at %v", p, r)
	}

	// Unpinned, from far down: the sidebar stays.
	a.chatScroll.ScrollTo(0, 600, ui.Animation{})
	tt.Advance(time.Second)
	y := a.chatScroll.Y
	a.togglePin(a.thread(string(a.threads[9].ID)))
	tt.Advance(time.Second)
	if a.chatScroll.Y == 0 || a.chatScroll.Y < y-100 {
		t.Errorf("unpinned: the sidebar went from %v to %v", y, a.chatScroll.Y)
	}
}

// The sidebar's sections, Pinned and Chats, fold and unfold as their
// titles are clicked, the chats in them gone while folded; layout.json
// keeps it. Pinning a chat unfolds Pinned, to show where it went.
func TestFoldSections(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.loadFrom(dir)
	t.Cleanup(func() { a.logs.wait(); a.layoutSaver.wait() })
	tt := ui.NewTester(a.view, 1120, 760)
	for _, q := range []string{"First", "Second"} {
		a.newChat()
		a.send(q)
	}
	a.togglePin(a.threads[0])
	tt.Advance(time.Second)
	pinned, chats, today := "["+locales.GroupPinned()+"]", "["+locales.SidebarChats()+"]", "["+locales.GroupToday()+"]"
	if got := sidebar(a); !slices.Equal(got, []string{pinned, "First", chats, today, "Second"}) {
		t.Fatalf("rows %q", got)
	}

	// Chats, folded with its title, over Today's (13 over its row; 26 under
	// Pinned's, folded): the list, named Chats too, is found by the name
	// first.
	clickChats := func(under ui.Rect, by float32) {
		t.Helper()
		tt.ClickAt(30, under.Y+by) // past the row's margin
		tt.Advance(time.Second)
	}
	td, _ := tt.Find(locales.GroupToday())
	clickChats(td, -13)
	if got := sidebar(a); !slices.Equal(got, []string{pinned, "First", chats}) || tt.HasText(locales.GroupToday()) {
		t.Errorf("Chats folded: rows %q", got)
	}
	// Pinned too, and both kept.
	if err := tt.Click(locales.GroupPinned()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := sidebar(a); !slices.Equal(got, []string{pinned, chats}) {
		t.Errorf("both folded: rows %q", got)
	}
	a.layoutSaver.wait()
	again := newApp(&counter{})
	again.loadFrom(dir)
	if !again.foldPinned || !again.foldChats {
		t.Errorf("kept: pinned %v, chats %v", again.foldPinned, again.foldChats)
	}

	// Pinning unfolds Pinned; Chats, of none left, goes.
	a.togglePin(a.threads[1])
	tt.Advance(time.Second)
	if got := sidebar(a); !slices.Equal(got, []string{pinned, "Second", "First"}) {
		t.Errorf("pinned while folded: rows %q", got)
	}

	// Pinned folded again, Chats unfolded under it, then Pinned.
	a.togglePin(a.threads[1])
	tt.Advance(time.Second)
	if err := tt.Click(locales.GroupPinned()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	p, _ := tt.Find(locales.GroupPinned())
	clickChats(p, p.H+26)
	if err := tt.Click(locales.GroupPinned()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := sidebar(a); !slices.Equal(got, []string{pinned, "First", chats, today, "Second"}) {
		t.Errorf("unfolded: rows %q", got)
	}
}
