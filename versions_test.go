package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// counter replies "Answer n", n counting its replies, keeping what it was
// asked.
type counter struct {
	n     int
	asked [][]turn
}

func (p *counter) ID() string                                 { return "counter" }
func (p *counter) Name() string                               { return "Counter" }
func (p *counter) Models() []string                           { return []string{"counter"} }
func (p *counter) List(ctx context.Context) ([]string, error) { return p.Models(), nil }

func (p *counter) Stream(ctx context.Context, model string, turns []turn, delta func(chunk)) error {
	p.n++
	p.asked = append(p.asked, turns)
	delta(chunk{Text: "Answer " + strconv.Itoa(p.n)})
	return nil
}

// path returns th's path shown, as its messages' texts.
func path(th *thread) string {
	var out []string
	for _, m := range th.Messages {
		out = append(out, m.Text)
	}
	return strings.Join(out, " | ")
}

// asked returns what turns said, as their texts.
func asked(turns []turn) string {
	var out []string
	for _, t := range turns {
		out = append(out, t.Content)
	}
	return strings.Join(out, " | ")
}

// A reply asked for again is a version beside it, which keeps what
// followed it, and the model is asked the conversation before it alone;
// going back to it shows the conversation as it went from it.
func TestRegenerate(t *testing.T) {
	p := &counter{}
	a := newApp(p)
	a.send("Q1")
	a.send("Q2")
	th := a.current()
	if got := path(th); got != "Q1 | Answer 1 | Q2 | Answer 2" {
		t.Fatalf("path %q", got)
	}

	a.regenerate(th, 1)
	if got := path(th); got != "Q1 | Answer 3" {
		t.Fatalf("regenerated: path %q", got)
	}
	if got := asked(p.asked[2]); got != "Q1" {
		t.Errorf("asked %q", got)
	}
	if vs := th.versions(th.Messages[1]); len(vs) != 2 || vs[0].Text != "Answer 1" || vs[1].Text != "Answer 3" {
		t.Errorf("versions %v", vs)
	}
	if len(th.Others) != 3 {
		t.Errorf("others %d, want 3: the first reply and what followed it", len(th.Others))
	}

	// Back to the first, and what followed it.
	a.showVersion(th, 1, th.versions(th.Messages[1])[0])
	if got := path(th); got != "Q1 | Answer 1 | Q2 | Answer 2" {
		t.Errorf("shown again: path %q", got)
	}
	// The last reply, again: the next question asks what shows alone.
	a.regenerate(th, 3)
	a.send("Q3")
	if got := asked(p.asked[len(p.asked)-1]); got != "Q1 | Answer 1 | Q2 | Answer 4 | Q3" {
		t.Errorf("asked %q", got)
	}
	// Forward to the second version of the first reply, and back: the
	// path left last shows again, Q3 and all.
	a.showVersion(th, 1, th.versions(th.Messages[1])[1])
	a.showVersion(th, 1, th.versions(th.Messages[1])[0])
	if got := path(th); got != "Q1 | Answer 1 | Q2 | Answer 4 | Q3 | Answer 5" {
		t.Errorf("back again: path %q", got)
	}

	// Not while a reply streams.
	th.busy = &message{}
	if a.regenerate(th, 1); path(th) != "Q1 | Answer 1 | Q2 | Answer 4 | Q3 | Answer 5" {
		t.Errorf("regenerated while busy: %q", path(th))
	}
	th.busy = nil
}

