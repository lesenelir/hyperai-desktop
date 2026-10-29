package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// newTester runs the view without a window, where the demo's replies come
// at once.
func newTester() (*app, *ui.Tester) {
	a := newApp(&demo{})
	return a, ui.NewTester(a.view, 1120, 760)
}

func TestSendWithEnter(t *testing.T) {
	a, tt := newTester()
	if !tt.HasText("What are you building with AI today?") {
		t.Fatalf("no welcome in %q", tt.Texts())
	}
	tt.Type("How do I cache?")
	tt.Key(0, ui.KeyEnter)
	if len(a.threads) != 1 || a.selected == "" {
		t.Fatalf("threads %d, selected %q after sending", len(a.threads), a.selected)
	}
	if a.draft != "" {
		t.Errorf("draft %q after sending", a.draft)
	}
	if !tt.HasText("How do I cache?") || !tt.HasText("A few things to note:") {
		t.Errorf("no message or reply in %q", tt.Texts())
	}
	if !tt.HasText("Today") {
		t.Errorf("chat not in the sidebar: %q", tt.Texts())
	}
}

func TestShiftEnterBreaksLine(t *testing.T) {
	a, tt := newTester()
	tt.Type("one")
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("two")
	if a.draft != "one\ntwo" || len(a.threads) != 0 {
		t.Errorf("draft %q, threads %d", a.draft, len(a.threads))
	}
}

func TestSuggestionAndNewChat(t *testing.T) {
	a, tt := newTester()
	// A prompt goes to the composer, to send as it is or changed.
	if err := tt.Click(suggestions[0].prompt); err != nil {
		t.Fatal(err)
	}
	if len(a.threads) != 0 || a.draft != suggestions[0].prompt {
		t.Fatalf("after a click on a prompt: %d chats, draft %q", len(a.threads), a.draft)
	}
	tt.Type(" With tests.")
	tt.Key(0, ui.KeyEnter)
	if len(a.threads) != 1 || !tt.HasText(suggestions[0].prompt+" With tests.") {
		t.Fatalf("prompt not sent: %q", tt.Texts())
	}
	if err := tt.Click("New chat"); err != nil {
		t.Fatal(err)
	}
	if a.selected != "" || !tt.HasText("What are you building with AI today?") {
		t.Errorf("selected %q after New chat, texts %q", a.selected, tt.Texts())
	}
	// The chat stays in the sidebar, and shows again when chosen.
	if err := tt.Click(a.threads[0].Title); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(suggestions[0].prompt) {
		t.Errorf("chat not shown again: %q", tt.Texts())
	}
}

func TestRemove(t *testing.T) {
	a, tt := newTester()
	tt.Type("Hello")
	tt.Key(0, ui.KeyEnter)
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Delete chat"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	if len(a.threads) != 0 || a.selected != "" || !tt.HasText("Your chats show here.") {
		t.Errorf("threads %d, selected %q, texts %q", len(a.threads), a.selected, tt.Texts())
	}
}

func TestGroupByDay(t *testing.T) {
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC)
	at := func(days int) *thread { return &thread{Updated: now.AddDate(0, 0, -days)} }
	groups := groupByDay([]*thread{at(40), at(0), at(1), at(3), at(0)}, now)
	var got []string
	for _, g := range groups {
		got = append(got, fmt.Sprintf("%s:%d", g.title, len(g.threads)))
	}
	if want := "Today:2 Yesterday:1 Previous 7 days:1 Older:1"; strings.Join(got, " ") != want {
		t.Errorf("groups %q, want %q", strings.Join(got, " "), want)
	}
}

func TestOpenRouterStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer key" || r.Header.Get("X-Title") != "HyperAI" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// OpenRouter sends comments while the model warms up.
		fmt.Fprint(w, ": OPENROUTER PROCESSING\n\n")
		for _, s := range []string{"Hel", "lo", "!"} {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", s)
		}
		if r.URL.Query().Has("fail") {
			fmt.Fprint(w, "data: {\"error\":{\"message\":\"Provider returned error\"}}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	p := &openRouter{baseURL: srv.URL, apiKey: "key", models: []string{"m"}, client: srv.Client()}
	var got strings.Builder
	if err := p.Stream(context.Background(), "m", []turn{{Role: "user", Content: "Hi"}}, func(s string) { got.WriteString(s) }); err != nil {
		t.Fatal(err)
	}
	if got.String() != "Hello!" {
		t.Errorf("reply %q", got.String())
	}

	// An error after the reply began ends it.
	failing := &openRouter{baseURL: srv.URL, apiKey: "key", client: &http.Client{Transport: failTransport{srv.Client().Transport}}}
	if err := failing.Stream(context.Background(), "m", nil, func(string) {}); err == nil || err.Error() != "Provider returned error" {
		t.Errorf("error %v for an error mid-stream", err)
	}

	p.apiKey = "wrong"
	if err := p.Stream(context.Background(), "m", nil, func(string) {}); err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("error %v for a bad key", err)
	}
}

func TestPalette(t *testing.T) {
	// hyperai-next's oklch(30% 0.01 20), oklch(100% 0.01 20) and, dark,
	// oklch(20% 0.01 4.45) in sRGB.
	for _, tc := range []struct {
		got  ui.Color
		want string
	}{
		{lightPalette.fg, "#332c2c"},
		{lightPalette.bg, "#fffdfc"},
		{darkPalette.bg, "#1a1415"},
	} {
		if got := fmt.Sprintf("#%02x%02x%02x", tc.got.R, tc.got.G, tc.got.B); got != tc.want {
			t.Errorf("color %s, want %s", got, tc.want)
		}
	}
}

func TestCSSOklch(t *testing.T) {
	// What the browser draws hyperai-next's accents as on a Mac, in
	// Display P3: each channel clipped, as Chrome does.
	enc := func(v float64) int {
		v = max(0, min(v, 1))
		if v > 0.0031308 {
			v = 1.055*math.Pow(v, 1/2.4) - 0.055
		} else {
			v *= 12.92
		}
		return int(math.Round(v * 255))
	}
	for _, tc := range []struct {
		l, c, h float64
		want    string
	}{
		{0.50, 0.28, 40, "#c10000"},
		{0.90, 0.212, 8, "#ffa4c4"},
	} {
		r, g, b := p3(tc.l, tc.c, tc.h)
		if got := fmt.Sprintf("#%02x%02x%02x", enc(r), enc(g), enc(b)); got != tc.want {
			t.Errorf("oklch(%v %v %v) is %s in P3, want %s", tc.l, tc.c, tc.h, got, tc.want)
		}
		// Back from P3, clipped, it is the same color.
		r, g, b = max(0, min(r, 1)), max(0, min(g, 1)), max(0, min(b, 1))
		r2, g2, b2 := p3(oklchOfP3(r, g, b))
		if math.Abs(r-r2)+math.Abs(g-g2)+math.Abs(b-b2) > 1e-6 {
			t.Errorf("round trip of %v %v %v is %v %v %v", r, g, b, r2, g2, b2)
		}
	}
}

