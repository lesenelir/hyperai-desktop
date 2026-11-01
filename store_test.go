package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// shape returns th's tree as replaying its log must leave it: the path
// shown, the other versions, and the version each message shows after it;
// each message by its place in the order of IDs, 1 first, 0 for none, so
// that a chat whose IDs were made anew has the same shape.
func shape(th *thread) string {
	all := th.all()
	slices.SortFunc(all, func(x, y *message) int { return compareIDs(x.ID, y.ID) })
	nth := map[ID]int{}
	for i, m := range all {
		nth[m.ID] = i + 1
	}
	var shown, others, next []int
	for _, m := range th.Messages {
		shown = append(shown, nth[m.ID])
	}
	for _, m := range all {
		if !slices.Contains(th.Messages, m) {
			others = append(others, nth[m.ID])
		}
		next = append(next, nth[m.Next])
	}
	return fmt.Sprintf("shown %v, others %v, next %v", shown, others, next)
}

// compareIDs orders IDs as they were made: those that are numbers, of the
// chats kept before UUIDs, by their numbers.
func compareIDs(x, y ID) int {
	if x.legacy() && y.legacy() {
		a, _ := strconv.Atoi(string(x))
		b, _ := strconv.Atoi(string(y))
		return cmp.Compare(a, b)
	}
	return cmp.Compare(x, y)
}

// legacyJSON returns threads as chats.json kept them before IDs were
// UUIDs: their IDs numbers, in the order the IDs were made.
func legacyJSON(t *testing.T, threads []*thread) []byte {
	t.Helper()
	b, err := json.MarshalIndent(threads, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var ids []ID
	for _, th := range threads {
		ids = append(ids, th.ID)
		for _, m := range th.all() {
			ids = append(ids, m.ID)
		}
	}
	slices.SortFunc(ids, compareIDs)
	for i, id := range ids {
		b = bytes.ReplaceAll(b, []byte(`"`+string(id)+`"`), []byte(strconv.Itoa(i+1)))
	}
	return b
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
func lines(t *testing.T, dir string, id ID) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, chatsDir, string(id)+".jsonl"))
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
	// Chats and messages to come take IDs past them.
	b.open(string(th.ID))
	b.send("Q3")
	q := b.threads[0].Messages[len(b.threads[0].Messages)-2]
	for _, m := range th.all() {
		if q.Text != "Q3" || q.ID <= m.ID || q.ID <= th.ID {
			t.Fatalf("a new message took ID %s, after %s", q.ID, m.ID)
		}
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
	raw, _ := os.ReadFile(filepath.Join(dir, chatsDir, string(th.ID)+".jsonl"))
	if !bytes.Contains(raw, []byte("<b>First</b> & more")) {
		t.Errorf("the log escapes HTML: %s", raw)
	}
}

// A question is kept as it is sent, before its reply ends; what came of
// the reply is kept aside as it streams, which a crash leaves, and the
// next launch shows as a reply stopped there; the reply, once it ends.
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
	// The app crashes here: what it left, read by the next launch.
	a.logs.wait()
	crashed := copyChats(t, dir)
	if got := lines(t, crashed, th.ID); !slices.Equal(got, []string{"chat", "message", "title"}) {
		t.Errorf("log while the reply streams: %v", got)
	}
	b := newApp(p)
	b.loadFrom(crashed)
	b.logs.wait()
	got := b.threads[0]
	if path(got) != "Held | Reply to Held" || got.Messages[1].Error != "" || got.Messages[1].Time.IsZero() {
		t.Errorf("after a crash: path %q, error %q", path(got), got.Messages[1].Error)
	}
	if want := []string{"chat", "message", "title", "message"}; !slices.Equal(lines(t, crashed, th.ID), want) {
		t.Errorf("recovered reply not logged: %v", lines(t, crashed, th.ID))
	}
	if _, err := os.Stat(filepath.Join(crashed, chatsDir, string(th.ID)+".partial")); err == nil {
		t.Error("the partial is still there once logged")
	}
	// Recovered once: launching again reads it from the log alone.
	c := restart(t, b, crashed)
	if got := path(c.threads[0]); got != "Held | Reply to Held" || len(c.threads[0].all()) != 2 {
		t.Errorf("launched again: %q, %d messages", got, len(c.threads[0].all()))
	}

	a.stop(th)
	d := restart(t, a, dir)
	if got := path(d.threads[0]); got != "Held | Reply to Held" {
		t.Errorf("once stopped, kept %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, chatsDir, string(th.ID)+".partial")); err == nil {
		t.Error("the partial is still there once the reply ended")
	}
}

