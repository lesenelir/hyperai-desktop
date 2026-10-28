package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// newTester runs the view without a window, where the demo's replies come
// at once.
func newTester() (*app, *ui.Tester) {
	a := newApp(&demo{})
	return a, ui.NewTester(a.view, 1120, 760)
}

func TestSendWithEnter(t *testing.T) {
	a, tt := newTester()
	if !tt.HasText("What should we work on?") {
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
	if err := tt.Click("Write a function"); err != nil {
		t.Fatal(err)
	}
	if len(a.threads) != 1 || !tt.HasText(suggestions[0].prompt) {
		t.Fatalf("suggestion not sent: %q", tt.Texts())
	}
	if err := tt.Click("New chat"); err != nil {
		t.Fatal(err)
	}
	if a.selected != "" || !tt.HasText("What should we work on?") {
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