func TestLoadEnv(t *testing.T) {
	got := parseEnv(`# a comment
export A=1
B = "two words"  
C='single # not a comment'
D=plain # a comment
E=
broken line
A=last
`)
	want := map[string]string{"A": "last", "B": "two words", "C": "single # not a comment", "D": "plain", "E": ""}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("parsed %v, want %v", got, want)
	}

	// The environment wins over the files, and the first file over the
	// later.
	dir := t.TempDir()
	first, second := dir+"/first.env", dir+"/second.env"
	os.WriteFile(first, []byte("HYPERAI_T1=file\nHYPERAI_T2=first\n"), 0o600)
	os.WriteFile(second, []byte("HYPERAI_T2=second\nHYPERAI_T3=second\n"), 0o600)
	t.Setenv("HYPERAI_T1", "shell")
	for _, k := range []string{"HYPERAI_T2", "HYPERAI_T3"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	loadEnv(first, dir+"/missing.env", second)
	for k, v := range map[string]string{"HYPERAI_T1": "shell", "HYPERAI_T2": "first", "HYPERAI_T3": "second"} {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

// failTransport asks the test server to fail mid-stream.
type failTransport struct{ http.RoundTripper }

func (f failTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.URL.RawQuery = "fail"
	return f.RoundTripper.RoundTrip(r)
}

func TestBatcher(t *testing.T) {
	var mu sync.Mutex
	var commits []string
	b := newBatcher(func(s string) {
		mu.Lock()
		commits = append(commits, s)
		mu.Unlock()
	})
	// The first chunk commits at once; those coming within commitInterval
	// of it wait and commit together.
	b.add("a")
	time.Sleep(commitInterval / 5)
	for _, s := range []string{"b", "c", "d"} {
		b.add(s)
		time.Sleep(time.Millisecond)
	}
	time.Sleep(2 * commitInterval)
	b.add("e")
	b.add("f")
	b.close() // commits what is left at once
	b.add("g")
	time.Sleep(2 * commitInterval)
	mu.Lock()
	defer mu.Unlock()
	if got := strings.Join(commits, "|"); got != "a|bcd|ef" {
		t.Errorf("commits %q, want a|bcd|ef", got)
	}
}

func TestScrollToEnd(t *testing.T) {
	a, tt := newTester()
	// Enough replies to scroll.
	for _, s := range suggestions {
		tt.Type(s.prompt)
		tt.Key(0, ui.KeyEnter)
	}
	tt.Advance(time.Second)
	th := a.current()
	if !th.list.AtEnd() || tt.HasText("Scroll to end") {
		t.Fatalf("at the end %v, with the button %v", th.list.AtEnd(), tt.HasText("Scroll to end"))
	}
	tt.Scroll(700, 300, 0, -2000)
	tt.Advance(time.Second)
	if th.list.AtEnd() || !tt.HasText("Scroll to end") {
		t.Fatalf("scrolled up: at the end %v, with the button %v", th.list.AtEnd(), tt.HasText("Scroll to end"))
	}
	from := th.scroll.Y
	if err := tt.Click("Scroll to end"); err != nil {
		t.Fatal(err)
	}
	// It glides: on its way after a moment, the button gone at once.
	tt.Advance(50 * time.Millisecond)
	if th.scroll.Y <= from || th.list.AtEnd() || tt.HasText("Scroll to end") {
		t.Errorf("50 ms after the click: from %v to %v of %v, at the end %v, with the button %v",
			from, th.scroll.Y, th.scroll.MaxY, th.list.AtEnd(), tt.HasText("Scroll to end"))
	}
	tt.Advance(time.Second)
	if !th.list.AtEnd() || tt.HasText("Scroll to end") {
		t.Errorf("after the glide: at the end %v, with the button %v", th.list.AtEnd(), tt.HasText("Scroll to end"))
	}
	// There, it follows the end again.
	tt.Type("One more")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	if !th.list.AtEnd() {
		t.Errorf("not following the end after a new reply")
	}
}

// atTop reports whether the question text shows at the top of the
// conversation, under the title bar, and where it shows.
func atTop(t *testing.T, tt *ui.Tester, text string) (bool, float32) {
	t.Helper()
	r, ok := tt.Find(text)
	if !ok {
		t.Fatalf("no %q in %q", text, tt.Texts())
	}
	y := r.Y - titleBarHeight - bubblePadY // its bubble's top
	return y > listPadTop-0.5 && y < listPadTop+0.5, y
}

func TestPinQuestionToTop(t *testing.T) {
	a, tt := newTester()
	tt.SetSize(1120, 1000) // tall enough for each reply under its question
	for _, s := range suggestions[:3] {
		tt.Type(s.prompt)
		tt.Key(0, ui.KeyEnter)
		tt.Advance(time.Second)
		// The question just sent shows at the top, its reply under it.
		if ok, y := atTop(t, tt, s.prompt); !ok {
			t.Errorf("%q %v below the top, want %v", s.prompt, y, listPadTop)
		}
	}
	// The last reply is short: room after it holds the question up, and the
	// list shows its end there.
	th := a.current()
	if th.pin.room <= 0 || !th.list.AtEnd() || tt.HasText("Scroll to end") {
		t.Errorf("room %v, at the end %v, with the button %v", th.pin.room, th.list.AtEnd(), tt.HasText("Scroll to end"))
	}
}

func TestPinOutgrown(t *testing.T) {
	// A window short enough that the first reply outgrows its room.
	a := newApp(&demo{})
	tt := ui.NewTester(a.view, 1120, 520)
	tt.Type("Hi")
	tt.Key(0, ui.KeyEnter)
	tt.Type(suggestions[0].prompt)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	// The question lets go, and the list follows the reply's end.
	th := a.current()
	if th.pin.id != 0 || !th.list.AtEnd() || !th.list.FollowEnd || tt.HasText("Scroll to end") {
		t.Errorf("pinned %v, at the end %v, following %v, with the button %v",
			th.pin.id, th.list.AtEnd(), th.list.FollowEnd, tt.HasText("Scroll to end"))
	}
}

func TestPinStreaming(t *testing.T) {
	// A first question from a new chat, whose reply streams in, and outgrows
	// a short window.
	a := newApp(&demo{})
	tt := ui.NewTester(a.view, 1120, 520)
	tt.Type("Hi")
	tt.Key(0, ui.KeyEnter)
	th := a.current()
	reply := th.Messages[1]
	full := reply.Text
	reply.Text, reply.streaming, reply.Elapsed = "", true, 0
	th.pinTop(0)
	followed := false
	for i := 0; i < len(full); i += 24 {
		// A commit, and the one frame it makes, as the window draws.
		reply.Text = full[:min(i+24, len(full))]
		tt.Frame()
		// The button never shows while the reply streams.
		if tt.HasText("Scroll to end") {
			t.Fatalf("the button shows with %d of %d bytes: pinned %v, room %v, at the end %v",
				len(reply.Text), len(full), th.pin.id, th.pin.room, th.list.AtEnd())
		}
		if th.pin.id == 0 {
			followed = true
			if !th.list.AtEnd() {
				t.Fatalf("not at the end with %d of %d bytes, once the reply outgrew its room", len(reply.Text), len(full))
			}
		}
	}
	if !followed {
		t.Error("the reply never outgrew its room")
	}
}

func TestOutline(t *testing.T) {
	a, tt := newTester()
	tt.Type(suggestions[0].prompt)
	tt.Key(0, ui.KeyEnter)
	if _, ok := tt.Find("Message 1: " + suggestions[0].prompt); ok {
		t.Error("an outline for one turn")
	}
	for _, s := range suggestions[1:3] {
		tt.Type(s.prompt)
		tt.Key(0, ui.KeyEnter)
	}
	tt.Advance(time.Second)
	th := a.current()
	turns := outlineTurns(th.Messages)
	if len(turns) != 3 {
		t.Fatalf("%d turns", len(turns))
	}
	tick, ok := tt.Find("Message 1: " + suggestions[0].prompt)
	if !ok {
		t.Fatalf("no tick for the first turn in %q", tt.Texts())
	}
	// On the left of the conversation, beside the sidebar.
	if tick.X > sidebarWidth+60 {
		t.Errorf("tick at %v, not on the left", tick.X)
	}
	// The pointer on a tick shows the card of its turn.
	tt.Move(tick.X+4, tick.Y+tick.H/2)
	tt.Advance(time.Second)
	if !tt.HasText(turns[0].reply) {
		t.Errorf("no card with %q", turns[0].reply)
	}
	// A click glides to its question: on its way after a moment.
	from := th.scroll.Y
	tt.ClickAt(tick.X+4, tick.Y+tick.H/2)
	tt.Advance(50 * time.Millisecond)
	if !th.scroll.Scrolling() || th.scroll.Y >= from {
		t.Errorf("50 ms after the click: scrolling %v, from %v to %v", th.scroll.Scrolling(), from, th.scroll.Y)
	}
	tt.Advance(time.Second)
	if ok, y := atTop(t, tt, suggestions[0].prompt); !ok {
		t.Errorf("first question %v below the top after the click, want %v", y, listPadTop)
	}
}

func TestOutlineFarTurn(t *testing.T) {
	// Many turns in a short window: the first question was never laid out,
	// so the glide goes by estimated heights, and the list puts it at the
	// top as it ends.
	a := newApp(&demo{})
	a.next++
	th := &thread{ID: a.next, Title: "Long"}
	for i := range 12 {
		a.next++
		th.Messages = append(th.Messages, &message{ID: a.next, Role: "user", Text: "Question " + strconv.Itoa(i+1)})
		a.next++
		th.Messages = append(th.Messages, &message{ID: a.next, Role: "assistant", Text: demoReplies[i%len(demoReplies)]})
	}
	a.threads, a.selected = []*thread{th}, strconv.Itoa(th.ID)
	tt := ui.NewTester(a.view, 1120, 520)
	tt.Advance(time.Second)
	tick, ok := tt.Find("Message 1: Question 1")
	if !ok {
		t.Fatalf("no tick in %q", tt.Texts())
	}
	tt.ClickAt(tick.X+4, tick.Y+tick.H/2)
	tt.Advance(2 * time.Second)
	if ok, y := atTop(t, tt, "Question 1"); !ok {
		t.Errorf("first question %v below the top, want %v", y, listPadTop)
	}
}

func TestFormatTime(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		t     time.Time
		short string
	}{
		{time.Date(2026, 10, 29, 2, 21, 0, 0, time.UTC), "2:21 AM"},
		{time.Date(2026, 10, 28, 23, 5, 0, 0, time.UTC), "Oct 28, 11:05 PM"},
		{time.Date(2025, 10, 29, 9, 0, 0, 0, time.UTC), "Oct 29, 2025, 9:00 AM"},
	} {
		short, full := formatTime(tc.t, now)
		if short != tc.short {
			t.Errorf("formatTime(%v) = %q, want %q", tc.t, short, tc.short)
		}
		if want := tc.t.Format("January 2, 2006 at 3:04 PM"); full != want {
			t.Errorf("formatTime(%v) full = %q, want %q", tc.t, full, want)
		}
	}
}

func TestCopyMessage(t *testing.T) {
	a, tt := newTester()
	tt.SetSize(1120, 1600) // the question and its whole reply show
	tt.Type("How do I cache?")
	tt.Key(0, ui.KeyEnter)
	th := a.threads[0]
	question, reply := th.Messages[0], th.Messages[1]
	if question.Time.IsZero() || reply.Time.IsZero() {
		t.Fatalf("times %v, %v", question.Time, reply.Time)
	}
	short, _ := formatTime(reply.Time, tt.Now())
	if !tt.HasText(short) {
		t.Fatalf("no time %q in %q", short, tt.Texts())
	}
	// The question's button is first, beside its bubble.
	if err := tt.Click("Copy"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != question.Text {
		t.Errorf("clipboard %q, want the question", tt.Clipboard())
	}
	if _, ok := tt.Find("Copied"); !ok {
		t.Errorf("no check after copying")
	}
	tt.Advance(copiedDuration + 100*time.Millisecond)
	if _, ok := tt.Find("Copied"); ok {
		t.Errorf("check still shows after %v", copiedDuration)
	}
}

// A reply's button lies under the outline's left edge where the window
// leaves the conversation little room beside it: the outline takes the
// pointer only over its ticks.
func TestCopyReplyBesideOutline(t *testing.T) {
	a, tt := newTester()
	for _, q := range []string{"How do I cache?", "And evict?"} {
		tt.Type(q)
		tt.Key(0, ui.KeyEnter)
		tt.Advance(2 * time.Second)
	}
	if !tt.HasText("Conversation outline") {
		t.Fatalf("no outline in %q", tt.Texts())
	}
	th := a.threads[0]
	i := len(th.Messages) - 1
	// The button is the first of the reply's actions, at its foot, 6 DIPs
	// out of the column's left.
	b := th.boxes[i]
	x, y := b.X+max(0, (b.W-maxWidth)/2)-6+15, b.Y+b.H-14
	if in := x - th.listBox.X; in > 12+40 {
		t.Fatalf("the button %v in from the pane is clear of the outline", in)
	}
	tt.ClickAt(x, y)
	if tt.Clipboard() != th.Messages[i].Text {
		t.Errorf("clipboard %.40q, want the reply", tt.Clipboard())
	}
}

func TestComposerFloats(t *testing.T) {
	a, tt := newTester()
	for _, s := range suggestions {
		tt.Type(s.prompt)
		tt.Key(0, ui.KeyEnter)
	}
	tt.Advance(time.Second)
	th := a.current()
	const h = 760 // newTester's window
	// The conversation runs down to the window's bottom, under the
	// composer, and its end shows above the composer.
	if bottom := th.listBox.Y + th.listBox.H; bottom < h-0.5 {
		t.Errorf("list ends at %v, want the window's bottom %v", bottom, h)
	}
	last := th.box(len(th.Messages) - 1)
	if top := h - a.footer; a.footer < 60 || last.Y+last.H > top-listPadBottom+0.5 {
		t.Errorf("last row ends at %v, the composer (%v tall) starts at %v", last.Y+last.H, a.footer, top)
	}
	// Scrolling over the composer scrolls the conversation.
	field, ok := tt.Find("Message")
	if !ok {
		t.Fatalf("no composer in %q", tt.Texts())
	}
	from := th.scroll.Y
	tt.Scroll(field.X+40, field.Y+10, 0, -300)
	tt.Advance(time.Second)
	if th.scroll.Y >= from || th.list.AtEnd() {
		t.Errorf("scrolling over the composer: from %v to %v, at the end %v", from, th.scroll.Y, th.list.AtEnd())
	}
	// It still sends.
	n := len(th.Messages)
	tt.Type("One more")
	if err := tt.Click("Send"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(th.Messages) != n+2 {
		t.Errorf("%d messages after Send, want %d", len(th.Messages), n+2)
	}
}

func TestComposerKeepsFocusAsChatStarts(t *testing.T) {
	a, tt := newTester()
	tt.Type("Hi")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	tt.Type("again")
	if a.draft != "again" {
		t.Errorf("draft %q after the first message, want %q", a.draft, "again")
	}
}

// TestComposerResizes checks every frame as the composer grows and shrinks:
// the conversation's end moves with it in the frame it changes, rather than
// a frame after, which left an empty band (or hid the last lines) for a
// frame; and a question pinned to the top stays there.
func TestComposerResizes(t *testing.T) {
	edit := func(tt *ui.Tester, check func(string)) {
		for i := 0; i < 15; i++ {
			tt.Type("line")
			tt.Key(ui.Shift, ui.KeyEnter)
			check("growing")
		}
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Key(0, ui.KeyBackspace)
		check("all deleted")
		tt.Type(strings.Repeat("wrap me ", 60))
		check("a long line")
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type(strings.Repeat("中文换行测试，", 40))
		check("a long line of Chinese")
		tt.Key(ui.Shift, ui.KeyEnter)
		check("a new line")
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Key(0, ui.KeyBackspace)
		check("all deleted again")
	}

	a, tt := newTester()
	for _, s := range suggestions {
		tt.Type(s.prompt)
		tt.Key(0, ui.KeyEnter)
	}
	tt.Advance(time.Second)
	th := a.current()
	th.pin = pin{}
	th.list.ScrollToEnd()
	tt.Advance(time.Second)
	edit(tt, func(when string) {
		// The layout of the frame the edit drew, as the next one reads it.
		tt.Frame()
		last := th.box(len(th.Messages) - 1)
		if gap := 760 - a.footer - (last.Y + last.H); gap < listPadBottom-0.5 || gap > listPadBottom+0.5 {
			t.Errorf("%s: %v between the last message and the composer, want %v", when, gap, listPadBottom)
		}
	})

	a, tt = newTester()
	tt.SetSize(1120, 1100) // room for the reply and the composer at its tallest
	tt.Type("Hi")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	th = a.current()
	if th.pin.id == 0 {
		t.Fatal("the question is not pinned")
	}
	edit(tt, func(when string) {
		tt.Frame()
		if y := th.box(0).Y - th.listBox.Y; y < listPadTop-0.5 || y > listPadTop+0.5 {
			t.Errorf("%s: the question %v below the top, want %v", when, y, listPadTop)
		}
	})
}

// TestScrollOverComposer checks what the wheel scrolls over the composer:
// the field while it has more lines than it shows, to its ends and no
// further; the conversation over the field when it shows all its text, and
// beside the field.
func TestScrollOverComposer(t *testing.T) {
	a, tt := newTester()
	for _, s := range suggestions {
		tt.Type(s.prompt)
		tt.Key(0, ui.KeyEnter)
	}
	tt.Advance(time.Second)
	th := a.current()
	f := a.field
	scroll := func(x, y, dy float32) {
		tt.Scroll(x, y, 0, dy)
		tt.Advance(time.Second)
	}

	// A field that shows all its text: the conversation.
	from := th.scroll.Y
	scroll(f.X+40, f.Y+f.H/2, -200)
	if th.scroll.Y >= from {
		t.Errorf("over a short field: the conversation from %v to %v, want up", from, th.scroll.Y)
	}
	th.list.ScrollToEnd()
	tt.Advance(time.Second)

	// More lines than the field shows: the field, up and down.
	for i := 0; i < 20; i++ {
		tt.Type("line")
		tt.Key(ui.Shift, ui.KeyEnter)
	}
	tt.Advance(time.Second)
	f = a.field
	if a.fieldScroll.MaxY <= 0 {
		t.Fatalf("the field does not scroll: %+v", a.fieldScroll)
	}
	from, fieldFrom := th.scroll.Y, a.fieldScroll.Y
	scroll(f.X+40, f.Y+f.H/2, -100)
	if a.fieldScroll.Y >= fieldFrom || th.scroll.Y != from {
		t.Errorf("over a long field, up: the field from %v to %v, the conversation from %v to %v",
			fieldFrom, a.fieldScroll.Y, from, th.scroll.Y)
	}
	// At its top, no further: the conversation stays.
	scroll(f.X+40, f.Y+f.H/2, -5000)
	if a.fieldScroll.Y != 0 || th.scroll.Y != from {
		t.Errorf("over a long field, past its top: the field at %v, the conversation from %v to %v",
			a.fieldScroll.Y, from, th.scroll.Y)
	}
	scroll(f.X+40, f.Y+f.H/2, 100)
	if a.fieldScroll.Y <= 0 || th.scroll.Y != from {
		t.Errorf("over a long field, down: the field at %v, the conversation from %v to %v",
			a.fieldScroll.Y, from, th.scroll.Y)
	}

	// Beside the field, by the model: the conversation.
	status, ok := tt.Find("Model")
	if !ok {
		t.Fatalf("no model picker in %q", tt.Texts())
	}
	scroll(status.X+10, status.Y+status.H/2, -200)
	if th.scroll.Y >= from {
		t.Errorf("over the status: the conversation from %v to %v, want up", from, th.scroll.Y)
	}
}

// TestHistory is Waku's session_navigation tests, on chat IDs.
func TestHistory(t *testing.T) {
	var h history
	h.visit("1", "2")
	h.visit("2", "3")
	step := func(got string, ok bool, want string) {
		t.Helper()
		if !ok && want != "" || ok && got != want {
			t.Errorf("went to %q (%v), want %q", got, ok, want)
		}
	}
	got, ok := h.goBack("3")
	step(got, ok, "2")
	got, ok = h.goBack("2")
	step(got, ok, "1")
	got, ok = h.goForward("1")
	step(got, ok, "2")
	// A new branch drops what was forward.
	h.visit("2", "4")
	got, ok = h.goForward("4")
	step(got, ok, "")
	got, ok = h.goBack("4")
	step(got, ok, "2")

	// Deleted chats are gone from it, and the steps they leave to nowhere.
	h = history{}
	h.visit("1", "2")
	h.visit("2", "3")
	got, ok = h.goBack("3")
	step(got, ok, "2")
	h.remove("1", "2")
	h.remove("3", "2")
	if len(h.back) != 0 || len(h.forward) != 0 {
		t.Errorf("back %q, forward %q after removing both sides", h.back, h.forward)
	}
	h = history{}
	h.visit("1", "2")
	h.visit("2", "1")
	h.visit("1", "3")
	h.remove("2", "3") // back was 1, 2, 1: one step to 1
	if !slices.Equal(h.back, []string{"1"}) {
		t.Errorf("back %q, want [1]", h.back)
	}
}

func TestBackForward(t *testing.T) {
	a, tt := newTester()
	back := "Back (" + backKeys.String() + ")"
	forward := "Forward (" + forwardKeys.String() + ")"
	backKey, forwardKey := func() { tt.Key(ui.Super, ui.KeyBracketLeft) }, func() { tt.Key(ui.Super, ui.KeyBracketRight) }
	if runtime.GOOS != "darwin" {
		backKey, forwardKey = func() { tt.Key(ui.Alt, ui.KeyLeft) }, func() { tt.Key(ui.Alt, ui.KeyRight) }
	}
	send := func(s string) string {
		tt.Type(s)
		tt.Key(0, ui.KeyEnter)
		tt.Advance(time.Second)
		return a.selected
	}
	at := func(want string) {
		t.Helper()
		if a.selected != want {
			t.Errorf("showing %q, want %q", a.selected, want)
		}
	}

	// A chat started from a new one takes its place: nothing to go back to.
	first := send("first")
	if err := tt.Click(back); err != nil {
		t.Fatal(err)
	}
	at(first)
	tt.Key(ui.Cmd, ui.KeyN)
	second := send("second")

	// The buttons, the keys (with the composer focused), and the mouse's.
	if err := tt.Click(back); err != nil {
		t.Fatal(err)
	}
	at(first)
	forwardKey()
	at(second)
	backKey()
	at(first)
	tt.Key(0, ui.KeyForward)
	at(second)
	tt.Key(0, ui.KeyBack)
	at(first)
	// Choosing a chat drops what was forward.
	tt.Key(ui.Cmd, ui.KeyN)
	at("")
	forwardKey()
	at("")
	backKey()
	at(first)

	// A chat deleted is gone from the history.
	if err := tt.Click(forward); err != nil {
		t.Fatal(err)
	}
	at("")
	if err := tt.Click(a.threads[1].Title); err != nil {
		t.Fatal(err)
	}
	at(second)
	a.remove(a.threads[0]) // first
	backKey()
	at("")
	backKey()
	at("")
	if len(a.nav.back) != 0 {
		t.Errorf("back %q after deleting the chat before", a.nav.back)
	}
}

func TestSearch(t *testing.T) {
	a, tt := newTester()
	for _, s := range suggestions[:3] {
		if err := tt.Click(s.prompt); err != nil {
			t.Fatal(err)
		}
		tt.Key(0, ui.KeyEnter)
		if err := tt.Click("New chat"); err != nil {
			t.Fatal(err)
		}
	}
	lru, stream, sched := a.threads[0], a.threads[1], a.threads[2]
	// The rows the search lists, by title.
	lists := func(want ...string) {
		t.Helper()
		var got []string
		for _, s := range a.searchSections() {
			for _, it := range s.items {
				got = append(got, it.title)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("lists %q, want %q", got, want)
		}
	}

	// ⌘F opens it over the window, the field focused, on All: the chats
	// used last, and what to do.
	tt.Key(ui.Cmd, ui.KeyF)
	if !a.searching || !tt.HasText("Recents") || !tt.HasText("Actions") || !tt.HasText("Artifacts") {
		t.Fatalf("searching %v, texts %q", a.searching, tt.Texts())
	}
	lists(sched.Title, stream.Title, lru.Title, "New chat")

	// By the title, in any case, and by what the messages say, with the
	// words around it.
	tt.Type("STREAMING")
	lists(stream.Title, "New chat")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("container/list")
	lists(lru.Title, "New chat")
	if h := a.searchSections()[0].items[0].hit; h.who != "HyperAI" || h.match != "container/list" ||
		!strings.HasPrefix(h.after, " keeps the order of use") || !strings.HasPrefix(h.before, "…") {
		t.Errorf("hit %q said %q … %q … %q", h.who, h.before, h.match, h.after)
	}
	// Settings match too.
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("sidebar")
	lists("Hide sidebar", "New chat")

	// Up and Down choose, around the ends, and Enter does it.
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("go")
	n := 0
	for _, s := range a.searchSections() {
		n += len(s.items)
	}
	if n < 3 || a.searchSel != 0 {
		t.Fatalf("%d rows, chosen %d", n, a.searchSel)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyUp)
	if a.searchSel != 1 {
		t.Errorf("chosen %d after Down, Down, Up, want 1", a.searchSel)
	}
	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyUp)
	if a.searchSel != n-1 {
		t.Errorf("chosen %d after going up past the top, want the last, %d", a.searchSel, n-1)
	}
	tt.Key(0, ui.KeyUp)
	chosen := a.searchSections()[0].items[a.searchSel].hit.th
	tt.Key(0, ui.KeyEnter)
	if a.searching || a.current() != chosen {
		t.Errorf("after Enter: searching %v, showing %v, want %q", a.searching, a.current(), chosen.Title)
	}

	// Left and Right change the tab while there is no query, around the
	// ends; with one, they move in it.
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Key(0, ui.KeyRight)
	if a.searchTab != tabChats {
		t.Errorf("tab %v after Right, want Chats", a.searchTab)
	}
	lists(sched.Title, stream.Title, lru.Title)
	tt.Key(0, ui.KeyLeft)
	tt.Key(0, ui.KeyLeft)
	if a.searchTab != tabSettings {
		t.Errorf("tab %v after Left, Left, want Settings", a.searchTab)
	}
	tt.Type("x")
	tt.Key(0, ui.KeyLeft)
	if a.searchTab != tabSettings || a.query != "x" {
		t.Errorf("Left in a query: tab %v, query %q", a.searchTab, a.query)
	}
	// Settings do what they say: here, choose the model.
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	models := a.prov.Models()
	if len(models) > 1 {
		tt.Key(0, ui.KeyDown)
		tt.Key(0, ui.KeyEnter)
		if a.model != models[1] || a.searching {
			t.Errorf("model %q, searching %v after choosing %q", a.model, a.searching, models[1])
		}
		tt.Key(ui.Cmd, ui.KeyF)
		tt.Key(0, ui.KeyLeft)
	} else {
		tt.Key(0, ui.KeyRight)
		tt.Key(0, ui.KeyRight)
	}

	// Projects and artifacts are not yet; a query of nothing finds nothing.
	for _, tab := range []struct {
		tab  searchTab
		text string
	}{{tabProjects, "No projects yet"}, {tabArtifacts, "No artifacts yet"}} {
		a.setSearchTab(tab.tab)
		tt.Frame()
		if !tt.HasText(tab.text) {
			t.Errorf("no %q in %q", tab.text, tt.Texts())
		}
	}
	a.setSearchTab(tabChats)
	tt.Type("nothing says this")
	if !tt.HasText("No results for “nothing says this”") {
		t.Errorf("no word of no match in %q", tt.Texts())
	}

	// Tab writes the query in a new chat.
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Tell me more")
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if a.searching || a.selected != "" || a.draft != "Tell me more" {
		t.Errorf("after Tab: searching %v, showing %q, draft %q", a.searching, a.selected, a.draft)
	}

	// Escape closes it, as do its button and a click beside it.
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Type("streaming")
	tt.Key(0, ui.KeyEscape)
	if a.searching {
		t.Errorf("still searching after Escape")
	}
	tt.Key(ui.Cmd, ui.KeyF)
	if err := tt.Click("Close"); err != nil {
		t.Fatal(err)
	}
	if a.searching {
		t.Errorf("still searching after Close")
	}
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.ClickAt(5, 740)
	if a.searching {
		t.Errorf("still searching after a click beside it")
	}

	// A click on a row does it: the first under Recents, the chat used
	// last.
	tt.Key(ui.Cmd, ui.KeyF)
	r, ok := tt.Find("Recents")
	if !ok {
		t.Fatalf("no Recents in %q", tt.Texts())
	}
	tt.ClickAt(r.X+40, r.Y+r.H+6+19)
	if a.searching || a.current() != sched {
		t.Errorf("after a click on the first row: searching %v, showing %v, want %q", a.searching, a.current(), sched.Title)
	}

	// ⌘F shows the sidebar too, for what it chooses.
	tt.Key(ui.Cmd|ui.Shift, ui.KeyS)
	tt.Key(ui.Cmd, ui.KeyF)
	if a.collapsed || !a.searching {
		t.Errorf("⌘F with the sidebar hidden: collapsed %v, searching %v", a.collapsed, a.searching)
	}
}

// TestPlaceholder checks the composer's hint: shown while it is empty, and
// gone as soon as anything is in it, Pinyin composing included.
func TestPlaceholder(t *testing.T) {
	a, tt := newTester()
	const hint = "Ask anything…"
	if !tt.HasText(hint) {
		t.Fatalf("no hint in an empty composer: %q", tt.Texts())
	}
	tt.Type("hi")
	if tt.HasText(hint) {
		t.Errorf("hint over %q", a.draft)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	if !tt.HasText(hint) {
		t.Errorf("no hint once emptied")
	}
	tt.Compose("ni", 2)
	if tt.HasText(hint) || a.draft != "" {
		t.Errorf("hint over Pinyin composing, draft %q", a.draft)
	}
	tt.Compose("", 0)
	if !tt.HasText(hint) {
		t.Errorf("no hint once the composing is gone")
	}
}

// TestSidebarResize drags the sidebar's edge, as the pointer does.
func TestSidebarResize(t *testing.T) {
	a, tt := newTester()
	a.send("Hi")
	tt.Frame()
	th := a.current()
	const y = 400
	edge := func() float32 { return th.listBox.X - 1 } // the line between the two
	drag := func(xs ...float32) {
		x := edge()
		tt.Press(x, y)
		for _, to := range xs {
			tt.Move(to, y)
			tt.Frame()
		}
		tt.Release(xs[len(xs)-1], y)
		tt.Frame()
		tt.Frame()
	}
	if x := edge(); x != sidebarWidth {
		t.Fatalf("the sidebar is %v wide at first, want %v", x, sidebarWidth)
	}
	drag(320, 352)
	if a.sideWidth != 352 || edge() != 352 {
		t.Errorf("dragged to 352: the sidebar %v, its edge at %v", a.sideWidth, edge())
	}
	// Past its bounds, it stops; coming back, the edge comes with the
	// pointer at once.
	drag(1000)
	if a.sideWidth != sidebarMax {
		t.Errorf("dragged past the most: %v, want %v", a.sideWidth, sidebarMax)
	}
	drag(10, 300)
	if a.sideWidth != 300 {
		t.Errorf("dragged past the least and back to 300: %v", a.sideWidth)
	}
	drag(10)
	if a.sideWidth != sidebarMin {
		t.Errorf("dragged past the least: %v, want %v", a.sideWidth, sidebarMin)
	}

	// A narrow window keeps the conversation's room, and gives the
	// sidebar its width back as it widens.
	drag(400)
	tt.SetSize(700, 760)
	tt.Frame()
	tt.Frame()
	if want := float32(700 - paneMin); edge() != want {
		t.Errorf("in a window 700 wide, the edge at %v, want %v", edge(), want)
	}
	tt.SetSize(1120, 760)
	tt.Frame()
	tt.Frame()
	if edge() != 400 {
		t.Errorf("widened again, the edge at %v, want 400", edge())
	}

	// What a double click on the edge does (a Tester's clicks are single):
	// as wide as at first.
	a.resetSidebar()
	tt.Frame()
	tt.Frame()
	if edge() != sidebarWidth {
		t.Errorf("reset: the edge at %v, want %v", edge(), sidebarWidth)
	}
}

func TestLayoutKept(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&demo{})
	a.loadLayout(dir)
	a.sideWidth = 333
	a.saveLayout()
	a.layoutSaver.wait()
	b := newApp(&demo{})
	b.loadLayout(dir)
	if b.sideWidth != 333 {
		t.Errorf("the sidebar is %v wide after a restart, want 333", b.sideWidth)
	}
	c := newApp(&demo{})
	c.loadLayout(t.TempDir()) // nothing kept
	if c.sideWidth != sidebarWidth {
		t.Errorf("with nothing kept, %v wide, want %v", c.sideWidth, sidebarWidth)
	}
}

// TestMark checks that the mark takes the text color: resources/logo.svg
// fills it in a pink of its own, which icons.go replaces.
func TestMark(t *testing.T) {
	if strings.Count(logoSVG, `fill="#FF6096"`) != 1 {
		t.Errorf("resources/logo.svg no longer fills its mark with #FF6096 once: update markIcon")
	}
}

// TestComposerPill checks when the composer is a pill and when a box: a
// line that fits beside the buttons keeps it a pill; a line break, or a
// line that outgrows it, opens it; and the field keeps the focus through.
func TestComposerPill(t *testing.T) {
	a, tt := newTester()
	is := func(when string, pill bool) {
		t.Helper()
		tt.Frame()
		if a.pill != pill {
			t.Errorf("%s: pill %v, want %v", when, a.pill, pill)
		}
	}
	is("empty", true)
	tt.Type("A short question")
	is("a short line", true)
	tt.Key(ui.Shift, ui.KeyEnter)
	is("a line broken", false)
	tt.Key(0, ui.KeyBackspace)
	is("the break deleted", true)
	tt.Type(strings.Repeat(" and more", 30))
	is("a line past the buttons", false)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	is("all deleted", true)
	// Typing still goes to the field, after it opened and closed.
	tt.Type("still here")
	if a.draft != "still here" {
		t.Errorf("draft %q after the composer opened and closed", a.draft)
	}

	// Over a conversation, it is a box, a short line or none.
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	is("over a conversation, empty", false)
	tt.Type("A short question")
	is("over a conversation, a short line", false)
}

// TestFieldScrollsUnderItsPadding checks that a draft longer than the field
// fades out as it scrolls up into the room over its text, rather than
// running up to the box's edge: the fade comes in with the scrolling, and
// is gone at the field's top.
func TestFieldScrollsUnderItsPadding(t *testing.T) {
	a, tt := newTester()
	a.send("Hi")
	tt.Frame()
	for i := 0; i < 20; i++ {
		tt.Type("line")
		tt.Key(ui.Shift, ui.KeyEnter)
	}
	tt.Advance(time.Second)
	if a.fieldScroll.MaxY <= 0 || a.fieldScroll.Y <= 0 {
		t.Fatalf("the field does not scroll: %+v", a.fieldScroll)
	}
	// Scrolled, its text fades out at its top; at its top, it does not.
	if a.fieldFade() != 1 {
		t.Errorf("fade %v scrolled %v, want 1", a.fieldFade(), a.fieldScroll.Y)
	}
	tt.Scroll(a.field.X+40, a.field.Y+20, 0, -5000)
	tt.Advance(time.Second)
	if a.fieldScroll.Y != 0 || a.fieldFade() != 0 {
		t.Errorf("at the field's top: scrolled %v, fade %v, want none", a.fieldScroll.Y, a.fieldFade())
	}
}

func TestLongQuestionCollapses(t *testing.T) {
	a, tt := newTester()
	tt.SetSize(1120, 1600)
	ask := func(text string) *message {
		t.Helper()
		a.newChat() // a chat each, so that one button shows
		a.send(text)
		tt.Frame()
		tt.Advance(time.Second)
		th := a.current()
		return th.Messages[len(th.Messages)-2]
	}
	line := float32(bodySize * bodyLineHeight)

	// As many lines as collapsing would leave and the slack past them: whole.
	short := ask(strings.TrimSpace(strings.Repeat("A line\n", clampLines+slackLines)))
	if tt.HasText("Show more") || short.height < (clampLines+slackLines)*line {
		t.Fatalf("%d lines: with the button %v, %v tall", clampLines+slackLines, tt.HasText("Show more"), short.height)
	}

	// Past them, by lines or by wrapping: clampLines of them, over the button.
	for _, text := range []string{
		strings.TrimSpace(strings.Repeat("A line\n", 40)),
		strings.Repeat("A long question that wraps across the bubble. ", 60),
	} {
		m := ask(text)
		collapsed := m.height
		if !tt.HasText("Show more") || collapsed > clampLines*line+2*bubblePadY+40 {
			t.Fatalf("long: with the button %v, %v tall", tt.HasText("Show more"), collapsed)
		}
		if err := tt.Click("Show more"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if !tt.HasText("Show less") || m.height < 20*line {
			t.Fatalf("expanded: with the button %v, %v tall", tt.HasText("Show less"), m.height)
		}
		if err := tt.Click("Show less"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if !tt.HasText("Show more") || m.height != collapsed {
			t.Fatalf("collapsed again: with the button %v, %v tall, want %v", tt.HasText("Show more"), m.height, collapsed)
		}
	}

	// Narrow, the bubble is too: a question that fits wide wraps past them.
	tt.SetSize(400, 1600)
	a.collapsed = true
	text := strings.Repeat("A question that wraps. ", 20)
	m := ask(text)
	if m.wrapWidth >= bubbleMaxWidth-2*bubblePadX || !tt.HasText("Show more") {
		t.Errorf("narrow: wrapped at %v, %d lines, with the button %v", m.wrapWidth, m.lines, tt.HasText("Show more"))
	}
	tt.SetSize(1120, 1600)
	tt.Advance(time.Second)
	if m.wrapWidth != bubbleMaxWidth-2*bubblePadX || tt.HasText("Show more") {
		t.Errorf("wide: wrapped at %v, %d lines, with the button %v", m.wrapWidth, m.lines, tt.HasText("Show more"))
	}
}

func TestSettings(t *testing.T) {
	a, tt := newTester()
	tt.Type("How do I cache?")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	chat := a.selected

	// Settings, at the sidebar's foot, turn it into theirs: back to the
	// app, and their pages, the first shown.
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("Back to app") || tt.HasText("How do I cache?") || !tt.HasText("How HyperAI starts") {
		t.Fatalf("settings show %q", tt.Texts())
	}
	if err := tt.Click("Appearance"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("How HyperAI looks") {
		t.Fatalf("no Appearance page in %q", tt.Texts())
	}

	// The field narrows the pages by title or group.
	if err := tt.Click("Search settings"); err != nil {
		t.Fatal(err)
	}
	tt.Type("model")
	tt.Advance(time.Second)
	if !tt.HasText("Default model") || tt.HasText("Keyboard shortcuts") || !tt.HasText("Providers") {
		t.Errorf("for %q, the pages %q", a.settingsQuery, tt.Texts())
	}
	// And the page follows, Appearance found no more.
	if a.settingsPage != "providers" {
		t.Errorf("for %q, the page %q, want the first found", a.settingsQuery, a.settingsPage)
	}
	tt.Type("zzz")
	tt.Advance(time.Second)
	if !tt.HasText("No settings match.") {
		t.Errorf("no word that none match in %q", tt.Texts())
	}

	// Back to the chat shown before, and again on the page shown last.
	if err := tt.Click("Back to app"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.settings || a.selected != chat || !tt.HasText("How do I cache?") {
		t.Fatalf("back: settings %v, chat %q, want %q", a.settings, a.selected, chat)
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Advance(time.Second)
	if !a.settings || a.settingsQuery != "" || !tt.HasText("Where replies come from") {
		t.Fatalf("⌘,: settings %v, query %q, texts %q", a.settings, a.settingsQuery, tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if a.settings {
		t.Fatalf("Escape left the settings showing")
	}

	// Showing a chat, however, leaves them.
	a.openSettings()
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyN)
	tt.Advance(time.Second)
	if a.settings || a.selected != "" {
		t.Errorf("new chat: settings %v, chat %q", a.settings, a.selected)
	}
}

func TestAppearance(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&demo{})
	a.loadLayout(dir)
	var applied []string
	a.applyTheme = func(theme string) { applied = append(applied, theme) }
	// Tall enough for the whole page, which the switches are at the foot of.
	tt := ui.NewTester(a.view, 1120, 1200)
	tt.Type("How do I cache?")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	question := a.threads[0].Messages[0]
	before := question.height

	a.openSettings()
	a.settingsPage = "appearance"
	tt.Frame()

	// A card chooses the theme, which the app takes.
	if err := tt.Click("Dark"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.theme != "dark" || !slices.Equal(applied, []string{"dark"}) {
		t.Errorf("theme %q, applied %q", a.theme, applied)
	}

	// The text size, which the conversation takes too.
	if err := tt.Click("Large"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.body != 17 || a.md.Theme.FontSize != 17 {
		t.Errorf("text %v, Markdown %v, want 17", a.body, a.md.Theme.FontSize)
	}

	// The sidebar's material, only where the desktop gives one.
	toggle(t, tt, "Translucent sidebar", "This system draws no material behind windows.")
	tt.Advance(time.Second)
	if !a.translucent {
		t.Errorf("the switch turned with no material to show")
	}
	tt.SetVibrancy(true)
	tt.Frame()
	toggle(t, tt, "Translucent sidebar", "Let the desktop show through the sidebar.")
	tt.Advance(time.Second)
	if a.translucent {
		t.Errorf("the sidebar stayed translucent")
	}
	toggle(t, tt, "Reduce motion", "Show changes at once rather than fading or gliding. On whenever the system asks for less motion.")
	tt.Advance(time.Second)
	if !a.reduceMotion {
		t.Errorf("motion not reduced")
	}

	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if question.height <= before {
		t.Errorf("the question %v tall at 17, %v at 15", question.height, before)
	}

	// All of it kept for the next start.
	a.layoutSaver.wait()
	b := newApp(&demo{})
	b.loadLayout(dir)
	if b.theme != "dark" || textSizes[b.textSize] != 17 || b.translucent || !b.reduceMotion {
		t.Errorf("after a restart: theme %q, text %v, translucent %v, still %v", b.theme, textSizes[b.textSize], b.translucent, b.reduceMotion)
	}
}

func TestAppFont(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&demo{})
	a.loadLayout(dir)
	a.fonts = []ui.FontFamily{{Name: "Avenir Next"}, {Name: "Test Mono", Monospaced: true}, {Name: "Test Sans"}}
	tt := ui.NewTester(a.view, 1120, 1300)
	a.openSettings()
	a.settingsPage = "appearance"
	tt.Frame()

	// The button opens the menu of fonts: the system's, chosen, then those
	// installed.
	about := "All of the window's text, but code."
	toggle(t, tt, "Font", about)
	if got, want := tt.Menu(), []string{"System", "-", "Avenir Next", "Test Mono", "Test Sans"}; !slices.Equal(got, want) {
		t.Fatalf("menu %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem("Test Sans"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	// Which the window, the conversation and its Markdown take, and the
	// button shows.
	if a.font != "Test Sans" || a.winTheme.Font != "Test Sans" || a.md.Theme.Font != "Test Sans" || a.textFont != "Test Sans" {
		t.Errorf("font %q, window %q, Markdown %q, measured in %q", a.font, a.winTheme.Font, a.md.Theme.Font, a.textFont)
	}
	if !tt.HasText("Test Sans") {
		t.Errorf("the button does not show the font: %q", tt.Texts())
	}

	// Kept for the next start.
	a.layoutSaver.wait()
	b := newApp(&demo{})
	b.loadLayout(dir)
	if b.font != "Test Sans" {
		t.Errorf("after a restart: font %q", b.font)
	}

	// The system's again.
	toggle(t, tt, "Font", about)
	if err := tt.ChooseMenuItem("System"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.font != "" || a.winTheme.Font != "" {
		t.Errorf("font %q, window %q, want the system's", a.font, a.winTheme.Font)
	}

	// Code's: the system's monospaced font at first, which Markdown names.
	if a.md.Theme.CodeFont != "monospace" {
		t.Errorf("code in %q, want monospace", a.md.Theme.CodeFont)
	}
	codeAbout := "Code blocks and code in a line, in replies."
	toggle(t, tt, "Code font", codeAbout)
	if got, want := tt.Menu(), []string{"System", "-", "Test Mono", "Other Fonts"}; !slices.Equal(got, want) {
		t.Fatalf("code menu %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem("Test Mono"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.codeFont != "Test Mono" || a.md.Theme.CodeFont != "Test Mono" || a.winTheme.Font != "" {
		t.Errorf("code %q, Markdown's %q, window %q", a.codeFont, a.md.Theme.CodeFont, a.winTheme.Font)
	}
	// Any other, of the submenu.
	toggle(t, tt, "Code font", codeAbout)
	if err := tt.ChooseMenuItem("Other Fonts", "Avenir Next"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.codeFont != "Avenir Next" || a.md.Theme.CodeFont != "Avenir Next" {
		t.Errorf("code %q, Markdown's %q, want Avenir Next", a.codeFont, a.md.Theme.CodeFont)
	}
	a.layoutSaver.wait()
	b = newApp(&demo{})
	b.loadLayout(dir)
	if b.codeFont != "Avenir Next" || b.font != "" {
		t.Errorf("after a restart: code %q, font %q", b.codeFont, b.font)
	}
}

func TestCodeTheme(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&demo{})
	a.loadLayout(dir)
	tt := ui.NewTester(a.view, 1120, 1300)
	a.openSettings()
	a.settingsPage = "appearance"
	tt.Frame()
	keyword := func() ui.Color { return a.md.Theme.CodeColors.Tokens[editor.TokenKeyword].Color }

	// The menu: Default, the editors' themes, then the others by name, all
	// with a light and a dark side.
	about := "The colors of code, in the app's light or dark as the app is."
	toggle(t, tt, "Code theme", about)
	want := []string{"Default", "-", "VS Code", "Xcode", "One", "-", "Gruvbox", "Kanagawa", "Night Owl", "Tokyo Night", "Vitesse"}
	if got := tt.Menu(); !slices.Equal(got, want) {
		t.Fatalf("menu %q, want %q", got, want)
	}

	// One colors the tokens, on the app's surface, as Default's are.
	surface := a.md.Theme.CodeBlockBackground
	if err := tt.ChooseMenuItem("One"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.codeTheme != "one" || keyword() != ui.Hex("#a626a4") {
		t.Errorf("theme %q, keywords %v, want One Light's", a.codeTheme, keyword())
	}
	if a.md.Theme.CodeBlockBackground != surface || a.md.CodeBlock != nil {
		t.Errorf("code on %v, want the app's %v", a.md.Theme.CodeBlockBackground, surface)
	}
	// And follows the app's appearance.
	tt.SetDark(true)
	tt.Advance(time.Second)
	if keyword() != ui.Hex("#c678dd") || a.md.Theme.CodeBlockBackground != darkPalette.md.CodeBlockBackground {
		t.Errorf("in the dark app: keywords %v on %v, want One Dark's on the app's", keyword(), a.md.Theme.CodeBlockBackground)
	}

	// Kept for the next start; a theme no longer offered is Default.
	a.layoutSaver.wait()
	b := newApp(&demo{})
	b.loadLayout(dir)
	if b.codeTheme != "one" {
		t.Errorf("after a restart: code theme %q", b.codeTheme)
	}
	os.WriteFile(filepath.Join(dir, "layout.json"), []byte(`{"codeTheme":"nord"}`), 0o644)
	b = newApp(&demo{})
	b.loadLayout(dir)
	if b.codeTheme != "" {
		t.Errorf("Nord, gone, loaded as %q", b.codeTheme)
	}

	// Default again: MyGo's colors.
	toggle(t, tt, "Code theme", about)
	if err := tt.ChooseMenuItem("Default"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.codeTheme != "" || a.md.Theme.CodeColors != darkPalette.md.CodeColors {
		t.Errorf("theme %q, colors not MyGo's", a.codeTheme)
	}
}

func TestSettingsSearchContents(t *testing.T) {
	a := newApp(&demo{})
	tt := ui.NewTester(a.view, 1120, 1500)
	a.openSettings()
	tt.Frame()
	search := func(q string) {
		t.Helper()
		a.settingsQuery = q
		tt.Frame()
		tt.Advance(time.Second)
	}

	// A shortcut finds its page, which says what it found there and shows
	// it alone.
	search("send")
	if a.settingsPage != "shortcuts" || !tt.HasText("Matching “send”") || !tt.HasText("Send") || tt.HasText("Next result") {
		t.Errorf("for send: page %q, texts %q", a.settingsPage, tt.Texts())
	}
	if got := a.settingsMatches(); len(got) != 1 || !slices.Equal(got[0].hits, []string{"Send"}) {
		t.Errorf("for send, the pages %v", got)
	}
	// Show all, the field cleared.
	if err := tt.Click("Show all"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.settingsQuery != "" || !tt.HasText("Next result") {
		t.Errorf("show all: query %q, texts %q", a.settingsQuery, tt.Texts())
	}

	// By its keys too; more hits say how many more.
	search("esc")
	if !tt.HasText("Close the settings and 2 more") {
		t.Errorf("for esc, the sidebar %q", tt.Texts())
	}

	// What Appearance offers finds it, and the page follows.
	search("dark")
	if a.settingsPage != "appearance" || !tt.HasText("Theme") {
		t.Errorf("for dark: page %q, texts %q", a.settingsPage, tt.Texts())
	}
	search("font")
	if got := a.settingsMatches(); len(got) != 1 || !slices.Equal(got[0].hits, []string{"Font", "Code font"}) {
		t.Errorf("for font, the pages %v", got)
	}

	// A page found by its own title shows all it holds.
	search("keyboard")
	if a.settingsPage != "shortcuts" || tt.HasText("Matching “keyboard”") || !tt.HasText("Next result") {
		t.Errorf("for keyboard: page %q, texts %q", a.settingsPage, tt.Texts())
	}
}

// TestAppearanceTerms holds the terms the settings' field finds Appearance
// by to the page: each the title of one of its rows.
func TestAppearanceTerms(t *testing.T) {
	a := newApp(&demo{})
	tt := ui.NewTester(a.view, 1120, 1500)
	a.openSettings()
	a.settingsPage = "appearance"
	tt.Frame()
	texts := tt.Texts()
	for _, term := range appearanceTerms() {
		if !slices.Contains(texts, term.title) {
			t.Errorf("no %q on the Appearance page", term.title)
		}
	}
}

func TestShortcutsPage(t *testing.T) {
	a := newApp(&demo{})
	tt := ui.NewTester(a.view, 1120, 1500)
	a.openSettings()
	a.settingsPage = "shortcuts"
	tt.Frame()

	// Every shortcut, by where it works, its keys drawn.
	texts := tt.Texts()
	for _, g := range shortcutGroups() {
		if !slices.Contains(texts, g.title) {
			t.Errorf("no group %q", g.title)
		}
		for _, r := range g.rows {
			if !slices.Contains(texts, r.title) {
				t.Errorf("no row %q", r.title)
			}
			for _, k := range r.keys {
				for _, cap := range k.caps() {
					if !strings.ContainsRune("←→↑↓", []rune(cap)[0]) && !slices.Contains(texts, cap) {
						t.Errorf("%q: no key %q", r.title, cap)
					}
				}
			}
		}
	}

	// And the keys the page gives do what it says.
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Advance(time.Second)
	if a.settings {
		t.Errorf("%v left the settings open", settingsKeys)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyS)
	tt.Advance(time.Second)
	if !a.collapsed {
		t.Errorf("%v did not hide the sidebar", sidebarKeys)
	}
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Advance(time.Second)
	if !a.searching {
		t.Errorf("%v did not open the search", findKeys)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	a.open("1")
	tt.Key(ui.Cmd, ui.KeyN)
	tt.Advance(time.Second)
	if a.selected != "" {
		t.Errorf("%v did not start a new chat", newChatKeys)
	}
	if runtime.GOOS == "darwin" {
		tt.Key(ui.Super, ui.KeyBracketLeft)
	} else {
		tt.Key(ui.Alt, ui.KeyLeft)
	}
	tt.Advance(time.Second)
	if a.selected != "1" {
		t.Errorf("%v did not go back: %q", backKeys, a.selected)
	}
}

// TestCodeThemesRead holds every code theme to what docs/code-themes.md
// asks of them, on the app's own code surfaces, light and dark: text at
// 4.5:1 at least, as WCAG's AA asks of text; keywords, strings, functions,
// types and numbers at 3:1; comments, faint by design, at 2.3:1.
func TestCodeThemesRead(t *testing.T) {
	luminance := func(c ui.Color) float64 {
		f := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= 0.04045 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
	}
	contrast := func(a, b ui.Color) float64 {
		la, lb := luminance(a), luminance(b)
		return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
	}
	floors := []struct {
		kind  editor.TokenKind
		floor float64
	}{
		{editor.TokenText, 4.5},
		{editor.TokenKeyword, 3}, {editor.TokenString, 3}, {editor.TokenFunction, 3},
		{editor.TokenType, 3}, {editor.TokenNumber, 3},
		{editor.TokenComment, 2.3},
	}
	for _, ct := range codeThemes[1:] {
		for _, p := range []*palette{lightPalette, darkPalette} {
			if ct.light == nil || ct.dark == nil {
				t.Fatalf("%s has no light side or no dark one", ct.title)
			}
			et := ct.colors(p.theme.Dark).editorTheme(p.theme)
			surface := p.md.CodeBlockBackground
			for _, f := range floors {
				if r := contrast(et.TokenColor(f.kind), surface); r < f.floor {
					t.Errorf("%s (dark %v): token %d at %.2f:1, under %.1f:1", ct.title, p.theme.Dark, f.kind, r, f.floor)
				}
			}
		}
	}
}

func TestReduceMotionSwitchSlides(t *testing.T) {
	a := newApp(&demo{})
	tt := ui.NewTester(a.view, 1120, 1300)
	a.openSettings()
	a.settingsPage = "appearance"
	tt.Advance(time.Second)
	title, about := "Reduce motion", "Show changes at once rather than fading or gliding. On whenever the system asks for less motion."
	r, _ := tt.Find(title)
	d, _ := tt.Find(about)
	right, y := int(r.X-16+settingsWidth-1-16), int((r.Y+d.Y+d.H)/2)
	knob := func() []uint8 {
		img := tt.Image()
		var px []uint8
		for x := right - 44; x < right; x++ {
			o := img.PixOffset(x, y)
			px = append(px, img.Pix[o:o+4]...)
		}
		return px
	}

	// Its knob slides as it turns motion off, as every switch's does, and
	// the app holds still after.
	tt.ClickAt(float32(right-12), float32(y))
	tt.Advance(70 * time.Millisecond)
	mid := knob()
	tt.Advance(time.Second)
	if !a.reduceMotion {
		t.Fatal("motion not reduced")
	}
	if slices.Equal(mid, knob()) {
		t.Errorf("the knob jumped to its end")
	}
	if c := a.motionSwitched; c.IsZero() {
		t.Errorf("the switch's time was not kept")
	}

	// And as it turns it on again.
	tt.ClickAt(float32(right-12), float32(y))
	tt.Advance(70 * time.Millisecond)
	mid = knob()
	tt.Advance(time.Second)
	if a.reduceMotion {
		t.Fatal("motion still reduced")
	}
	if slices.Equal(mid, knob()) {
		t.Errorf("the knob jumped back")
	}

	// Motion reduced again, the pointer gone, another switch jumps.
	tt.ClickAt(float32(right-12), float32(y))
	tt.Advance(time.Second)
	tt.Move(10, 10)
	tt.SetVibrancy(true)
	tt.Advance(time.Second)
	r, _ = tt.Find("Translucent sidebar")
	y = int(r.Y + 12)
	tt.ClickAt(float32(right-12), float32(y))
	tt.Advance(time.Millisecond)
	mid = knob()
	tt.Advance(time.Second)
	if a.translucent || !a.reduceMotion {
		t.Fatalf("translucent %v, motion reduced %v", a.translucent, a.reduceMotion)
	}
	if !slices.Equal(mid, knob()) {
		t.Errorf("a switch slid with motion reduced")
	}
}

// toggle clicks the switch of the settings row titled title, about which
// says more: on the right of the page's card, which the title is 16 in
// from, centered on the two.
func toggle(t *testing.T, tt *ui.Tester, title, about string) {
	t.Helper()
	r, ok := tt.Find(title)
	d, ok2 := tt.Find(about)
	if !ok || !ok2 {
		t.Fatalf("no %q or %q in %q", title, about, tt.Texts())
	}
	right := r.X - 16 + settingsWidth - 1 - 16
	tt.ClickAt(right-12, (r.Y+d.Y+d.H)/2)
	tt.Advance(time.Second)
}
