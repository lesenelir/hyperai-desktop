package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// shape returns th's tree as replaying its log must leave it: the path
// shown, the other versions, and the version each message shows after it.
func shape(th *thread) string {
	var shown, others, next []string
	for _, m := range th.Messages {
		shown = append(shown, fmt.Sprint(m.ID))
	}
	all := th.all()
	slices.SortFunc(all, func(x, y *message) int { return x.ID - y.ID })
	for _, m := range all {
		if !slices.Contains(th.Messages, m) {
			others = append(others, fmt.Sprint(m.ID))
		}
		next = append(next, fmt.Sprintf("%d→%d", m.ID, m.Next))
	}
	return fmt.Sprintf("shown %v, others %v, next %v", shown, others, next)
}

// restart returns an app that loads what a kept in dir, once it is written.
func restart(t *testing.T, a *app, dir string) *app {
	t.Helper()
	a.logs.wait()
	b := newApp(a.prov)
	b.loadFrom(dir)
	t.Cleanup(func() { b.logs.wait() })
	return b
}

// lines returns the events of the log of the chat id in dir, by type.
func lines(t *testing.T, dir string, id int) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, chatsDir, fmt.Sprint(id)+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for line := range bytes.Lines(b) {
		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("not an event: %q", line)
		}
		out = append(out, e.T)
	}
	return out
}

// A chat's log replays to the tree it was: sent, asked again, edited, and
// its versions gone through, every message shows the version after it it
// showed last.
func TestLogReplaysVersions(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.loadFrom(dir)
	t.Cleanup(func() { a.logs.wait() })
	a.send("Q1")
	th := a.current()
	a.send("Q2")
	a.regenerate(th, 3)
	a.regenerate(th, 3)
	a.resend(th, 2, "Q2 edited")
	a.regenerate(th, 1)
	// Back to the first answer to Q1, then to Q2 before it was edited,
	// and its second answer; then to the last answer to Q1 and back, which
	// shows the first as it was left.
	a.showVersion(th, 1, th.versions(th.Messages[1])[0])
	a.showVersion(th, 2, th.versions(th.Messages[2])[0])
	a.showVersion(th, 3, th.versions(th.Messages[3])[1])
	a.showVersion(th, 1, th.versions(th.Messages[1])[1])
	a.showVersion(th, 1, th.versions(th.Messages[1])[0])
	if got := path(th); got != "Q1 | Answer 1 | Q2 | Answer 3" {
		t.Fatalf("path %q", got)
	}
	want := shape(th)

	b := restart(t, a, dir)
	if len(b.threads) != 1 {
		t.Fatalf("%d chats", len(b.threads))
	}
	if got := shape(b.threads[0]); got != want {
		t.Errorf("replayed:\n %s\nwant\n %s", got, want)
	}
	if got, want := path(b.threads[0]), path(th); got != want {
		t.Errorf("path %q, want %q", got, want)
	}
	if b.next != a.next {
		t.Errorf("next %d, want %d", b.next, a.next)
	}
	// Chats and messages to come take IDs past them.
	b.open(fmt.Sprint(th.ID))
	b.send("Q3")
	if m := b.threads[0].Messages; m[len(m)-2].Text != "Q3" || m[len(m)-2].ID <= a.next {
		t.Errorf("a new message took ID %d", m[len(m)-2].ID)
	}
}

// What the index keeps comes back, and each change appends to the log
// what it lacks, not all of it again.
func TestLogKeepsChanges(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.loadFrom(dir)
	t.Cleanup(func() { a.logs.wait() })
	a.send("<b>First</b> & more")
	th := a.current()
	a.togglePin(th)
	th.Title = "Renamed"
	th.Draft = "Half a thought"
	a.save()
	a.save()
	b := restart(t, a, dir)
	got := b.threads[0]
	if got.Title != "Renamed" || got.Pinned.IsZero() || got.Draft != "Half a thought" || got.Provider != "counter" ||
		got.Messages[0].Text != "<b>First</b> & more" || got.Messages[1].Text != "Answer 1" {
		t.Errorf("kept %+v, path %q", got, path(got))
	}
	if want := []string{"chat", "message", "title", "message", "title"}; !slices.Equal(lines(t, dir, th.ID), want) {
		t.Errorf("log %v, want %v", lines(t, dir, th.ID), want)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, chatsDir, fmt.Sprint(th.ID)+".jsonl"))
	if !bytes.Contains(raw, []byte("<b>First</b> & more")) {
		t.Errorf("the log escapes HTML: %s", raw)
	}
}

