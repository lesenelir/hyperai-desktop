# Chat storage

How HyperAI keeps its chats on disk, and how it moves from one
`chats.json` to a log per chat. This is a plan: nothing below is built
yet. The comparison it rests on looked at ChatWise and Alma, which use
SQLite, and at Codex, DeepSeek Harness and Claude Code, which keep an
append-only JSONL file per session.

## Today

[store.go](../store.go) keeps every chat in one file, `chats.json`, in the
user data directory. Every chat and every message is a single JSON array,
indented, the `thread` and `message` structs of [app.go](../app.go) as
their `json` tags write them. Files sent with a question are copied into
`attachments/`, named with a random hex name (`keepFiles`,
[attach.go](../attach.go)), and the message keeps only that name.

Each `a.save()` encodes **all** chats on the main thread and writes them
to `chats.json.tmp`, then renames that over `chats.json`, on a goroutine
(`saver`). That is safe, and fine at 47 chats and 226 KB, but:

- **Every change rewrites everything.** A new message, a pin, a rename, a
  draft kept as the user leaves a chat, a version shown: each one encodes
  and writes every chat. The cost grows with the history, not with the
  change.
- **A question is not kept until its reply ends.** `send` adds the question
  and `ask` streams the reply, but the first `save` comes when the reply
  ends (`finish`), when the title arrives, or when the reply is stopped. If
  the app quits in the middle, the question and the reply are both lost.
- **It has no room for an agent.** One reply in Agent mode can be dozens of
  tool calls with large outputs. Rewriting the whole history after each one
  costs too much, and a crash loses the whole turn.

## The decision

**One append-only JSONL log per chat, plus a small index of the chats.**
The log is the source of truth. The index is what the sidebar needs, and
can be rebuilt from the logs. SQLite is not used now. If it ever comes, it
is a derived index, as in Codex: logs stay the source, and the database
can be deleted and rebuilt from them.

Why not SQLite:

- **MyGo builds with `CGO_ENABLED=0`.** `mattn/go-sqlite3` is out, so
  SQLite would mean `modernc.org/sqlite` (pure Go, several MB of binary) or
  `ncruces/go-sqlite3` (on wazero, which the app already uses). Both work,
  but cost more than encoding/json and os.
- **The direction is an agent.** Codex, Claude Code and DeepSeek Harness
  all keep their sessions as an append-only event log: one line per step,
  safe across a crash, and replayable to resume a session. ChatWise and Alma
  started as chat apps and added agents on top of SQLite later.
- **The scale fits in memory.** One user's chats, even 1,500 of them and
  50 MB of text as in ChatWise, load and search fast in memory, which is
  how [search.go](../search.go) works today.
- **The change is small.** The in-memory model (`thread`, `message`, the
  version tree of [versions.go](../versions.go)) stays as it is. Only how
  it is saved and loaded changes.

SQLite becomes worth it when one of these is true: the app needs queries
across chats that memory can't serve fast (folders, tags, filters,
statistics, full-text search over tens of thousands of chats); chats sync
between devices; or more than one process writes the same data.

## Layout

```
<user data>/
  chats/
    index.json          the chats, for the sidebar
    <id>.jsonl          one chat's log, by its ID
  attachments/          unchanged
  chats.json.migrated   the old file, kept after migrating
```

`<user data>` is `~/Library/Application Support/hyperai-desktop/` on macOS
(`hyperai-desktop Dev/` for `mygo dev`), as today. Every file is written
with mode 0600, as `chats.json` is.

## The index

`chats/index.json` holds what changes often and what the sidebar shows,
without reading any log:

```json
{
  "version": 1,
  "next": 412,
  "chats": [
    {
      "id": 12,
      "title": "…",
      "created": "2026-11-01T22:40:00Z",
      "updated": "2026-11-01T22:44:00Z",
      "pinned": "2026-11-01T22:45:00Z",
      "model": "…",
      "provider": "anthropic",
      "effort": "high",
      "draft": "…"
    }
  ]
}
```

- **The fields are the `thread` fields that are not messages:** `Title`,
  `Updated`, `Pinned`, `Model`, `Provider`, `Effort`, `Draft`, plus
  `created`. Their `json` names stay the same, and empty ones are left out
  as today.
