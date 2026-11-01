package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/google/uuid"
)

// ID identifies a chat or a message, unique wherever it was made, so that
// the chats of two computers can come together (docs/storage.md): a
// UUIDv7, as text. Its first 48 bits are the time it was made, in
// milliseconds, and the IDs one process makes only grow, so that the order
// of IDs is the order things were made, which the versions of a message
// are told in (versions.go). "" is none: the parent of a chat's first
// message.
type ID string

// newID returns a new ID, past every one this process made.
func newID() ID { return ID(uuid.Must(uuid.NewV7()).String()) }

// UnmarshalJSON reads an ID as text, or as a number, as the chats kept
// before IDs were UUIDs have them: as its digits, which legacy tells, and
// upgradeIDs replaces. 0 is none.
func (id *ID) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && (b[0] == '-' || b[0] >= '0' && b[0] <= '9') {
		var n int64
		if err := json.Unmarshal(b, &n); err != nil {
			return err
		}
		*id = ""
		if n != 0 {
			*id = ID(strconv.FormatInt(n, 10))
		}
		return nil
	}
	if bytes.Equal(b, []byte("null")) {
		*id = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*id = ID(s)
	return nil
}

// legacy reports whether id is a number, of the chats kept before IDs were
// UUIDs.
func (id ID) legacy() bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// upgradeIDs gives the chats and messages of threads whose IDs are
// numbers new IDs, in the order of the numbers, which was the order they
// were made, and the messages that follow and show them the new IDs too.
// It returns the new ID of each old one, nil where none was a number.
func upgradeIDs(threads []*thread) map[ID]ID {
	var old []ID
	seen := map[ID]bool{}
	note := func(id ID) {
		if id.legacy() && !seen[id] {
			seen[id] = true
			old = append(old, id)
		}
	}
	for _, th := range threads {
		note(th.ID)
		for _, m := range th.all() {
			note(m.ID)
			note(m.Parent)
			note(m.Next)
		}
	}
	if len(old) == 0 {
		return nil
	}
	num := func(id ID) int64 { n, _ := strconv.ParseInt(string(id), 10, 64); return n }
	slices.SortFunc(old, func(x, y ID) int { return cmp.Compare(num(x), num(y)) })
	to := make(map[ID]ID, len(old))
	for _, id := range old {
		to[id] = newID()
	}
	swap := func(id *ID) {
		if n, ok := to[*id]; ok {
			*id = n
		}
	}
	for _, th := range threads {
		swap(&th.ID)
		for _, m := range th.all() {
			swap(&m.ID)
			swap(&m.Parent)
			swap(&m.Next)
		}
	}
	return to
}
