package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// A chat is named at once of its first message (title), and then, as
// docs/titles.md has it, by a model, which a few words of what the chat is
// about name better than its first 48 runes: asked as the message is sent,
// beside the reply, of the provider the chat asks, on its cheap model.

// titlePrompt asks for a chat's title, the chat's first message after it,
// closed by titlePromptEnd.
const (
	titlePrompt = "Write a title for a chat that begins with the message below. " +
		"Reply with the title alone: at most six words, in the language of the message, " +
		"with no quotes and no trailing period. Do not answer the message.\n\n<message>\n"
	titlePromptEnd = "\n</message>"
)

// titleTimeout is how long a title may take: past it, one is no use, as the
// user has moved on.
const titleTimeout = 30 * time.Second

// titleMaxRunes is how long what a model says may be, at most, to be a
// title: a longer one is an answer to the message, not a name for it.
const titleMaxRunes = 80

// nameChat asks for a title of the chat th, which first was sent first, of
// the provider p, on the cheap model of its kind (titleModel), else model,
// the chat's, which it asks again where the cheap one fails. The title
// lands on the main thread (titled); one that fails leaves the chat as it is.
func (a *app) nameChat(th *thread, p provider, model, first string) {
	cheap := model
	if kind := a.kindOf(p.ID()); kind != nil && kind.titleModel != "" {
		cheap = kind.titleModel
	}
	id, local := th.ID, th.Title
	a.async(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), titleTimeout)
		defer cancel()
		name, err := askTitle(ctx, p, cheap, first)
		if err != nil && cheap != model && !errors.Is(err, context.DeadlineExceeded) {
			name, err = askTitle(ctx, p, model, first)
		}
		return func() {
			if err == nil {
				a.titled(id, local, name)
			}
		}
	})
}

// askTitle asks model of p for the title of a chat that begins with first,
// as normalizeTitle makes it; an error where it gave none.
func askTitle(ctx context.Context, p provider, model, first string) (string, error) {
	var b strings.Builder
	err := p.Stream(ctx, model, []turn{{Role: "user", Content: titlePrompt + first + titlePromptEnd}}, func(ch chunk) {
		b.WriteString(ch.Text) // what it reasoned is not the title
	})
	if err != nil {
		return "", err
	}
	name := normalizeTitle(b.String())
	if name == "" {
		return "", errors.New("no title")
	}
	return name, nil
}

// titled names the chat id name, where it is still there and still has its
// local title, local: not where it was deleted meanwhile, or named
// otherwise. The sidebar and the search show it at once, and the chats are
// saved, for it to stay.
func (a *app) titled(id int, local, name string) {
	th := a.thread(strconv.Itoa(id))
	if th == nil || th.Title != local || name == local {
		return
	}
	th.Title = name
	a.rev++
	a.save()
}

// normalizeTitle returns what a model said as a title: its first line,
// without the label, Markdown and quotes models put round it, nor a
// trailing period, cut as the local title is; "" where nothing is left,
// or it is too long to be a title.
func normalizeTitle(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	// Its Markdown and quotes, then its label, then those inside it:
	// **Title:** "…".
	s = trimDecoration(s)
	for _, label := range []string{"Title:", "title:", "标题：", "标题:", "タイトル：", "タイトル:", "제목:"} {
		s = strings.TrimPrefix(s, label)
	}
	// And a period after them, or in them: 「…」。, "….".
	for prev := ""; s != prev; {
		prev = s
		s = trimDecoration(s)
		s = strings.TrimRightFunc(s, func(r rune) bool { return r == '.' || r == '。' || unicode.IsSpace(r) })
	}
	if s == "" || len([]rune(s)) > titleMaxRunes {
		return ""
	}
	return title(s)
}

// trimDecoration trims the heading marks, emphasis and quotes round s.
func trimDecoration(s string) string {
	s = strings.TrimLeft(s, "# ")
	return strings.Trim(s, " *_`\"'“”‘’「」『』«»„")
}
