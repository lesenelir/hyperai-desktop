package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// The chats are kept in a folder of their own, as docs/storage.md says: a
// log of each, which is only appended to, and an index of them all, for
// the sidebar, which is written whole.
const (
	chatsDir   = "chats"
	indexFile  = "index.json"
	logVersion = 1
	// oldChats is where every chat was kept before, in one file, which
	// the first launch since moves into logs, renaming it oldChats +
	// ".migrated".
	oldChats = "chats.json"
	// partialEvery is how often the reply streaming in a chat is kept
	// aside, in <id>.partial, for a crash to leave what came of it.
	partialEvery = time.Second
	// sweepGrace is how old a file in attachments/ that no log names must
	// be for the sweep to delete it: younger, it may be a question's whose
	// line is not written yet.
	sweepGrace = time.Hour
)

// chatIndex is what index.json holds: the counter of the IDs, and of each
// chat what the sidebar shows and what changes often.
type chatIndex struct {
	Version int          `json:"version"`
	Next    int          `json:"next"`
	Chats   []indexEntry `json:"chats"`
}

// indexEntry is a chat as the index keeps it: a thread's fields but its
// messages.
type indexEntry struct {
	ID       int       `json:"id"`
	Title    string    `json:"title"`
	Updated  time.Time `json:"updated"`
	Pinned   time.Time `json:"pinned,omitzero"`
	Model    string    `json:"model,omitempty"`
	Effort   string    `json:"effort,omitempty"`
	Provider string    `json:"provider,omitempty"`
	Draft    string    `json:"draft,omitempty"`
}

// event is a line of a chat's log: its type, when it happened, and what
// the types but "message" carry. Of the log's first line ("chat"), the
// log's version and the chat's ID; of "title", the chat's title; of
// "show", the last message of the path shown.
type event struct {
	T     string    `json:"t"`
	At    time.Time `json:"at,omitzero"`
	V     int       `json:"v,omitempty"`
	ID    int       `json:"id,omitempty"`
	Title string    `json:"title,omitempty"`
	Leaf  int       `json:"leaf,omitempty"`
}

// messageEvent is a line of the type "message": the message as its json
// tags write it, but Next, which replaying the log makes.
type messageEvent struct {
	T  string    `json:"t"`
	At time.Time `json:"at,omitzero"`
	*message
}

// load reads the saved chats from the app's user data directory.
func (a *app) load() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return
	}
	a.loadFrom(dir)
}

// loadFrom reads the layout and the chats kept in dir, moving them out of
// chats.json the first time, and shows the chat shown last, where General
// says to and it is still there.
func (a *app) loadFrom(dir string) {
	a.loadLayout(dir)
	a.loadConf(dir)
	defer func() {
		if a.onLaunch == launchLast && a.thread(a.keptChat) != nil {
			a.show(a.keptChat)
		}
	}()
	a.path = filepath.Join(dir, chatsDir)
	a.filesDir = filepath.Join(dir, "attachments")
	if err := migrate(dir); err != nil {
		log.Printf("moving the chats out of %s: %v", oldChats, err)
	}
	a.threads, a.next = readChats(a.path)
	a.onDisk = map[int]bool{}
	recovered := false
	for _, th := range a.threads {
		a.onDisk[th.ID] = true
		recovered = recovered || th.partial
	}
	a.rev++
	// A reply a crash cut short goes in its log at once.
	if recovered {
		a.save()
	}
	chats, files := a.path, a.filesDir
	a.writes.Add(1)
	go func() {
		defer a.writes.Done()
		sweepAttachments(chats, files, time.Now().Add(-sweepGrace))
	}()
}