// copyChats returns a folder with a copy of the chats kept in dir.
func copyChats(t *testing.T, dir string) string {
	t.Helper()
	out := t.TempDir()
	if err := os.CopyFS(filepath.Join(out, chatsDir), os.DirFS(filepath.Join(dir, chatsDir))); err != nil {
		t.Fatal(err)
	}
	return out
}

// A write that fails is written again by the next save: the chat's log
// whole, and the logs of the chats deleted.
func TestLogRetriesFailedWrites(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("writes in a folder of mode 0500 succeed")
	}
	dir := t.TempDir()
	cdir := filepath.Join(dir, chatsDir)
	os.Mkdir(cdir, 0o500)
	a := newApp(&counter{})
	a.loadFrom(dir)
	t.Cleanup(func() { os.Chmod(cdir, 0o700); a.logs.wait() })
	a.send("Q1")
	a.logs.wait()
	if _, err := os.Stat(filepath.Join(cdir, string(a.threads[0].ID)+".jsonl")); err == nil {
		t.Fatal("written in a folder that takes no files")
	}
	os.Chmod(cdir, 0o700)
	a.save()
	b := restart(t, a, dir)
	if len(b.threads) != 1 || path(b.threads[0]) != "Q1 | Answer 1" {
		t.Fatalf("after the retry: %d chats", len(b.threads))
	}
	if want := []string{"chat", "message", "message", "title", "show"}; !slices.Equal(lines(t, dir, b.threads[0].ID), want) {
		t.Errorf("rewritten log %v, want %v", lines(t, dir, b.threads[0].ID), want)
	}
	// And appends on from there.
	b.open(string(b.threads[0].ID))
	b.send("Q2")
	c := restart(t, b, dir)
	if got := path(c.threads[0]); got != "Q1 | Answer 1 | Q2 | Answer 2" {
		t.Errorf("after appending: %q", got)
	}

	// A delete that fails is asked again.
	os.Chmod(cdir, 0o500)
	c.remove(c.threads[0])
	c.logs.wait()
	os.Chmod(cdir, 0o700)
	c.save()
	if d := restart(t, c, dir); len(d.threads) != 0 {
		t.Errorf("the deleted chat came back")
	}
}

