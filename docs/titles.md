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

**The user decides, on the Default model page.** Its Chat titles card
turns the generated titles off (Generate titles, on at first), and
chooses the Title model: Automatic at first, which is the rule above,
then any model offered, which every new chat's first message is then sent
to, as the row says. Where that one fails, the chat's own model is asked,
which sends the message nowhere new; where its provider is removed,
Automatic is chosen again. Both are kept in `providers.json`
(`noTitles`, `titleModel`).

**A user's name always wins.** A title lands only on a chat whose title is
still the one it was asked to replace, so that a name the user typed while
it was asked for stays. That is why one field is enough, where Waku keeps
two (the user's and the app's): no title is ever asked for over a name the
user gave, but by Regenerate Title, which the user asks for.

**Renaming, in place.** Rename… in the chat's menu, of the title bar or of
its row in the sidebar, turns the title there into a field, at its size
and weight, on the composer's face in a hairline, the text where the
title's was, all of it chosen, the caret at its start so that a long one
shows from there, as the Finder's names do. Return keeps what it holds,
its spaces collapsed and cut as a title's; so does clicking away, or the
field going (its row scrolled away, the sidebar hidden, another chat
shown); Escape keeps the name, as does a field left empty. Escape ends the
renaming alone, not a reply streaming in. In the title bar, the field is
out of the room that drags the window.

**Regenerate Title**, in the same menus, asks for a title anew of the
chat's first question with text, of the model the chat asks and as a
chat that starts is named (the Title model first, then the provider's
cheapest), over whatever title it has. While it is asked for, a spinner
shows beside the title, in the title bar (14) and the sidebar's row (12),
where a chat named as it starts shows none: that title replaces the
local one quietly, under the reply's own spinner. It is faded while one is
being asked for, and for a chat of files alone; a failure, which the user
asked about, shows in a toast, as the provider said it.

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