// save keeps what changed of the chats, where the app has a place for them
// (not in tests): it appends to each chat's log what it does not hold yet,
// deletes the logs of the chats deleted, and writes the index. It encodes
// them here, on the main thread, where they are safe to read, and leaves
// writing them to a goroutine, so that a frame never waits for the disk.
// A reply streaming in is not kept until it ends.
func (a *app) save() {
	if a.path == "" {
		return
	}
	a.retryFailed()
	var ops []logOp
	live := make(map[int]bool, len(a.threads))
	idx := chatIndex{Version: logVersion, Next: a.next, Chats: make([]indexEntry, 0, len(a.threads))}
	for _, th := range a.threads {
		live[th.ID] = true
		ops = append(ops, a.logChanges(th)...)
		idx.Chats = append(idx.Chats, indexEntry{ID: th.ID, Title: th.Title, Updated: th.Updated, Pinned: th.Pinned,
			Model: th.Model, Effort: th.Effort, Provider: th.Provider, Draft: th.Draft})
	}
	// The logs go before the index, so that the index never holds a chat
	// that a crash left without a log.
	for id := range a.onDisk {
		if !live[id] {
			ops = append(ops, logOp{id: id, path: a.logPath(id), kind: opRemove},
				logOp{path: a.partialPath(id), kind: opRemove})
			delete(a.onDisk, id)
		}
	}
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return
	}
	a.logs.write(ops, filepath.Join(a.path, indexFile), b)
}

func (a *app) logPath(id int) string { return filepath.Join(a.path, strconv.Itoa(id)+".jsonl") }

func (a *app) partialPath(id int) string {
	return filepath.Join(a.path, strconv.Itoa(id)+".partial")
}

// retryFailed has the next save write again what the writer could not:
// the log of a chat an append to failed, whole, and the delete of a log
// that failed.
func (a *app) retryFailed() {
	for _, op := range a.logs.takeFailed() {
		switch op.kind {
		case opRemove:
			if op.id != 0 {
				a.onDisk[op.id] = true
			}
		default:
			if th := a.threadByID(op.id); th != nil {
				th.rewrite = true
			}
		}
	}
}

func (a *app) threadByID(id int) *thread {
	for _, th := range a.threads {
		if th.ID == id {
			return th
		}
	}
	return nil
}

// logChanges returns the changes th's log needs: where an append to it
// failed, all of it written anew; else the lines it lacks, appended. And,
// once its reply ended, its partial deleted.
func (a *app) logChanges(th *thread) []logOp {
	var ops []logOp
	if th.rewrite {
		th.rewrite = false
		a.onDisk[th.ID] = true
		ops = append(ops, logOp{id: th.ID, path: a.logPath(th.ID), data: wholeLog(th), kind: opReplace})
	} else if b := a.newLines(th); len(b) > 0 {
		ops = append(ops, logOp{id: th.ID, path: a.logPath(th.ID), data: b})
	}
	if th.partial && th.busy == nil {
		th.partial = false
		ops = append(ops, logOp{path: a.partialPath(th.ID), kind: opRemove})
	}
	return ops
}

// wholeLog returns th's log as replaying it shows th, and notes it holds
// all of it: its first line, the messages that are not streaming in, in
// replay order, its title, and the path shown.
func wholeLog(th *thread) []byte {
	th.logged = map[int]bool{}
	out := appendLine(nil, event{T: "chat", At: time.Now().UTC(), V: logVersion, ID: th.ID})
	for _, m := range replayOrder(th) {
		if m.streaming {
			continue
		}
		out = appendMessage(out, m, m.Time)
		th.logged[m.ID] = true
	}
	th.logTitle = th.Title
	if th.Title != "" {
		out = appendLine(out, event{T: "title", Title: th.Title})
	}
	th.logLeaf = shownLeaf(th)
	if th.logLeaf != 0 {
		out = appendLine(out, event{T: "show", Leaf: th.logLeaf})
	}
	return out
}

// keepPartial keeps reply, streaming in th, aside in th's partial, at most
// once every partialEvery: what a crash leaves of it, which the next
// launch shows as a reply stopped there.
func (a *app) keepPartial(th *thread, reply *message) {
	if a.path == "" || !reply.streaming || time.Since(th.partialAt) < partialEvery {
		return
	}
	th.partialAt, th.partial = time.Now(), true
	b := appendMessage(nil, reply, time.Now().UTC())
	a.logs.write([]logOp{{path: a.partialPath(th.ID), data: b, kind: opReplace}}, "", nil)
}