// The sweep deletes the attachments no log names, once they are old
// enough not to be a question's whose line is to come.
func TestSweepAttachments(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.loadFrom(dir)
	a.files = []*attachment{{Name: "kept.txt", Type: typeText, data: []byte("kept")}}
	a.send("With a file")
	a.logs.wait()
	a.writes.Wait()
	files := filepath.Join(dir, "attachments")
	kept := a.threads[0].Messages[0].Files[0].File
	old := time.Now().Add(-2 * sweepGrace)
	for name, at := range map[string]time.Time{"orphan.txt": old, "new.txt": time.Now()} {
		os.WriteFile(filepath.Join(files, name), []byte(name), 0o600)
		os.Chtimes(filepath.Join(files, name), at, at)
	}
	os.Chtimes(filepath.Join(files, kept), old, old)

	b := newApp(&counter{})
	b.loadFrom(dir)
	b.writes.Wait()
	var left []string
	entries, _ := os.ReadDir(files)
	for _, e := range entries {
		left = append(left, e.Name())
	}
	slices.Sort(left)
	if want := []string{kept, "new.txt"}; !slices.Equal(left, slices.Sorted(slices.Values(want))) {
		t.Errorf("left %v, want %v", left, want)
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
	os.Remove(filepath.Join(cdir, string(a.threads[2].ID)+".jsonl"))

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
	if len(c.threads) != 2 || c.threads[0].Title != "Named" {
		t.Errorf("with no index: %d chats", len(c.threads))
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
	if _, err := os.Stat(filepath.Join(dir, chatsDir, string(gone.ID)+".jsonl")); err == nil {
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
	b := legacyJSON(t, a.threads)
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
			t.Errorf("chat %s:\n %s\nwant\n %s", th.ID, got, want[i])
		}
		if th.Title != old[i].Title || !th.Updated.Equal(old[i].Updated) || !th.Pinned.Equal(old[i].Pinned) || th.Draft != old[i].Draft {
			t.Errorf("chat %s: %+v", th.ID, th)
		}
	}
	if m := c.threads[0].Messages; m[len(m)-1].Error != "overloaded" {
		t.Errorf("a reply's error is gone")
	}
	// New IDs, in the order of the old.
	for _, th := range c.threads {
		for _, m := range append(th.all(), &message{ID: th.ID}) {
			if m.ID.legacy() || len(m.ID) != 36 {
				t.Errorf("chat %s: ID %q", th.ID, m.ID)
			}
		}
	}
	if c.threads[0].ID >= c.threads[1].ID || c.threads[1].ID >= c.threads[2].ID {
		t.Errorf("chats out of order: %s, %s, %s", c.threads[0].ID, c.threads[1].ID, c.threads[2].ID)
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
	if strings.Contains(strings.Join(lines(t, dir, d.threads[2].ID), " "), "show show") {
		t.Errorf("log %v", lines(t, dir, d.threads[2].ID))
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

// A migration that fails saves nothing, for an index would say the chats
// were moved: they show as chats.json has them, where it can be read, and
// the next launch moves them.
func TestMigrateFails(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "chats.json")
	os.WriteFile(old, []byte(`[{"id":1,"title":"Old","messages":[`), 0o600)
	a := newApp(&counter{})
	a.loadFrom(dir)
	a.send("New")
	a.save()
	a.logs.wait()
	if len(a.threads) != 1 || a.path != "" {
		t.Errorf("unreadable chats.json: %d chats, saving to %q", len(a.threads), a.path)
	}
	if _, err := os.Stat(filepath.Join(dir, chatsDir, indexFile)); err == nil {
		t.Fatal("an index was written, which says the chats were moved")
	}

	os.WriteFile(old, []byte(`[{"id":1,"title":"Old","messages":[{"id":2,"role":"user","text":"Q",`+
		`"files":[{"name":"a.txt","type":"text/plain","file":"0123456789abcdef.txt"}]}]}]`), 0o600)
	file := filepath.Join(dir, "attachments", "0123456789abcdef.txt")
	os.MkdirAll(filepath.Dir(file), 0o700)
	os.WriteFile(file, []byte("kept"), 0o600)
	if os.Getuid() != 0 && runtime.GOOS != "windows" {
		// A folder that takes no files: the chats are read, not moved.
		os.Chmod(dir, 0o500)
		b := newApp(&counter{})
		b.loadFrom(dir)
		if len(b.threads) != 1 || b.threads[0].Title != "Old" || b.path != "" {
			t.Errorf("unwritable folder: %d chats, saving to %q", len(b.threads), b.path)
		}
		// A chat deleted then comes back at the next launch, with its
		// files: attachments/, which still takes changes, keeps them.
		b.remove(b.threads[0])
		b.writes.Wait()
		os.Chmod(dir, 0o700)
		if _, err := os.Stat(file); err != nil {
			t.Errorf("the deleted chat's file is gone: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, chatsDir+".new")); err == nil {
			finishSwap(filepath.Join(dir, chatsDir))
		}
	}
	c := newApp(&counter{})
	c.loadFrom(dir)
	t.Cleanup(func() { c.logs.wait() })
	if len(c.threads) != 1 || c.threads[0].Title != "Old" || c.path == "" || len(c.threads[0].Messages[0].Files) != 1 {
		t.Errorf("once it can be: %d chats, saving to %q", len(c.threads), c.path)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("the moved chat's file is gone: %v", err)
	}
	if _, err := os.Stat(old + ".migrated"); err != nil {
		t.Errorf("not moved: %v", err)
	}
}

// The chat shown last, by its number in layout.json, shows again once
// chats.json is moved, and at every launch after.
func TestMigrateKeepsChatShown(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "chats.json"), []byte(`[`+
		`{"id":1,"title":"First","messages":[{"id":2,"role":"user","text":"Q"}]},`+
		`{"id":3,"title":"Shown","messages":[{"id":4,"role":"user","text":"Q"}]}]`), 0o600)
	os.WriteFile(filepath.Join(dir, "layout.json"), []byte(`{"onLaunch":"last","chat":"3"}`), 0o600)
	a := newApp(&counter{})
	a.loadFrom(dir)
	if th := a.current(); th == nil || th.Title != "Shown" {
		t.Fatalf("shown %q", a.selected)
	}
	a.layoutSaver.wait()
	b := restart(t, a, dir)
	if th := b.current(); th == nil || th.Title != "Shown" {
		t.Errorf("launched again: shown %q", b.selected)
	}
}

// A reply's partial is deleted only once the reply is in the log: where
// the append fails, it stays, for a crash to leave the reply, until the
// log is written again.
func TestPartialKeptUntilLogged(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("writes to a file of mode 0400 succeed")
	}
	dir := t.TempDir()
	p := &held{gate: map[string]chan struct{}{}}
	a := newApp(p)
	a.loadFrom(dir)
	t.Cleanup(func() { a.logs.wait() })
	posts := make(chan func(), 100)
	a.post = func(f func()) { posts <- f }
	a.send("Held")
	th := a.current()
	run := func(until func() bool) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for !until() {
			select {
			case f := <-posts:
				f()
			case <-deadline:
				t.Fatal("the stream posted nothing")
			}
		}
	}
	run(func() bool { return th.busy.Text != "" })
	a.logs.wait()
	logFile, partial := filepath.Join(dir, chatsDir, string(th.ID)+".jsonl"), filepath.Join(dir, chatsDir, string(th.ID)+".partial")
	if _, err := os.Stat(partial); err != nil {
		t.Fatalf("no partial while the reply streams: %v", err)
	}
	os.Chmod(logFile, 0o400)
	close(p.release("Held"))
	run(func() bool { return th.busy == nil })
	a.logs.wait()
	if _, err := os.Stat(partial); err != nil {
		t.Fatal("the partial was deleted, its reply not in the log")
	}
	// A crash here leaves the reply, in its partial.
	if b := newApp(p); true {
		b.loadFrom(copyChats(t, dir))
		b.logs.wait()
		if got := path(b.threads[0]); got != "Held | Reply to Held" {
			t.Errorf("after a crash: %q", got)
		}
	}
	os.Chmod(logFile, 0o600)
	a.save()
	c := restart(t, a, dir)
	if got := path(c.threads[0]); got != "Held | Reply to Held, done." {
		t.Errorf("written again: %q", got)
	}
	if _, err := os.Stat(partial); err == nil {
		t.Error("the partial stays once its reply is in the log")
	}
}

// A crash between a log and the index leaves the index behind: the log's
// title and latest question win, and the next save writes no title back.
func TestIndexBehindLog(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.loadFrom(dir)
	a.send("Q1")
	th := a.current()
	a.logs.wait()
	behind, _ := os.ReadFile(filepath.Join(dir, chatsDir, indexFile))
	th.Title = "Renamed"
	a.send("Q2")
	a.logs.wait()
	before := lines(t, dir, th.ID)
	os.WriteFile(filepath.Join(dir, chatsDir, indexFile), behind, 0o600)

	b := newApp(&counter{})
	b.loadFrom(dir)
	t.Cleanup(func() { b.logs.wait() })
	got := b.threads[0]
	if got.Title != "Renamed" || got.Updated.Before(got.Messages[2].Time) {
		t.Errorf("title %q, updated %v, last question %v", got.Title, got.Updated, got.Messages[2].Time)
	}
	b.save()
	b.logs.wait()
	if after := lines(t, dir, th.ID); !slices.Equal(after, before) {
		t.Errorf("log %v, was %v", after, before)
	}
}