- **`next` is the ID counter.** Chats and messages share one counter
  (`a.next`), which `loadFrom` finds today by reading every message. The
  index keeps it, so the counter is known before any log is read. On load,
  `next` is the larger of the index's value and the IDs seen in the logs.
- **It is written whole**, encoded on the main thread and written by the
  `saver` as `chats.json` is now (a `.tmp` file, then a rename). It is
  small, a few hundred bytes per chat, so a draft, a pin or a rename costs
  that much and no more.
- **The draft is kept only here**, not in the log. It changes too often
  for the log, and losing it in a rebuild is acceptable.

## The log

`chats/<id>.jsonl` is one JSON object per line, in the order things
happened, and is only ever appended to. Every line has a type `t` and a
time `at` (RFC 3339, UTC):

```jsonl
{"t":"chat","at":"…","v":1,"id":12}
{"t":"message","at":"…","id":13,"role":"user","text":"…","files":[{"name":"a.pdf","type":"application/pdf","file":"15f99a47b549ec16.pdf"}],"time":"…"}
{"t":"title","at":"…","title":"…"}
{"t":"message","at":"…","id":14,"parent":13,"role":"assistant","text":"…","reasoning":"…","reasoned":3.2,"usage":{"input":1200,"output":800},"firstToken":0.8,"elapsed":12.5,"time":"…"}
{"t":"message","at":"…","id":15,"parent":12,"role":"user","text":"…","time":"…"}
{"t":"show","at":"…","leaf":14}
```

| Type | Written when | Fields |
| --- | --- | --- |
| `chat` | The chat is created: the first line, and only the first | `v`, the log format version; `id`, the chat's ID |
| `message` | A question is sent, edited or sent again (`send`, `resend`), as soon as it is created. A reply has ended, failed or been stopped (`finish`, `stop`) | The `message` struct as its `json` tags write it: `id`, `parent`, `role`, `text`, `files`, `error`, `elapsed`, `time`, `reasoning`, `reasoned`, `usage`, `firstToken` |
| `title` | The chat is renamed by the user or named by a model (`renameEnd`, `titled`) | `title` |
| `show` | The user shows another version (`showVersion`) | `leaf`, the last message of the path now shown |

### Replaying a log into a chat

The messages form the same tree as today: `parent` links each message to
the one it follows, and `next` remembers which child was shown last. The
tree and the path shown are rebuilt like this:

- **A `message` event adds the message, and the path becomes its
  ancestors and itself.** This matches every way messages are added today.
  `send` appends after the last message of the path. `ask` appends the
  reply under the question. `resend` and `regenerate` cut the path at the
  edited message and add the new version under its parent. In each case
  the new path is the new message's chain of parents.
- **A `show` event makes the path the ancestors of `leaf`, and `leaf`
  itself.** `showVersion` goes down from the version chosen by following
  `next`, and the log records where that ended. `leaf` alone is enough,
  because a message's ancestors are fixed.
- **After each event, `markPath` runs on the path.** Then every message's
  `next` ends up the same as it was in memory, including messages off the
  path, which keep the `next` they had when they were last shown.
- **`Messages` is the final path, and `Others` is every other message,**
  as `cut` and `showVersion` leave them.

A test proves this: driving a chat through sends, edits, regenerations and
version switches, then replaying its log, must give the same `Messages`,
`Others` and `next` values as the chat in memory.

### Rules

- **A reply is logged once, when it ends.** The text that streams in is not
  logged: that would be dozens of lines a second, and replaying a partial
  reply is not useful in chat. If the app quits during a reply, the
  question is kept and the reply is lost. Today both are lost.
- **Unknown types are skipped.** A newer build may add types; an older
  build reading the log ignores them. A change to the meaning of an
  existing type bumps `v` in the `chat` line.
- **Agent mode adds types without changing these.** Tool calls, tool
  results and compaction become new event types, defined with Agent mode.
  Each step is appended as it completes, so a crash loses at most the
  current step.

## Writing

- **Appends are queued.** They go through an ordered queue per chat on one
  writer goroutine, like the `saver`, so the main thread never waits for
  the disk and the lines of one chat stay in order. The event is encoded
  on the main thread, where the message is safe to read.
