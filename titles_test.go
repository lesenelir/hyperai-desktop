package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// namer replies to questions, and names chats as a model does, round its
// title what models put there; as Anthropic, whose cheap model it fails
// with where cheapFails, and every title where titleFails.
type namer struct {
	title      string
	cheapFails bool
	titleFails bool
	titled     []string // the models asked for a title
}

func (p *namer) ID() string                                 { return "anthropic" }
func (p *namer) Name() string                               { return "Namer" }
func (p *namer) Models() []string                           { return []string{"claude-opus-5-5"} }
func (p *namer) List(ctx context.Context) ([]string, error) { return p.Models(), nil }

func (p *namer) Stream(ctx context.Context, model string, turns []turn, delta func(chunk)) error {
	if !strings.HasPrefix(turns[0].Content, titlePrompt) {
		delta(chunk{Text: "Sure."})
		return nil
	}
	p.titled = append(p.titled, model)
	if p.titleFails || p.cheapFails && model == "claude-haiku-5-5" {
		return errors.New("404 Not Found: no such model")
	}
	delta(chunk{Reasoning: "A short name."})
	delta(chunk{Text: p.title})
	return nil
}

// A chat started is named by the cheap model of its provider, in place of
// its first message's runes; by the chat's model where the cheap one
// fails; and keeps them where both do.
func TestNameChat(t *testing.T) {
	first := "How do I make the composer in my MyGo app send on Enter but break the line on Shift+Enter?"
	for _, c := range []struct {
		name   string
		p      *namer
		want   string
		models []string
	}{
		{"cheap", &namer{title: "**Title:** \"Composer Enter key\"\nmore"}, "Composer Enter key", []string{"claude-haiku-5-5"}},
		{"fallback", &namer{title: "回车发送消息。", cheapFails: true}, "回车发送消息", []string{"claude-haiku-5-5", "claude-opus-5-5"}},
		{"failed", &namer{titleFails: true}, title(first), []string{"claude-haiku-5-5", "claude-opus-5-5"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := newApp(c.p)
			a.nameChats = true
			a.send(first)
			th := a.current()
			if th.Title != c.want || strings.Join(c.p.titled, ",") != strings.Join(c.models, ",") {
				t.Errorf("title %q of %v, want %q of %v", th.Title, c.p.titled, c.want, c.models)
			}
			// Only as the chat starts.
			a.send("And on Cmd+Enter?")
			if len(c.p.titled) != len(c.models) {
				t.Errorf("asked again: %v", c.p.titled)
			}
		})
	}
}

// Files alone keep the first's name; a title lands nowhere on a chat
// deleted or named otherwise meanwhile.
func TestTitleLands(t *testing.T) {
	_, text, _, _ := testFiles(t)
	p := &namer{title: "Notes"}
	a := newApp(p)
	a.nameChats = true
	a.attach([]string{text})
	a.send("")
	if th := a.current(); th.Title != "notes.md" || len(p.titled) != 0 {
		t.Errorf("of files alone: title %q, asked %v", th.Title, p.titled)
	}

	th := &thread{ID: 7, Title: "How do I cache?"}
	a.threads = append(a.threads, th)
	rev := a.rev
	a.titled(7, "How do I cache?", "Caching")
	if th.Title != "Caching" || a.rev == rev {
		t.Errorf("title %q, rev %d after %d", th.Title, a.rev, rev)
	}
	a.titled(7, "How do I cache?", "Something else")
	if th.Title != "Caching" {
		t.Errorf("named over another title: %q", th.Title)
	}
	a.titled(8, "Gone", "Name")
}

// What a model says is made a title, or none where it answered the message.
func TestNormalizeTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Caching in Go":                     "Caching in Go",
		"  \"Caching in Go.\"  ":            "Caching in Go",
		"**Title:** Caching in Go":          "Caching in Go",
		"# 缓存设计\n\n解释：……":                   "缓存设计",
		"标题：「回车发送」。":                        "回车发送",
		"タイトル：『キャッシュ』":                      "キャッシュ",
		"":                                  "",
		"\"\"":                              "",
		strings.Repeat("A long answer ", 8): "",
		"Caching… and more":                 "Caching… and more",
	} {
		if got := normalizeTitle(in); got != want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

// A title comes after the reply began, not before: it runs beside it, on
// its own goroutine, and lands through post.
func TestTitleBesideReply(t *testing.T) {
	p := &namer{title: "Caching"}
	a := newApp(p)
	a.nameChats = true
	posts := make(chan func(), 16)
	a.post = func(f func()) { posts <- f }
	a.send("How do I cache?")
	th := a.current()
	if th.Title != "How do I cache?" || th.busy == nil {
		t.Fatalf("title %q, replying %v as it is sent", th.Title, th.busy != nil)
	}
	deadline := time.After(5 * time.Second)
	for th.Title != "Caching" || th.busy != nil {
		select {
		case f := <-posts:
			f()
		case <-deadline:
			t.Fatalf("title %q, replying %v", th.Title, th.busy != nil)
		}
	}
}