// newLines returns the lines th's log lacks: its first, where it has none
// yet; the messages that ended since, oldest first; the path shown, where
// replaying the log would show another; and its title, where it changed.
func (a *app) newLines(th *thread) []byte {
	var out []byte
	now := time.Now().UTC()
	if th.logged == nil {
		th.logged = map[int]bool{}
		a.onDisk[th.ID] = true
		out = appendLine(out, event{T: "chat", At: now, V: logVersion, ID: th.ID})
	}
	if len(th.logged) < len(th.Messages)+len(th.Others) {
		msgs := th.all()
		slices.SortFunc(msgs, func(x, y *message) int { return x.ID - y.ID })
		for _, m := range msgs {
			if m.streaming || th.logged[m.ID] {
				continue
			}
			out = appendMessage(out, m, now)
			th.logged[m.ID] = true
			th.logLeaf = m.ID
		}
	}
	if leaf := shownLeaf(th); leaf != 0 && leaf != th.logLeaf {
		out = appendLine(out, event{T: "show", At: now, Leaf: leaf})
		th.logLeaf = leaf
	}
	if th.Title != th.logTitle {
		out = appendLine(out, event{T: "title", At: now, Title: th.Title})
		th.logTitle = th.Title
	}
	return out
}

// shownLeaf returns the last message of th's path that is not streaming
// in, 0 for none.
func shownLeaf(th *thread) int {
	for i := len(th.Messages) - 1; i >= 0; i-- {
		if !th.Messages[i].streaming {
			return th.Messages[i].ID
		}
	}
	return 0
}

// appendLine appends v to b as a line of JSON, its HTML as it is.
func appendLine(b []byte, v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		log.Printf("encoding a line of a chat's log: %v", err)
		return b
	}
	return append(b, buf.Bytes()...)
}

// appendMessage appends m to b as a "message" line: as it is, but Next,
// which replaying the log makes.
func appendMessage(b []byte, m *message, at time.Time) []byte {
	c := *m
	c.Next = 0
	return appendLine(b, messageEvent{T: "message", At: at, message: &c})
}

// readChats returns the chats kept in dir, and the next ID to give: those
// the index lists, in its order, whose logs are there, then those of the
// logs it lacks, by ID, named and dated by their logs.
func readChats(dir string) ([]*thread, int) {
	var idx chatIndex
	if b, err := os.ReadFile(filepath.Join(dir, indexFile)); err == nil {
		if err := json.Unmarshal(b, &idx); err != nil {
			log.Printf("reading the chats' index: %v", err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Printf("reading the chats: %v", err)
	}
	logs := map[int]*thread{}
	var ids []int
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".jsonl")
		id, err := strconv.Atoi(name)
		if !ok || err != nil || id <= 0 || e.IsDir() {
			continue
		}
		if th := readLog(filepath.Join(dir, e.Name()), id); th != nil {
			recoverPartial(filepath.Join(dir, name+".partial"), th)
			logs[id] = th
			ids = append(ids, id)
		}
	}
	next := idx.Next
	threads := make([]*thread, 0, len(logs))
	for _, e := range idx.Chats {
		th := logs[e.ID]
		if th == nil {
			continue
		}
		delete(logs, e.ID)
		th.Title, th.Updated, th.Pinned = e.Title, e.Updated, e.Pinned
		th.Model, th.Effort, th.Provider, th.Draft = e.Model, e.Effort, e.Provider, e.Draft
		threads = append(threads, th)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if th := logs[id]; th != nil {
			th.Title = th.logTitle
			if th.Title == "" {
				th.Title = firstTitle(th)
			}
			threads = append(threads, th)
		}
	}
	for _, th := range threads {
		next = max(next, th.ID)
		for m := range th.logged {
			next = max(next, m)
		}
	}
	return threads, next
}

// firstTitle returns the title send gives th: of its first question, or
// its first file, where it is of files alone.
func firstTitle(th *thread) string {
	for _, m := range th.all() {
		if m.Role != "user" {
			continue
		}
		if m.Text == "" && len(m.Files) > 0 {
			return title(m.Files[0].Name)
		}
		return title(m.Text)
	}
	return ""
}

