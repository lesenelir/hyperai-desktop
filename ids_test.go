package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"
)

// IDs are UUIDs that only grow, and read the numbers of the chats kept
// before as their digits.
func TestIDs(t *testing.T) {
	var ids []ID
	for range 1000 {
		ids = append(ids, newID())
	}
	if !slices.IsSorted(ids) || len(slices.Compact(slices.Clone(ids))) != len(ids) {
		t.Error("IDs made one after another do not grow")
	}
	if len(ids[0]) != 36 || ids[0].legacy() {
		t.Errorf("ID %q", ids[0])
	}
	var got struct{ A, B, C, D, E ID }
	if err := json.Unmarshal([]byte(`{"A":42,"B":0,"C":"`+string(ids[0])+`","D":null}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.A != "42" || !got.A.legacy() || got.B != "" || got.C != ids[0] || got.D != "" || got.E != "" {
		t.Errorf("read %+v", got)
	}

	// Made anew in the order of the numbers, not of the text: 9 before 10.
	th := &thread{ID: "1", Messages: []*message{{ID: "9"}, {ID: "10", Parent: "9", Next: "11"}}, Others: []*message{{ID: "11", Parent: "10"}}}
	to := upgradeIDs([]*thread{th})
	if len(to) != 4 || !(th.ID < th.Messages[0].ID && th.Messages[0].ID < th.Messages[1].ID && th.Messages[1].ID < th.Others[0].ID) {
		t.Errorf("upgraded out of order: %v", to)
	}
	if th.Messages[1].Parent != th.Messages[0].ID || th.Messages[1].Next != th.Others[0].ID || th.Others[0].Parent != th.Messages[1].ID {
		t.Error("links not upgraded")
	}
	if upgradeIDs([]*thread{th}) != nil {
		t.Error("upgraded twice")
	}
}

// v1 returns the chats kept in dir as the logs of version 1 kept them: the
// IDs numbers, in the order they were made, the logs named by them, and
// the index with the counter; and the chat layout.json shows, by the
// number its ID becomes.
func v1(t *testing.T, a *app, dir string) {
	t.Helper()
	a.logs.wait()
	a.layoutSaver.wait()
	var ids []ID
	for _, th := range a.threads {
		ids = append(ids, th.ID)
		for _, m := range th.all() {
			ids = append(ids, m.ID)
		}
	}
	slices.Sort(ids)
	num := map[ID]string{}
	for i, id := range ids {
		num[id] = strconv.Itoa(i + 1)
	}
	cdir := filepath.Join(dir, chatsDir)
	entries, _ := os.ReadDir(cdir)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(cdir, e.Name()))
		for id, n := range num {
			b = bytes.ReplaceAll(b, []byte(`"`+string(id)+`"`), []byte(n))
		}
		name := e.Name()
		if e.Name() == indexFile {
			b = bytes.Replace(b, []byte(`"version": 2,`), []byte(`"version": 1, "next": `+strconv.Itoa(len(ids))+`,`), 1)
		} else {
			os.Remove(filepath.Join(cdir, name))
			ext := filepath.Ext(name)
			name = num[ID(name[:len(name)-len(ext)])] + ext
		}
		os.WriteFile(filepath.Join(cdir, name), b, 0o600)
	}
	layout := filepath.Join(dir, "layout.json")
	b, _ := os.ReadFile(layout)
	for id, n := range num {
		b = bytes.ReplaceAll(b, []byte(`"`+string(id)+`"`), []byte(`"`+n+`"`))
	}
	os.WriteFile(layout, b, 0o600)
}

// The logs of version 1, whose IDs are numbers, are written anew with
// UUIDs, as they were otherwise, the chat shown last shown again; and the
// old folder is kept as chats.v1.
func TestUpgradeV1Logs(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.loadFrom(dir)
	t.Cleanup(func() { a.logs.wait(); a.layoutSaver.wait() })
	tt := ui.NewTester(a.view, 1120, 760)
	a.send("Q1")
	first := a.current()
	a.send("Q2")
	a.regenerate(first, 3)
	a.showVersion(first, 3, first.versions(first.Messages[3])[0])
	a.togglePin(first)
	a.newChat()
	a.send("Other")
	a.open(string(first.ID))
	tt.Frame() // which keeps the chat shown in layout.json
	var want []string
	for _, th := range a.threads {
		want = append(want, shape(th)+" "+path(th)+" "+th.Title)
	}
	v1(t, a, dir)
	if _, err := os.Stat(filepath.Join(dir, chatsDir, "1.jsonl")); err != nil {
		t.Fatalf("no log of version 1: %v", err)
	}

	b := newApp(&counter{})
	b.loadFrom(dir)
	t.Cleanup(func() { b.logs.wait() })
	if len(b.threads) != 2 {
		t.Fatalf("%d chats", len(b.threads))
	}
	for i, th := range b.threads {
		if got := shape(th) + " " + path(th) + " " + th.Title; got != want[i] {
			t.Errorf("chat %d: %s, want %s", i, got, want[i])
		}
		for _, m := range append(th.all(), &message{ID: th.ID}) {
			if m.ID.legacy() || len(m.ID) != 36 {
				t.Errorf("ID %q", m.ID)
			}
		}
	}
	if b.threads[0].Pinned.IsZero() || b.keptChat != string(b.threads[0].ID) {
		t.Errorf("pinned %v, chat shown last %q", b.threads[0].Pinned, b.keptChat)
	}
	if _, err := os.Stat(filepath.Join(dir, oldLogs, "1.jsonl")); err != nil {
		t.Errorf("the old logs are not kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, chatsDir, string(b.threads[0].ID)+".jsonl")); err != nil {
		t.Errorf("no log by the new ID: %v", err)
	}

	// Upgraded once: the next launch reads the same IDs.
	b.send("Q3")
	c := restart(t, b, dir)
	if c.threads[0].ID != b.threads[0].ID || path(c.threads[0]) != path(b.threads[0]) {
		t.Errorf("launched again: %s %q", c.threads[0].ID, path(c.threads[0]))
	}
	// layout.json keeps the chat shown last by its new ID.
	if c.keptChat != string(b.threads[0].ID) {
		t.Errorf("launched again, the chat shown last is %q", c.keptChat)
	}
}

// A crash while the chats' folder is written anew leaves the old folder
// or the new one whole: where both are, the new one is dropped; where the
// old one was renamed away, the new one takes its place.
func TestFinishSwap(t *testing.T) {
	dir := t.TempDir()
	cdir := filepath.Join(dir, chatsDir)
	os.MkdirAll(cdir, 0o700)
	os.MkdirAll(cdir+".new", 0o700)
	os.WriteFile(filepath.Join(cdir+".new", "half"), nil, 0o600)
	finishSwap(cdir)
	if _, err := os.Stat(cdir + ".new"); err == nil {
		t.Error("a new folder beside the old one is kept")
	}
	if _, err := os.Stat(cdir); err != nil {
		t.Error("the old folder is gone")
	}

	// A new folder with no index was cut short as it was written: with no
	// old one beside it, as moving chats.json leaves it, it is dropped too.
	os.Rename(cdir, filepath.Join(dir, oldLogs))
	os.MkdirAll(cdir+".new", 0o700)
	os.WriteFile(filepath.Join(cdir+".new", "half.jsonl"), nil, 0o600)
	finishSwap(cdir)
	if _, err := os.Stat(cdir + ".new"); err == nil {
		t.Error("a new folder with no index is kept")
	}
	if _, err := os.Stat(cdir); err == nil {
		t.Error("a new folder with no index took the old one's place")
	}

	os.MkdirAll(cdir+".new", 0o700)
	os.WriteFile(filepath.Join(cdir+".new", indexFile), []byte(`{"version":2,"chats":[]}`), 0o600)
	finishSwap(cdir)
	if _, err := os.Stat(filepath.Join(cdir, indexFile)); err != nil {
		t.Errorf("the new folder did not take the old one's place: %v", err)
	}
}