// A question is kept as it is sent, before its reply ends; the reply as
// it ends, stopped too.
func TestLogKeepsQuestionBeforeReply(t *testing.T) {
	dir := t.TempDir()
	p := &held{gate: map[string]chan struct{}{}}
	a := newApp(p)
	a.loadFrom(dir)
	t.Cleanup(func() { a.logs.wait() })
	posts := make(chan func(), 100)
	a.post = func(f func()) { posts <- f }
	a.send("Held")
	th := a.current()
	for th.busy.Text == "" {
		select {
		case f := <-posts:
			f()
		case <-time.After(5 * time.Second):
			t.Fatal("the stream posted nothing")
		}
	}
	b := restart(t, a, dir)
	if got := path(b.threads[0]); got != "Held" {
		t.Errorf("while the reply streams, kept %q", got)
	}
	a.stop(th)
	b = restart(t, a, dir)
	if got := path(b.threads[0]); got != "Held | Reply to Held" {
		t.Errorf("once stopped, kept %q", got)
	}
}

// A last line a crash broke is dropped, and the next begins a line of its
// own; a broken line elsewhere is skipped.
func TestLogBrokenLines(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.loadFrom(dir)
	a.send("Q1")
	id := a.current().ID
	a.logs.wait()
	file := filepath.Join(dir, chatsDir, fmt.Sprint(id)+".jsonl")
	b, _ := os.ReadFile(file)
	parts := bytes.SplitAfterN(b, []byte("\n"), 2)
	b = slices.Concat(parts[0], []byte("not json\n"), parts[1], []byte(`{"t":"message","id":99,"ro`))
	os.WriteFile(file, b, 0o600)

	c := newApp(&counter{})
	c.loadFrom(dir)
	t.Cleanup(func() { c.logs.wait() })
	if got := path(c.threads[0]); got != "Q1 | Answer 1" {
		t.Fatalf("path %q", got)
	}
	c.open(fmt.Sprint(id))
	c.send("Q2")
	d := restart(t, c, dir)
	// Another app, whose counter counts from 1 again.
	if got := path(d.threads[0]); got != "Q1 | Answer 1 | Q2 | Answer 1" {
		t.Errorf("after the broken line: path %q", got)
	}
}

// The index is repaired against the logs: a log it lacks is listed, named
// by its title, and an entry without a log is dropped; with no index at
// all, the logs make one.
func TestIndexRepaired(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.loadFrom(dir)
	a.send("Kept")
	a.newChat()
	a.send("Lost index")
	a.newChat()
	a.send("Lost log")
	named := a.threads[0]
	named.Title = "Named"
	a.save()
	a.logs.wait()
	cdir := filepath.Join(dir, chatsDir)
	var idx chatIndex
	b, _ := os.ReadFile(filepath.Join(cdir, indexFile))
	json.Unmarshal(b, &idx)
	idx.Chats = slices.Delete(idx.Chats, 1, 2)
	b, _ = json.Marshal(idx)
	os.WriteFile(filepath.Join(cdir, indexFile), b, 0o600)
	os.Remove(filepath.Join(cdir, fmt.Sprint(a.threads[2].ID)+".jsonl"))

	c := newApp(&counter{})
	c.loadFrom(dir)
	var titles []string
	for _, th := range c.threads {
		titles = append(titles, th.Title)
	}
	if want := []string{"Named", "Lost index"}; !slices.Equal(titles, want) {
		t.Errorf("chats %q, want %q", titles, want)
	}
	if th := c.threads[1]; th.Updated.IsZero() {
		t.Errorf("a chat out of the index has no date")
	}

	os.Remove(filepath.Join(cdir, indexFile))
	c = newApp(&counter{})
	c.loadFrom(dir)
	if len(c.threads) != 2 || c.threads[0].Title != "Named" || c.next < a.threads[1].Messages[1].ID {
		t.Errorf("with no index: %d chats, next %d", len(c.threads), c.next)
	}
}

// A chat deleted takes its log with it, and the index its entry.
func TestLogDeleted(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.loadFrom(dir)
	a.send("Gone")
	gone := a.current()
	a.newChat()
	a.send("Stays")
	a.remove(gone)
	b := restart(t, a, dir)
	if len(b.threads) != 1 || b.threads[0].Title != "Stays" {
		t.Errorf("after deleting: %d chats", len(b.threads))
	}
	if _, err := os.Stat(filepath.Join(dir, chatsDir, fmt.Sprint(gone.ID)+".jsonl")); err == nil {
		t.Error("the deleted chat's log is still there")
	}
	b.removeAll()
	c := restart(t, b, dir)
	if len(c.threads) != 0 {
		t.Errorf("after deleting all: %d chats", len(c.threads))
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, chatsDir)); len(entries) != 1 {
		t.Errorf("left %d files, want the index", len(entries))
	}
}

