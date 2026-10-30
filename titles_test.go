package main

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// namer replies to questions, and names chats as a model does, round its
// title what models put there; as Anthropic, whose cheap model it fails
// with where cheapFails, and every title where titleFails.
type namer struct {
	title      string
	cheapFails bool
	titleFails bool
	titled     []string // the models asked for a title
}

func (p *namer) ID() string                                 { return "anthropic" }
func (p *namer) Name() string                               { return "Namer" }
func (p *namer) Models() []string                           { return []string{"claude-opus-5-5"} }
func (p *namer) List(ctx context.Context) ([]string, error) { return p.Models(), nil }

func (p *namer) Stream(ctx context.Context, model string, turns []turn, delta func(chunk)) error {
	if !strings.HasPrefix(turns[0].Content, titlePrompt) {
		delta(chunk{Text: "Sure."})
		return nil
	}
	p.titled = append(p.titled, model)
	if p.titleFails || p.cheapFails && model == "claude-haiku-5-5" {
		return errors.New("404 Not Found: no such model")
	}
	delta(chunk{Reasoning: "A short name."})
	delta(chunk{Text: p.title})
	return nil
}

// A chat started is named by the cheap model of its provider, in place of
// its first message's runes; by the chat's model where the cheap one
// fails; and keeps them where both do.
func TestNameChat(t *testing.T) {
	first := "How do I make the composer in my MyGo app send on Enter but break the line on Shift+Enter?"
	for _, c := range []struct {
		name   string
		p      *namer
		want   string
		models []string
	}{
		{"cheap", &namer{title: "**Title:** \"Composer Enter key\"\nmore"}, "Composer Enter key", []string{"claude-haiku-5-5"}},
		{"fallback", &namer{title: "回车发送消息。", cheapFails: true}, "回车发送消息", []string{"claude-haiku-5-5", "claude-opus-5-5"}},
		{"failed", &namer{titleFails: true}, title(first), []string{"claude-haiku-5-5", "claude-opus-5-5"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := newApp(c.p)
			a.nameChats = true
			a.send(first)
			th := a.current()
			if th.Title != c.want || strings.Join(c.p.titled, ",") != strings.Join(c.models, ",") {
				t.Errorf("title %q of %v, want %q of %v", th.Title, c.p.titled, c.want, c.models)
			}
			// Only as the chat starts.
			a.send("And on Cmd+Enter?")
			if len(c.p.titled) != len(c.models) {
				t.Errorf("asked again: %v", c.p.titled)
			}
		})
	}
}

// Files alone keep the first's name; a title lands nowhere on a chat
// deleted or named otherwise meanwhile.
func TestTitleLands(t *testing.T) {
	_, text, _, _ := testFiles(t)
	p := &namer{title: "Notes"}
	a := newApp(p)
	a.nameChats = true
	a.attach([]string{text})
	a.send("")
	if th := a.current(); th.Title != "notes.md" || len(p.titled) != 0 {
		t.Errorf("of files alone: title %q, asked %v", th.Title, p.titled)
	}

	th := &thread{ID: 7, Title: "How do I cache?"}
	a.threads = append(a.threads, th)
	rev := a.rev
	a.titled(7, "How do I cache?", "Caching")
	if th.Title != "Caching" || a.rev == rev {
		t.Errorf("title %q, rev %d after %d", th.Title, a.rev, rev)
	}
	a.titled(7, "How do I cache?", "Something else")
	if th.Title != "Caching" {
		t.Errorf("named over another title: %q", th.Title)
	}
	a.titled(8, "Gone", "Name")
}

