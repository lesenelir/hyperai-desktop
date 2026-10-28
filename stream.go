package main

import (
	"strings"
	"sync"
	"time"
)

// commitInterval is how often a streaming reply commits what came at most:
// about 20 times a second, where a provider sends tens of chunks. The
// reveal of markdown.Stream paces the words out of what came, so the
// batching does not show (docs/performance.md).
const commitInterval = 50 * time.Millisecond

// batcher gathers a reply's chunks as they come on the stream's goroutine,
// and commits them at most every commitInterval: the first chunk of a
// batch schedules its commit, and the chunks until then join it. Commits
// are made in order, one at a time.
type batcher struct {
	mu     sync.Mutex
	commit func(s string) // publishes s, as through Window.Update
	buf    strings.Builder
	timer  *time.Timer // the batch's commit to come, nil when none
	last   time.Time   // when the last commit was made
	closed bool
}

func newBatcher(commit func(s string)) *batcher { return &batcher{commit: commit} }

// add adds a chunk to the batch, scheduling its commit when it starts one.
func (b *batcher) add(s string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || s == "" {
		return
	}
	b.buf.WriteString(s)
	if b.timer == nil {
		b.timer = time.AfterFunc(max(0, commitInterval-time.Since(b.last)), b.flush)
	}
}

// flush commits the batch.
func (b *batcher) flush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flushLocked()
}

func (b *batcher) flushLocked() {
	b.timer = nil
	if b.closed || b.buf.Len() == 0 {
		return
	}
	s := b.buf.String()
	b.buf.Reset()
	b.last = time.Now()
	// Committed under the lock, so that a commit of the timer's and the
	// last one, of close, are made in the order the chunks came.
	b.commit(s)
}

// close commits what is left at once, as the stream ends, and drops what
// comes after.
func (b *batcher) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.timer != nil {
		b.timer.Stop()
	}
	b.flushLocked()
	b.closed = true
}