- **One line, one write.** Each event is encoded to a single line ending in
  `\n` and written with one `Write` to a file opened with `O_APPEND`.
- **The log is written before the index.** When a change touches both (a
  new message also updates `updated`), the log line is queued first. If
  the app crashes between the two, the load below repairs the index from
  the log.
- **On quit, the app waits for the queue**, as it waits for the `saver` and
  for attachments (`writes`) today. Nothing calls fsync on each line; a
  crash of the OS can lose the last lines, as it can lose `chats.json`
  today.

What the current `a.save()` calls become:

| Today | Then |
| --- | --- |
| `send`, `resend` (no save today) | append the question's `message`; save the index |
| `finish`, `stop` | append the reply's `message`; save the index |
| `showVersion` | append `show` |
| `renameEnd`, `titled` | append `title`; save the index |
| `togglePin`, `keepDraft` | save the index |
| `remove`, `removeAll` | save the index, then delete the logs (and the attachments, as now) |

## Loading

1. **Read `index.json`.** The sidebar can show from it alone.
2. **Read every log**, as `loadFrom` reads every chat today, since the
   search, the outline and Copy as Markdown need the messages. Later, if
   measurement shows launch is slow (see
   [performance.md](performance.md)), logs can be read lazily, and the
   search can read the logs it has not loaded yet.
3. **Skip a broken last line.** A crash can leave a partial last line.
   Reading stops at the last complete line, and the file is truncated to
   that point before anything is appended to it. Otherwise the next line
   would be glued to the broken one. A broken line anywhere else is
   skipped and logged.
4. **Repair the index against the logs.**
   - A log with no entry in the index gets one: `created` from the `chat`
     line, `updated` from the last question's `time`, the title from the
     last `title` event, or else `title()` of the first question, as
     `send` makes it.
   - An entry whose log is missing is dropped.
   - If `index.json` is missing or cannot be read, it is rebuilt from the
     logs this way. Pins and drafts are lost in that case.

## Migrating from chats.json

At launch, if `chats/index.json` does not exist and `chats.json` does:

1. Read `chats.json` as `loadFrom` does now, including `link()` for chats
   kept before versions existed.
2. For each chat, write `chats/<id>.jsonl`: the `chat` line, then every
   message from `all()` **in ID order**, written as it is with its `next`
   included, then one `title` line, then a `show` line whose `leaf` is the
   last message of `Messages`. Parents always have smaller IDs than their
   children, because both come from the same increasing counter, so ID
   order adds every message after its parent. Writing `next` as it was
   keeps the version each message showed last.
3. Write `index.json`, with `next` set to the counter.
4. Rename `chats.json` to `chats.json.migrated`. It is kept, not deleted.
   Nothing reads it again, and the user can delete it.

Each log and the index are first written to `.tmp` files, then renamed.
The migration counts as done only once `index.json` exists. If it is
interrupted, it runs again at the next launch and overwrites what it
wrote. A test checks that a `chats.json` read the old way and the same
chats after migrating are the same.

## Tests

- Round trip: chats changed in every way listed above, saved, then
  loaded, equal the chats in memory.
- Version replay, as described in "Replaying a log into a chat".
- A log with a broken last line loads, and the next append starts on a
  new line.
- The index is rebuilt and repaired: missing, corrupt, a log with no entry,
  an entry with no log.
- Migration from `chats.json`, including chats from before versions and
  replies that failed or were stopped.
- Concurrency (`go test -race`): two chats streaming and finishing at once
  append to their own logs in order, as `TestChatsStreamAtOnce` does.

## Later

- **A derived SQLite index**, with `ncruces/go-sqlite3` (wazero) or
  `modernc.org/sqlite`, and FTS5, for full-text search once memory is not
  enough. It is rebuilt from the logs, as Codex's `thread_history` is
  projected from its rollouts.
- **Compressing old logs** with zstd, as Codex does for sessions it no
  longer writes.
- **Rewriting a log** to drop what was replaced, if logs ever grow large
  from edits. No current feature needs it: chats are deleted whole and
  messages never are.