// What a model says is made a title, or none where it answered the message.
func TestNormalizeTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Caching in Go":                     "Caching in Go",
		"  \"Caching in Go.\"  ":            "Caching in Go",
		"**Title:** Caching in Go":          "Caching in Go",
		"# 缓存设计\n\n解释：……":                   "缓存设计",
		"标题：「回车发送」。":                        "回车发送",
		"タイトル：『キャッシュ』":                      "キャッシュ",
		"":                                  "",
		"\"\"":                              "",
		strings.Repeat("A long answer ", 8): "",
		"Caching… and more":                 "Caching… and more",
	} {
		if got := normalizeTitle(in); got != want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

// A title comes after the reply began, not before: it runs beside it, on
// its own goroutine, and lands through post.
func TestTitleBesideReply(t *testing.T) {
	p := &namer{title: "Caching"}
	a := newApp(p)
	a.nameChats = true
	posts := make(chan func(), 16)
	a.post = func(f func()) { posts <- f }
	a.send("How do I cache?")
	th := a.current()
	if th.Title != "How do I cache?" || th.busy == nil {
		t.Fatalf("title %q, replying %v as it is sent", th.Title, th.busy != nil)
	}
	deadline := time.After(5 * time.Second)
	for th.Title != "Caching" || th.busy != nil {
		select {
		case f := <-posts:
			f()
		case <-deadline:
			t.Fatalf("title %q, replying %v", th.Title, th.busy != nil)
		}
	}
}

// The Default model page chooses the model that names new chats, of each
// chat's provider's cheapest at first, and turns the names off, which
// fades the choice out of reach; providers.json keeps both.
func TestTitleSettings(t *testing.T) {
	dir := t.TempDir()
	p := &namer{title: "Caching"}
	a := newApp(p)
	a.loadFrom(dir)
	a.nameChats = true
	tt := ui.NewTester(a.view, 1120, 760)
	a.openSettings()
	a.settingsPage = "model"
	tt.Frame()
	for _, s := range []string{locales.TitlesCard(), locales.TitlesRow(), locales.TitleModelRow(), locales.TitleModelAuto()} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	toggle(t, tt, locales.TitleModelRow(), locales.TitleModelAbout())
	// Automatic, the one chosen at first, then the models offered.
	if got, want := tt.Menu(), []string{locales.TitleModelAuto(), "-", "claude-opus-5-5"}; !slices.Equal(got, want) {
		t.Fatalf("menu %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem("claude-opus-5-5"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.titleModel != (modelRef{"anthropic", "claude-opus-5-5"}) {
		t.Fatalf("title model %+v", a.titleModel)
	}
	// Asked in place of the provider's cheapest.
	a.closeSettings()
	a.send("How do I cache?")
	if strings.Join(p.titled, ",") != "claude-opus-5-5" || a.current().Title != "Caching" {
		t.Errorf("asked %v, title %q", p.titled, a.current().Title)
	}

	// Off: no chat is named, and the model is out of reach.
	a.openSettings()
	a.settingsPage = "model"
	tt.Frame()
	toggle(t, tt, locales.TitlesRow(), locales.TitlesAbout())
	if !a.noTitles {
		t.Fatal("the switch did not turn the titles off")
	}
	tt.CloseMenu()
	toggle(t, tt, locales.TitleModelRow(), locales.TitleModelAbout())
	if m := tt.Menu(); len(m) != 0 {
		t.Errorf("the title model opens while off: %q", m)
	}
	a.closeSettings()
	a.newChat()
	a.send("Another question")
	if len(p.titled) != 1 || a.current().Title != "Another question" {
		t.Errorf("named while off: asked %v, title %q", p.titled, a.current().Title)
	}

	// Kept for the next start.
	a.confSaver.wait()
	again := newApp(p)
	again.loadConf(dir)
	if !again.noTitles || again.titleModel != (modelRef{"anthropic", "claude-opus-5-5"}) {
		t.Errorf("kept noTitles %v, model %+v", again.noTitles, again.titleModel)
	}
	// A provider removed takes its model with it: Automatic again.
	again.removeProvider("anthropic")
	if again.titleModel != (modelRef{}) {
		t.Errorf("title model %+v after its provider went", again.titleModel)
	}
}

// The title bar's menu turns the title into a field, its text chosen:
// Return keeps what it holds, Escape what was, and so does a field left
// empty; clicking away keeps it too.
func TestRenameInTitleBar(t *testing.T) {
	p := &namer{title: "Caching"}
	a := newApp(p)
	tt := ui.NewTester(a.view, 1120, 760)
	tt.Type("How do I cache?")
	tt.Key(0, ui.KeyEnter)
	th := a.current()
	rename := func() {
		t.Helper()
		if err := tt.Click(locales.More()); err != nil {
			t.Fatal(err)
		}
		if got := tt.Menu(); !slices.Contains(got, locales.RenameChatItem()) || !slices.Contains(got, locales.RegenerateTitleItem()) {
			t.Fatalf("menu %q", got)
		}
		if err := tt.ChooseMenuItem(locales.RenameChatItem()); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.renaming == "" || a.renameInSidebar || a.renameDraft != th.Title {
			t.Fatalf("renaming %q in the sidebar %v, holding %q", a.renaming, a.renameInSidebar, a.renameDraft)
		}
	}

	// Typed over the title, which is chosen.
	rename()
	tt.Type("Cache  notes ")
	tt.Key(0, ui.KeyEnter)
	if th.Title != "Cache notes" || a.renaming != "" {
		t.Fatalf("title %q, renaming %q", th.Title, a.renaming)
	}
	if _, ok := tt.Find("Cache notes"); !ok {
		t.Errorf("the title bar shows %q", tt.Texts())
	}

	// Escape: as it was.
	rename()
	tt.Type("Something else")
	tt.Key(0, ui.KeyEscape)
	if th.Title != "Cache notes" || a.renaming != "" {
		t.Errorf("after Escape: title %q, renaming %q", th.Title, a.renaming)
	}

	// Left empty: as it was.
	rename()
	tt.Key(0, ui.KeyBackspace)
	tt.Key(0, ui.KeyEnter)
	if th.Title != "Cache notes" {
		t.Errorf("emptied: title %q", th.Title)
	}

	// Clicking away keeps it.
	rename()
	tt.Type("Caches")
	r, _ := tt.Find(locales.ComposerPlaceholder())
	tt.ClickAt(r.X+10, r.Y+r.H/2)
	tt.Frame()
	if th.Title != "Caches" || a.renaming != "" {
		t.Errorf("clicked away: title %q, renaming %q", th.Title, a.renaming)
	}
}

// A row of the sidebar is renamed in place, from its menu.
func TestRenameInSidebar(t *testing.T) {
	a, tt := newTester()
	tt.Type("How do I cache?")
	tt.Key(0, ui.KeyEnter)
	th := a.current()
	if err := tt.RightClick("How do I cache?"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.RenameChatItem()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	if !a.renameInSidebar || a.renaming != strconv.Itoa(th.ID) {
		t.Fatalf("renaming %q in the sidebar %v", a.renaming, a.renameInSidebar)
	}
	tt.Type("LRU cache")
	tt.Key(0, ui.KeyEnter)
	if th.Title != "LRU cache" {
		t.Errorf("title %q", th.Title)
	}
	// Hiding the sidebar as it is renamed keeps what its field holds.
	a.startRename(th, true)
	a.renameDraft = "LRU caches"
	tt.Frame()
	a.collapsed = true
	tt.Advance(time.Second)
	if th.Title != "LRU caches" || a.renaming != "" {
		t.Errorf("hidden: title %q, renaming %q", th.Title, a.renaming)
	}
}

// Regenerate Title asks anew, over a name the user gave too, once at a
// time, and says why where it fails.
func TestRegenerateTitle(t *testing.T) {
	p := &namer{title: "Caching"}
	a := newApp(p)
	tt := ui.NewTester(a.view, 1120, 760)
	a.send("How do I cache?")
	tt.Frame()
	th := a.current()
	th.Title = "Mine"
	if !a.canRetitle(th) {
		t.Fatal("cannot regenerate the title")
	}
	if err := tt.Click(locales.More()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.RegenerateTitleItem()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if th.Title != "Caching" || th.naming || strings.Join(p.titled, ",") != "claude-haiku-5-5" {
		t.Fatalf("title %q, naming %v, asked %v", th.Title, th.naming, p.titled)
	}
	// Once at a time.
	th.naming = true
	if a.canRetitle(th) {
		t.Error("regenerates while one is asked for")
	}
	th.naming = false

	p.titleFails = true
	a.retitle(th)
	tt.Frame()
	if th.Title != "Caching" || !tt.HasText(locales.TitleFailed()) || !tt.HasText("no such model") {
		t.Errorf("title %q; texts %q", th.Title, tt.Texts())
	}

	// Of files alone, nothing to name it of.
	files := &thread{ID: 99, Title: "notes.md", Messages: []*message{{Role: "user", Files: []*attachment{{Name: "notes.md"}}}}}
	if a.canRetitle(files) {
		t.Error("regenerates a chat of files alone")
	}
}

// Escape in the field ends the renaming alone: the reply streaming in
// goes on.
func TestRenameEscapeKeepsReply(t *testing.T) {
	p := &held{gate: map[string]chan struct{}{}}
	a := newApp(p)
	posts := make(chan func(), 64)
	a.post = func(f func()) { posts <- f }
	tt := ui.NewTester(a.view, 1120, 760)
	a.send("Hello")
	th := a.current()
	tt.Frame()
	if th.busy == nil {
		t.Fatal("no reply streaming in")
	}
	a.startRename(th, false)
	tt.Frame()
	tt.Type("Greeting")
	tt.Key(0, ui.KeyEscape)
	if a.renaming != "" || th.Title != "Hello" {
		t.Errorf("renaming %q, title %q", a.renaming, th.Title)
	}
	if th.busy == nil {
		t.Error("Escape stopped the reply")
	}
	// A second Escape, the field gone, stops it.
	tt.Key(0, ui.KeyEscape)
	if th.busy != nil {
		t.Error("Escape did not stop the reply once the renaming ended")
	}
	close(p.release("Hello"))
	for len(posts) > 0 {
		(<-posts)()
	}
}

// A row of the sidebar shows, under the pointer, the button of its menu,
// the one its right click opens, which does not open its chat.
func TestRowMoreButton(t *testing.T) {
	a, tt := newTester()
	tt.Type("How do I cache?")
	tt.Key(0, ui.KeyEnter)
	first := a.current()
	a.newChat()
	tt.Frame()
	tt.Type("Plan the release")
	tt.Key(0, ui.KeyEnter)
	shown := a.selected
	r, ok := tt.Find("How do I cache?")
	if !ok {
		t.Fatalf("no row in %q", tt.Texts())
	}
	tt.Move(r.X+4, r.Y+r.H/2)
	tt.Advance(time.Second)
	// Over the end of the title's line, as far in as the row's text.
	tt.ClickAt(a.sideWidth-barClear-6-rowMoreSize/2, r.Y+r.H/2)
	want := []string{locales.RenameChatItem(), locales.RegenerateTitleItem(), "-", locales.DeleteChatItem()}
	if got := tt.Menu(); !slices.Equal(got, want) {
		t.Fatalf("menu %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem(locales.RenameChatItem()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.selected != shown || a.renaming != strconv.Itoa(first.ID) || !a.renameInSidebar {
		t.Errorf("shown %q (was %q), renaming %q in the sidebar %v", a.selected, shown, a.renaming, a.renameInSidebar)
	}
}