// chats.json moves into logs as it was: each chat's tree and the versions
// it showed last, its title, pin and draft, a reply that failed; and is
// kept beside them, renamed.
func TestMigrateChatsJSON(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.send("Q1")
	th := a.current()
	a.send("Q2")
	a.regenerate(th, 3)
	a.resend(th, 2, "Q2 edited")
	a.regenerate(th, 1)
	a.showVersion(th, 1, th.versions(th.Messages[1])[0])
	a.showVersion(th, 3, th.versions(th.Messages[3])[0])
	th.Messages[len(th.Messages)-1].Error = "overloaded"
	th.Pinned, th.Draft = time.Now().UTC().Truncate(time.Second), "Draft"
	a.newChat()
	a.send("Other")
	b, err := json.MarshalIndent(a.threads, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	// One kept before versions, whose messages follow none.
	b = append(b[:len(b)-1], []byte(`,{"id":900,"title":"Old","updated":"2025-01-02T03:04:05Z","messages":[`+
		`{"id":901,"role":"user","text":"Q"},{"id":902,"role":"assistant","text":"A"}]}]`)...)
	os.WriteFile(filepath.Join(dir, "chats.json"), b, 0o600)
	var want []string
	var old []*thread
	json.Unmarshal(b, &old)
	for _, th := range old {
		th.link()
		want = append(want, shape(th))
	}

	c := newApp(&counter{})
	c.loadFrom(dir)
	t.Cleanup(func() { c.logs.wait() })
	if len(c.threads) != 3 {
		t.Fatalf("%d chats", len(c.threads))
	}
	for i, th := range c.threads {
		if got := shape(th); got != want[i] {
			t.Errorf("chat %d:\n %s\nwant\n %s", th.ID, got, want[i])
		}
		if th.Title != old[i].Title || !th.Updated.Equal(old[i].Updated) || !th.Pinned.Equal(old[i].Pinned) || th.Draft != old[i].Draft {
			t.Errorf("chat %d: %+v", th.ID, th)
		}
	}
	if m := c.threads[0].Messages; m[len(m)-1].Error != "overloaded" {
		t.Errorf("a reply's error is gone")
	}
	if c.next != 902 {
		t.Errorf("next %d", c.next)
	}
	if _, err := os.Stat(filepath.Join(dir, "chats.json")); err == nil {
		t.Error("chats.json is still there")
	}
	if _, err := os.Stat(filepath.Join(dir, "chats.json.migrated")); err != nil {
		t.Errorf("chats.json not kept: %v", err)
	}
	// Moved once: the logs are read from then on.
	c.threads[1].Title = "Changed"
	c.save()
	d := restart(t, c, dir)
	if d.threads[1].Title != "Changed" || shape(d.threads[0]) != want[0] {
		t.Errorf("after moving: %q, %s", d.threads[1].Title, shape(d.threads[0]))
	}
	if strings.Contains(strings.Join(lines(t, dir, 900), " "), "show show") {
		t.Errorf("log %v", lines(t, dir, 900))
	}
}

// Two chats whose replies end at once each append to their own log, in
// order (go test -race).
func TestLogsWrittenAtOnce(t *testing.T) {
	dir := t.TempDir()
	p := &held{gate: map[string]chan struct{}{}}
	a := newApp(p)
	a.loadFrom(dir)
	t.Cleanup(func() { a.logs.wait() })
	posts := make(chan func(), 100)
	a.post = func(f func()) { posts <- f }
	var chats []*thread
	for _, q := range []string{"A", "B"} {
		a.newChat()
		a.send(q)
		chats = append(chats, a.current())
	}
	close(p.release("A"))
	close(p.release("B"))
	deadline := time.After(5 * time.Second)
	for chats[0].busy != nil || chats[1].busy != nil {
		select {
		case f := <-posts:
			f()
		case <-deadline:
			t.Fatal("the replies did not end")
		}
	}
	b := restart(t, a, dir)
	for i, q := range []string{"A", "B"} {
		if got, want := path(b.threads[i]), q+" | Reply to "+q+", done."; got != want {
			t.Errorf("chat %s: %q, want %q", q, got, want)
		}
		if want := []string{"chat", "message", "title", "message"}; !slices.Equal(lines(t, dir, chats[i].ID), want) {
			t.Errorf("log of %s: %v, want %v", q, lines(t, dir, chats[i].ID), want)
		}
	}
}
