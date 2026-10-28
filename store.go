package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// load reads the saved chats from the app's user data directory.
func (a *app) load() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return
	}
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
}

// save writes the chats, where the app has a place for them (not in
// tests). It replaces the file at once, so that a crash leaves the old one.
func (a *app) save() {
	if a.path == "" {
		return
	}
	b, err := json.MarshalIndent(a.threads, "", "  ")
	if err != nil {
		return
	}
	tmp := a.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, a.path)
	}
}