// readLog returns the chat id whose log is at path, nil where it holds no
// message. A last line a crash broke is cut from the file, for the next
// to begin a line of its own; any other line that is not an event is
// skipped.
func readLog(path string, id int) *thread {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("reading a chat's log: %v", err)
		return nil
	}
	if n := len(b); n > 0 && b[n-1] != '\n' {
		keep := bytes.LastIndexByte(b, '\n') + 1
		log.Printf("%s: cutting a broken last line", path)
		if err := os.Truncate(path, int64(keep)); err != nil {
			log.Printf("cutting it: %v", err)
		}
		b = b[:keep]
	}
	th := &thread{ID: id, logged: map[int]bool{}}
	byID := map[int]*message{}
	var msgs []*message
	var created time.Time
	for line := range bytes.Lines(b) {
		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			log.Printf("%s: skipping a line: %v", path, err)
			continue
		}
		switch e.T {
		case "chat":
			created = e.At
		case "message":
			m := &message{}
			if err := json.Unmarshal(line, &messageEvent{message: m}); err != nil || m.ID <= 0 || byID[m.ID] != nil {
				log.Printf("%s: skipping a message", path)
				continue
			}
			m.Next = 0
			byID[m.ID] = m
			msgs = append(msgs, m)
			th.logged[m.ID] = true
			th.showPath(byID, m.ID)
		case "title":
			th.logTitle = e.Title
		case "show":
			if byID[e.Leaf] != nil {
				th.showPath(byID, e.Leaf)
			}
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	on := make(map[*message]bool, len(th.Messages))
	for _, m := range th.Messages {
		on[m] = true
	}
	for _, m := range msgs {
		if !on[m] {
			th.Others = append(th.Others, m)
		}
	}
	// When the user last sent in it, which the index keeps; where it has
	// no entry, its last question's, else when it began.
	th.Updated = created
	for _, m := range msgs {
		if m.Role == "user" && m.Time.After(th.Updated) {
			th.Updated = m.Time
		}
	}
	return th
}

// recoverPartial adds to th the reply its partial at path kept, which a
// crash left streaming in: as a reply stopped there, on the path shown,
// which the next save appends to its log, deleting the partial. A partial
// whose reply the log holds, of a save the crash cut short, is deleted.
func recoverPartial(path string, th *thread) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	m := &message{}
	ev := messageEvent{message: m}
	if err := json.Unmarshal(bytes.TrimSpace(b), &ev); err != nil || m.ID <= 0 || th.logged[m.ID] {
		os.Remove(path)
		return
	}
	byID := map[int]*message{m.ID: m}
	for _, o := range th.all() {
		byID[o.ID] = o
	}
	if m.Parent != 0 && byID[m.Parent] == nil {
		os.Remove(path)
		return
	}
	m.Next, m.Time = 0, ev.At
	if m.Text == "" {
		m.Error = errStopped
	}
	all, leaf := append(th.all(), m), th.logLeaf
	th.showPath(byID, m.ID)
	th.Others = slices.DeleteFunc(all, func(o *message) bool { return slices.Contains(th.Messages, o) })
	// The log still ends where it did, without the reply.
	th.logLeaf, th.partial = leaf, true
}

// sweepAttachments deletes the files in files that no log or partial in
// chats names, nor chats.json beside it, and that were not changed since before: those of chats a
// crash cut the deleting of short. It reads the logs as they are on disk,
// not as they were read, so that a line skipped as broken still keeps its
// files.
func sweepAttachments(chats, files string, before time.Time) {
	entries, err := os.ReadDir(files)
	if err != nil {
		return
	}
	logs, err := os.ReadDir(chats)
	if err != nil {
		return
	}
	var named [][]byte
	// And chats.json, which a migration that failed leaves, or the old one
	// it kept.
	for _, n := range []string{oldChats, oldChats + ".migrated"} {
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(chats), n)); err == nil {
			named = append(named, b)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return
		}
	}
	for _, e := range logs {
		if n := e.Name(); strings.HasSuffix(n, ".jsonl") || strings.HasSuffix(n, ".partial") {
			b, err := os.ReadFile(filepath.Join(chats, n))
			if err != nil {
				// What it names is not known: delete nothing.
				return
			}
			named = append(named, b)
		}
	}
	all := bytes.Join(named, nil)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() || !info.ModTime().Before(before) {
			continue
		}
		if !bytes.Contains(all, []byte(`"`+e.Name()+`"`)) {
			if err := os.Remove(filepath.Join(files, e.Name())); err != nil {
				log.Printf("sweeping an attachment: %v", err)
			}
		}
	}
}