// A question edited, with the button beside it, is sent again as a new
// version, which keeps the one it replaces and what followed it; the
// switcher under it shows them in turn. Escape cancels.
func TestEditQuestion(t *testing.T) {
	p := &counter{}
	a := newApp(p)
	tt := ui.NewTester(a.view, 1120, 760)
	tt.Type("Q1")
	tt.Key(0, ui.KeyEnter)
	tt.Type("Q2")
	tt.Key(0, ui.KeyEnter)
	th := a.current()
	edit := func(q string) {
		t.Helper()
		r, ok := tt.Find(q)
		if !ok {
			t.Fatalf("no %q in %q", q, tt.Texts())
		}
		tt.Move(r.X+4, r.Y+r.H/2)
		tt.Advance(time.Second)
		// Beside its bubble, past the copy button next to it: every
		// question has one.
		copyX := r.X - bubblePadX - 6 - actionSize/2
		tt.ClickAt(copyX-actionSize-barGap, r.Y+r.H/2)
		tt.Frame()
		if a.editingMsg == "" || a.editText != q {
			t.Fatalf("editing %s, holding %q", a.editingMsg, a.editText)
		}
	}

	// Escape: as it was.
	edit("Q2")
	tt.Type(" more")
	tt.Key(0, ui.KeyEscape)
	if a.editingMsg != "" || path(th) != "Q1 | Answer 1 | Q2 | Answer 2" {
		t.Fatalf("after Escape: editing %s, path %q", a.editingMsg, path(th))
	}

	// The first question, edited: a version of it, the rest kept. Scrolled
	// back to, under another title than its text, which the sidebar shows.
	th.Title = "Chat"
	th.pin = pin{}
	th.scroll.ScrollTo(0, 0, ui.Animation{})
	tt.Advance(time.Second)
	edit("Q1")
	tt.Type(" again")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if got := path(th); got != "Q1 again | Answer 3" {
		t.Fatalf("edited: path %q", got)
	}
	if got := asked(p.asked[2]); got != "Q1 again" {
		t.Errorf("asked %q", got)
	}
	if !tt.HasText("2 / 2") {
		t.Fatalf("no switcher in %q", tt.Texts())
	}
	if err := tt.Click(locales.PreviousVersion()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := path(th); got != "Q1 | Answer 1 | Q2 | Answer 2" || !tt.HasText("1 / 2") {
		t.Errorf("previous: path %q; texts %q", got, tt.Texts())
	}
}

// The versions are kept with the chat, and a chat kept before them reads
// as one of each.
func TestVersionsKept(t *testing.T) {
	dir := t.TempDir()
	p := &counter{}
	a := newApp(p)
	a.loadFrom(dir)
	t.Cleanup(func() { a.logs.wait() })
	a.send("Q1")
	a.regenerate(a.current(), 1)
	a.logs.wait()

	again := newApp(p)
	again.loadFrom(dir)
	th := again.threads[0]
	if got := path(th); got != "Q1 | Answer 2" || len(th.versions(th.Messages[1])) != 2 {
		t.Fatalf("kept: path %q, versions %d", got, len(th.versions(th.Messages[1])))
	}
	again.showVersion(th, 1, th.versions(th.Messages[1])[0])
	if got := path(th); got != "Q1 | Answer 1" {
		t.Errorf("shown: path %q", got)
	}

	old := t.TempDir()
	os.WriteFile(filepath.Join(old, "chats.json"), []byte(`[{"id":1,"title":"Old","messages":[`+
		`{"id":2,"role":"user","text":"Q"},{"id":3,"role":"assistant","text":"A"},`+
		`{"id":4,"role":"user","text":"Q2"},{"id":5,"role":"assistant","text":"A2"}]}]`), 0o600)
	b := newApp(p)
	b.loadFrom(old)
	th = b.threads[0]
	for i, m := range th.Messages {
		if want := ID(""); i > 0 && m.Parent != th.Messages[i-1].ID || i == 0 && m.Parent != want {
			t.Errorf("message %d follows %q", i, m.Parent)
		}
	}
	if vs := th.versions(th.Messages[2]); len(vs) != 1 {
		t.Errorf("versions of an old chat's message: %d", len(vs))
	}
}

// The buttons that change the conversation change it once the list that
// holds them is built: a version, a reply asked for again or a question
// sent again shows fewer messages than the list had rows as it was built,
// which indexed past them.
func TestVersionButtonsShortenPath(t *testing.T) {
	p := &counter{}
	a := newApp(p)
	tt := ui.NewTester(a.view, 1120, 760)
	a.send("Q1")
	a.send("Q2")
	th := a.current()
	th.Title = "Chat"
	th.pin = pin{}
	th.scroll.ScrollTo(0, 0, ui.Animation{})
	tt.Advance(time.Second)

	// The first reply, asked for again with its button.
	r, ok := tt.Find("Answer 1")
	if !ok {
		t.Fatalf("no first reply in %q", tt.Texts())
	}
	tt.Move(r.X+4, r.Y+r.H/2)
	tt.Advance(time.Second)
	if err := tt.Click(locales.Regenerate()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := path(th); got != "Q1 | Answer 3" {
		t.Fatalf("regenerated: path %q", got)
	}

	// Back to the longer version, then forward to the shorter one.
	if err := tt.Click(locales.PreviousVersion()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := path(th); got != "Q1 | Answer 1 | Q2 | Answer 2" {
		t.Fatalf("previous: path %q", got)
	}
	if err := tt.Click(locales.NextVersion()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := path(th); got != "Q1 | Answer 3" {
		t.Fatalf("next: path %q", got)
	}

	// Back again, and the first question sent again with Send.
	if err := tt.Click(locales.PreviousVersion()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	th.pin = pin{}
	th.scroll.ScrollTo(0, 0, ui.Animation{})
	tt.Advance(time.Second)
	// The first question's Edit, the first in view.
	if err := tt.Click(locales.EditMessage()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Type(" changed")
	if !tt.HasText(locales.Cancel()) {
		t.Fatalf("the question is not edited in view: %q", tt.Texts())
	}
	// The edited question's Send, in the list, before the composer's.
	if err := tt.Click(locales.Send()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := path(th); got != "Q1 changed | Answer 4" {
		t.Errorf("sent again: path %q", got)
	}
}
