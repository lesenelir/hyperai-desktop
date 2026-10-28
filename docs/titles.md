# Chat titles

How each chat in the sidebar gets its name, and how a better one should
arrive. Adapted from Waku's `docs/titles.md`: Waku drives coding agents
that mostly name their own sessions, which HyperAI's providers do not, so
what carries over is the placeholder-then-replace design, not the
per-provider plumbing.

## Today: the local title

A chat is created by its first message ([`app.send`](../app.go)), and
[`title`](../app.go) names it from that message: whitespace collapsed to
single spaces, cut at **48 runes** with an ellipsis. The title is set once
and never changes. It is stored as `Title` in `chats.json` and shown in the
sidebar row, the title bar, and the heading of **Copy as Markdown**.

This is a placeholder, not a title: a question like "How do I make the
composer in my MyGo app send on Enter but…" says what was asked, not what
the chat is about.

## Next: generated titles

Not built yet. When it is, it should keep to these rules, which Waku
arrived at the hard way:

**The local title stays the first one.** It shows the instant the message
is sent, so the sidebar never holds an empty or "New chat" row. A generated
title replaces it silently when it lands.

**Latency is what matters.** A title that lands after the user has moved on
is, from their side, no title at all. Ask for it as soon as the first
message is sent, in parallel with the reply, not after the reply ends —
it needs only the first message.

**A cheap model, pinned.** Naming a chat is a small, fixed task: ask
`anthropic/claude-haiku-5.5` on OpenRouter whatever model the chat itself
uses, with no streaming, and a prompt asking for at most six words, no
quotes, no trailing period, in the language of the message.

**Two fields, so a user's name always wins.** When chats can be renamed,
split the title as Waku does:

| Field | Owner | Set by |
| --- | --- | --- |
| `Title` | the user | renaming; wins whenever set |
| `AutoTitle` | the app | the local title, then the generated one |

The sidebar shows `Title`, else `AutoTitle`. A generated title therefore
never overwrites a name the user typed, and a chat saved before the split
reads its old `Title` as the user's, which is safe.

**Normalize once, at the end.** Trim, take the first line, strip quotes
and a trailing period, cap at 48 runes as the local title is; an empty
result keeps the local title.

**Fail quietly.** Without a key (the demo), on an error, or when the chat
was deleted meanwhile, keep the local title and log nothing to the user.
The request runs on a goroutine and lands through `Window.Update`, like
the reply ([performance.md](performance.md)); it saves the chats when it
does, so the title survives a quit.
