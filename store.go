package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"
)

// load reads the saved chats from the app's user data directory.
func (a *app) load() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return
	}
	a.loadFrom(dir)
}

// loadFrom reads the layout and the chats kept in dir, and shows the chat
// shown last, where General says to and it is still there.
func (a *app) loadFrom(dir string) {
	a.loadLayout(dir)
	a.loadConf(dir)
	defer func() {
		if a.onLaunch == launchLast && a.thread(a.keptChat) != nil {
			a.selected = a.keptChat
		}
	}()
	a.path = filepath.Join(dir, "chats.json")
	b, err := os.ReadFile(a.path)
	if err != nil || json.Unmarshal(b, &a.threads) != nil {
		return
	}
	for _, th := range a.threads {
		a.next = max(a.next, th.ID)
		for _, m := range th.Messages {
			a.next = max(a.next, m.ID)
		}
	}
	a.rev++
}

// save writes the chats, where the app has a place for them (not in
// tests). It encodes them here, on the main thread, where they are safe
// to read, and leaves writing them to a goroutine, so that a frame never
// waits for the disk.
func (a *app) save() {
	if a.path == "" {
		return
	}
	b, err := json.MarshalIndent(a.threads, "", "  ")
	if err != nil {
		return
	}
	a.saver.save(a.path, b)
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
		// Replaced at once, so that a crash leaves the old file.
		tmp := path + ".tmp"
		err := os.WriteFile(tmp, b, 0o600)
		if err == nil {
			err = os.Rename(tmp, path)
		}
		if err != nil {
			log.Printf("saving the chats: %v", err)
		}
	}
}

// wait returns once what was saved is written, as the app quits.
func (s *saver) wait() { s.done.Wait() }
