# Chat titles

How each chat in the sidebar gets its name, and how a better one should
arrive. Adapted from Waku's `docs/titles.md`: Waku drives coding agents
that mostly name their own sessions, which HyperAI's providers do not, so
what carries over is the placeholder-then-replace design, not the
per-provider plumbing.

## The local title

A chat is created by its first message ([`app.send`](../app.go)), and
[`title`](../app.go) names it from that message: whitespace collapsed to
single spaces, cut at **48 runes** with an ellipsis; a chat of files alone
is named by the first. It is stored as `Title` in `chats.json` and shown in
the sidebar row, the title bar, the search, and the heading of **Copy as
Markdown**.

This is a placeholder, not a title: a question like "How do I make the
composer in my MyGo app send on Enter but…" says what was asked, not what
the chat is about.

## Generated titles

[titles.go](../titles.go) has a model replace it, keeping to these rules,
which Waku arrived at the hard way:

**The local title stays the first one.** It shows the instant the message
is sent, so the sidebar never holds an empty or "New chat" row. A generated
title replaces it silently when it lands.

**Latency is what matters.** A title that lands after the user has moved on
is, from their side, no title at all. Ask for it as soon as the first
message is sent, in parallel with the reply, not after the reply ends —
it needs only the first message.

**The chat's own provider, on its cheap model.** Naming a chat is a small,
fixed task, for the cheapest model that does it (`titleModel` of
[providerKinds](../provider.go)): Claude Haiku 5.5 for Anthropic and on
OpenRouter, GPT-5 mini for OpenAI, Gemini 3.5 Flash-Lite for Google, DeepSeek
Chat for DeepSeek; the chat's own model for the servers on the user's
computer, Copilot and the providers the user added, and where the cheap one
fails. Never another provider than the chat's: a chat on Ollama is on the
user's computer to stay there, and its first message is not to go to a
service it never chose. The prompt asks for at most six words, no quotes,
no trailing period, in the language of the message, and not to answer it;
the message goes in `<message>` tags. What the model reasoned is dropped.

**A user's name always wins.** A title lands only on a chat whose title is
still the local one it replaces, so that it never overwrites another. When
chats can be renamed, split the title as Waku does:

| Field | Owner | Set by |
| --- | --- | --- |
| `Title` | the user | renaming; wins whenever set |
| `AutoTitle` | the app | the local title, then the generated one |

The sidebar shows `Title`, else `AutoTitle`. A generated title therefore
never overwrites a name the user typed, and a chat saved before the split
reads its old `Title` as the user's, which is safe.

**Normalize once, at the end.** Trim, take the first line, strip the
Markdown, labels ("Title:", "标题：") and quotes models put round it and a
trailing period, cap at 48 runes as the local title is; an empty result,
or one over 80 runes, which answered the message rather than naming it,
keeps the local title.

**Fail quietly.** On an error, after 30 seconds, or when the chat was
deleted meanwhile, keep the local title and say nothing to the user; the
demo, in tests and `HYPERAI_BENCH`, is never asked (`app.nameChats`). The
request runs on a goroutine and lands through `Window.Update`, like the
reply ([performance.md](performance.md)); it saves the chats when it does,
so the title survives a quit.
