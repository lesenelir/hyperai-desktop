package main

import (
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
	t.Cleanup(func() { a.saver.wait() })
	tt := ui.NewTester(a.view, 1120, 760)
	for _, q := range []string{"First", "Second", "Third"} {
		a.newChat()
		a.send(q)
	}
	tt.Frame()
	today := "[" + locales.GroupToday() + "]"
	if got := sidebar(a); !slices.Equal(got, []string{today, "Third", "Second", "First"}) {
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
	if got := sidebar(a); !slices.Equal(got, []string{pinned, "Second", "First", today, "Third"}) {
		t.Errorf("pinned: rows %q", got)
	}
	if !tt.HasText(locales.GroupPinned()) {
		t.Errorf("no Pinned group in %q", tt.Texts())
	}

	// The title bar's menu says Unpin for the pinned chat shown.
	a.open("1") // First
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
	if got := sidebar(a); !slices.Equal(got, []string{pinned, "Second", today, "Third", "First"}) {
		t.Errorf("unpinned: rows %q", got)
	}

	// Kept with the chats.
	a.saver.wait()
	again := newApp(&counter{})
	again.loadFrom(dir)
	if got := sidebar(again); !slices.Equal(got, []string{pinned, "Second", today, "Third", "First"}) {
		t.Errorf("kept: rows %q", got)
	}
}
