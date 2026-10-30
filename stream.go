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
// batch schedules its commit, and the chunks until then join it, their
// text and their reasoning each. Commits are made in order, one at a time.
type batcher struct {
	mu        sync.Mutex
	commit    func(ch chunk) // publishes ch, as through Window.Update
	text      strings.Builder
	reasoning strings.Builder
	used      *usage      // the reply's usage, the latest given
	timer     *time.Timer // the batch's commit to come, nil when none
	last      time.Time   // when the last commit was made
	closed    bool
}

func newBatcher(commit func(ch chunk)) *batcher { return &batcher{commit: commit} }

// add adds a chunk to the batch, scheduling its commit when it starts one.
func (b *batcher) add(ch chunk) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || ch == (chunk{}) {
		return
	}
	b.text.WriteString(ch.Text)
	b.reasoning.WriteString(ch.Reasoning)
	if ch.Usage != nil {
		b.used = ch.Usage
	}
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
	if b.closed || b.text.Len()+b.reasoning.Len() == 0 && b.used == nil {
		return
	}
	ch := chunk{Text: b.text.String(), Reasoning: b.reasoning.String(), Usage: b.used}
	b.text.Reset()
	b.reasoning.Reset()
	b.used = nil
	b.last = time.Now()
	// Committed under the lock, so that a commit of the timer's and the
	// last one, of close, are made in the order the chunks came.
	b.commit(ch)
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

// thinkTags takes the reasoning out of the text of a reply that opens with
// <think>, as models served without a reasoning parser write it (LM
// Studio, llama.cpp): what the tags hold is the reasoning, and what
// follows them the text. A reply that does not open so is text whole, so
// that the tags elsewhere, as in code about them, stay the text's. It
// holds back what may be part of a tag, split between chunks, until the
// next tells.
type thinkTags struct {
	state thinkState
	held  string // what may be part of a tag
	lead  bool   // the text after the tags, whose leading space goes
}

type thinkState int

const (
	thinkStart thinkState = iota // before the reply's first word
	thinkIn                      // in the tags
	thinkOut                     // in the text
)

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// feed returns what s, as the reply's text goes on with it, adds to the
// reasoning and the text.
func (t *thinkTags) feed(s string) chunk {
	t.held += s
	var out chunk
	for {
		switch t.state {
		case thinkStart:
			rest := strings.TrimLeft(t.held, " \t\r\n")
			switch {
			case strings.HasPrefix(rest, thinkOpen):
				t.held, t.state = rest[len(thinkOpen):], thinkIn
			case strings.HasPrefix(thinkOpen, rest):
				// Space, or the tag's start: the next chunk tells.
				return out
			default:
				t.state = thinkOut
			}
		case thinkIn:
			if i := strings.Index(t.held, thinkClose); i >= 0 {
				out.Reasoning += t.held[:i]
				t.held, t.state, t.lead = t.held[i+len(thinkClose):], thinkOut, true
				continue
			}
			n := len(t.held) - partialSuffix(t.held, thinkClose)
			out.Reasoning += t.held[:n]
			t.held = t.held[n:]
			return out
		default:
			if t.lead {
				if t.held = strings.TrimLeft(t.held, " \t\r\n"); t.held == "" {
					return out
				}
				t.lead = false
			}
			out.Text += t.held
			t.held = ""
			return out
		}
	}
}

// flush returns what feed held back, as the reply ends: reasoning, of
// tags never closed, or text.
func (t *thinkTags) flush() chunk {
	held := t.held
	t.held = ""
	switch {
	case t.state == thinkIn:
		return chunk{Reasoning: held}
	case t.lead:
		return chunk{}
	}
	return chunk{Text: held}
}

// partialSuffix returns the length of the longest end of s that begins
// tag, short of the whole tag.
func partialSuffix(s, tag string) int {
	for n := min(len(s), len(tag)-1); n > 0; n-- {
		if strings.HasSuffix(s, tag[:n]) {
			return n
		}
	}
	return 0
}