// showPath makes th's path the messages leaf follows, of byID, and leaf,
// marking it, as replaying a line of its log does.
func (th *thread) showPath(byID map[int]*message, leaf int) {
	var path []*message
	for id := leaf; id != 0 && len(path) <= len(byID); {
		m := byID[id]
		if m == nil {
			break
		}
		path = append(path, m)
		id = m.Parent
	}
	slices.Reverse(path)
	th.Messages = path
	th.markPath()
	th.logLeaf = leaf
}

// migrate moves the chats out of chats.json in dir into logs and an index,
// where it is there and they were not moved yet, and renames it. Each
// chat's messages go in an order that replays to the versions shown last:
// every message after the one it follows, and of the versions of one, the
// one shown last after the others, with all that followed each.
func migrate(dir string) error {
	cdir := filepath.Join(dir, chatsDir)
	if _, err := os.Stat(filepath.Join(cdir, indexFile)); err == nil {
		return nil
	}
	old := filepath.Join(dir, oldChats)
	b, err := os.ReadFile(old)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	var threads []*thread
	if err := json.Unmarshal(b, &threads); err != nil {
		return err
	}
	if err := os.MkdirAll(cdir, 0o700); err != nil {
		return err
	}
	idx := chatIndex{Version: logVersion, Chats: []indexEntry{}}
	for _, th := range threads {
		th.link()
		idx.Next = max(idx.Next, th.ID)
		for _, m := range th.all() {
			idx.Next = max(idx.Next, m.ID)
		}
		out := wholeLog(th)
		if err := replace(filepath.Join(cdir, strconv.Itoa(th.ID)+".jsonl"), out); err != nil {
			return err
		}
		idx.Chats = append(idx.Chats, indexEntry{ID: th.ID, Title: th.Title, Updated: th.Updated, Pinned: th.Pinned,
			Model: th.Model, Effort: th.Effort, Provider: th.Provider, Draft: th.Draft})
	}
	ib, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	if err := replace(filepath.Join(cdir, indexFile), ib); err != nil {
		return err
	}
	return os.Rename(old, old+".migrated")
}

// replayOrder returns th's messages in an order whose replay leaves each
// message's Next as it is: depth first from the first questions, the
// versions of one oldest first but the one it shows after it (Next) last.
// A message whose parent is not there begins a tree of its own.
func replayOrder(th *thread) []*message {
	all := th.all()
	byID := make(map[int]*message, len(all))
	for _, m := range all {
		byID[m.ID] = m
	}
	kids := map[int][]*message{}
	for _, m := range all {
		p := m.Parent
		if byID[p] == nil {
			p = 0
		}
		kids[p] = append(kids[p], m)
	}
	out := make([]*message, 0, len(all))
	var walk func(id, next int)
	walk = func(id, next int) {
		ks := kids[id]
		slices.SortFunc(ks, func(x, y *message) int { return x.ID - y.ID })
		if i := slices.IndexFunc(ks, func(k *message) bool { return k.ID == next }); i >= 0 {
			ks = append(slices.Delete(slices.Clone(ks), i, i+1), ks[i])
		}
		for _, k := range ks {
			out = append(out, k)
			walk(k.ID, k.Next)
		}
	}
	first := 0
	if len(th.Messages) > 0 {
		first = th.Messages[0].ID
	}
	walk(0, first)
	return out
}

// opKind is what a logOp does to its file.
type opKind int

const (
	opAppend  opKind = iota // appends data
	opReplace               // writes data in place of the file, at once
	opRemove                // deletes the file
)

// logOp is a change to a file of the chats: a log, of the chat id (0 for
// none), or a partial.
type logOp struct {
	id   int
	path string
	data []byte
	kind opKind
}

// logWriter changes the chats' files and writes their index on a
// goroutine, in the order asked: every change asked while it writes waits,
// and of the indexes waiting only the latest is written, after the logs.
// An append that fails leaves its log broken: the appends after it are
// skipped, until the log is written whole again, and failed lists it for
// the next save to.
type logWriter struct {
	mu      sync.Mutex
	ops     []logOp
	path    string // the index's
	index   []byte // the index to write next, nil when none waits
	writing bool
	failed  []logOp
	broken  map[string]bool // read and written by run alone
	done    sync.WaitGroup
}

// write asks for ops, then index written at path, nil for none.
func (w *logWriter) write(ops []logOp, path string, index []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ops = append(w.ops, ops...)
	if index != nil {
		w.path, w.index = path, index
	}
	if !w.writing {
		w.writing = true
		w.done.Add(1)
		go w.run()
	}
}

func (w *logWriter) run() {
	defer w.done.Done()
	for {
		w.mu.Lock()
		ops, path, index := w.ops, w.path, w.index
		w.ops, w.index = nil, nil
		if len(ops) == 0 && index == nil {
			w.writing = false
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()
		var failed []logOp
		for _, op := range ops {
			if err := w.apply(op); err != nil {
				log.Printf("saving the chats: %v", err)
				failed = append(failed, op)
			}
		}
		if index != nil {
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				log.Printf("saving the chats' index: %v", err)
			}
			// One that fails is written again by the next save, which
			// writes it whole each time.
			if err := replace(path, index); err != nil {
				log.Printf("saving the chats' index: %v", err)
			}
		}
		if len(failed) > 0 {
			w.mu.Lock()
			w.failed = append(w.failed, failed...)
			w.mu.Unlock()
		}
	}
}

// apply makes the change op asks. An append to a log broken before is
// skipped, as failing: its next line would go after a broken one.
func (w *logWriter) apply(op logOp) error {
	if w.broken == nil {
		w.broken = map[string]bool{}
	}
	switch op.kind {
	case opRemove:
		delete(w.broken, op.path)
		if err := os.Remove(op.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	case opReplace:
		err := os.MkdirAll(filepath.Dir(op.path), 0o700)
		if err == nil {
			err = replace(op.path, op.data)
		}
		if err == nil {
			delete(w.broken, op.path)
		}
		return err
	}
	if w.broken[op.path] {
		return fmt.Errorf("%s: skipped, after a failed write", filepath.Base(op.path))
	}
	err := os.MkdirAll(filepath.Dir(op.path), 0o700)
	if err == nil {
		err = appendTo(op.path, op.data)
	}
	if err != nil {
		w.broken[op.path] = true
	}
	return err
}

// appendTo appends b to the file at path, in one write.
func appendTo(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// takeFailed returns the changes that failed since it was last called.
func (w *logWriter) takeFailed() []logOp {
	w.mu.Lock()
	defer w.mu.Unlock()
	f := w.failed
	w.failed = nil
	return f
}

// wait returns once what was asked is written, as the app quits.
func (w *logWriter) wait() { w.done.Wait() }

// replace writes b to path at once, through a file beside it renamed over
// it, so that a crash leaves the old file whole.
func replace(path string, b []byte) error {
	tmp := path + ".tmp"
	err := os.WriteFile(tmp, b, 0o600)
	if err == nil {
		err = os.Rename(tmp, path)
	}
	return err
}

// saver writes files on a goroutine, one at a time: a save asked while one
// is written waits, and only the latest of those waiting is written.
type saver struct {
	mu      sync.Mutex
	path    string
	pending []byte // what to write next, nil when nothing waits
	writing bool
	done    sync.WaitGroup
}

func (s *saver) save(path string, b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path, s.pending = path, b
	if !s.writing {
		s.writing = true
		s.done.Add(1)
		go s.write()
	}
}

func (s *saver) write() {
	defer s.done.Done()
	for {
		s.mu.Lock()
		path, b := s.path, s.pending
		s.pending = nil
		if b == nil {
			s.writing = false
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		if err := replace(path, b); err != nil {
			log.Printf("saving %s: %v", filepath.Base(path), err)
		}
	}
}

// wait returns once what was saved is written, as the app quits.
func (s *saver) wait() { s.done.Wait() }
