# Chat storage

How HyperAI keeps its chats on disk, and how it moves from one
`chats.json` to a log per chat ([store.go](../store.go)). The comparison
it rests on looked at ChatWise and Alma, which use SQLite, and at Codex,
DeepSeek Harness and Claude Code, which keep an append-only JSONL file per
session.

## Before

The app used to keep every chat in one file, `chats.json`, in the user
data directory. Every chat and every message is a single JSON array,
indented, the `thread` and `message` structs of [app.go](../app.go) as
their `json` tags write them. Files sent with a question are copied into
`attachments/`, named with a random hex name (`keepFiles`,
[attach.go](../attach.go)), and the message keeps only that name.

Each `a.save()` encoded **all** chats on the main thread and writes them
to `chats.json.tmp`, then renamed that over `chats.json`, on a goroutine
(`saver`). That was safe, and fine at 47 chats and 226 KB, but:

- **Every change rewrites everything.** A new message, a pin, a rename, a
  draft kept as the user leaves a chat, a version shown: each one encodes
  and writes every chat. The cost grows with the history, not with the
  change.
- **A question was not kept until its reply ended.** `send` added the
  question and `ask` streamed the reply, but the first `save` came when the
  reply ended (`finish`), when the title arrived, or when the reply was
  stopped. If the app quit in the middle, the question and the reply were
  both lost.
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
    <id>.partial        the reply streaming in, while it does
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
  `Updated`, `Pinned`, `Model`, `Provider`, `Effort` and `Draft`
  (`indexEntry`). Their `json` names are the ones `thread` had, and empty
  ones are left out.
- **`next` is the ID counter.** Chats and messages share one counter
  (`a.next`), which `loadFrom` finds today by reading every message. The
  index keeps it, so the counter is known before any log is read. On load,
  `next` is the larger of the index's value and the IDs seen in the logs.
- **It is written whole**, encoded on the main thread and written on a
  goroutine through a `.tmp` file and a rename, as `chats.json` was. It is
  small, a few hundred bytes per chat, so a draft, a pin or a rename costs
  that much and no more.
- **The draft is kept only here**, not in the log. It changes too often
  for the log, and losing it in a rebuild is acceptable.

## The log

`chats/<id>.jsonl` is one JSON object per line, in the order things
happened, and is only ever appended to. Every line has a type `t` and,
except in logs moved from `chats.json`, a time `at` (RFC 3339, UTC). A
message with no `parent` is a version of the first question:

```jsonl
{"t":"chat","at":"…","v":1,"id":12}
{"t":"message","at":"…","id":13,"role":"user","text":"…","files":[{"name":"a.pdf","type":"application/pdf","file":"15f99a47b549ec16.pdf"}],"time":"…"}
{"t":"title","at":"…","title":"…"}
{"t":"message","at":"…","id":14,"parent":13,"role":"assistant","text":"…","reasoning":"…","reasoned":3.2,"usage":{"input":1200,"output":800},"firstToken":0.8,"elapsed":12.5,"time":"…"}
{"t":"message","at":"…","id":15,"role":"user","text":"… edited","time":"…"}
{"t":"show","at":"…","leaf":14}
```

| Type | Written when | Fields |
| --- | --- | --- |
| `chat` | The chat is created: the first line, and only the first | `v`, the log format version; `id`, the chat's ID |
| `message` | A question is asked (`ask`, as `send`, `resend` and `regenerate` call it), as the reply starts. A reply has ended, failed or been stopped (`finish`, `stop`) | The `message` struct as its `json` tags write it, but `next`, which the replay makes: `id`, `parent`, `role`, `text`, `files`, `error`, `elapsed`, `time`, `reasoning`, `reasoned`, `usage`, `firstToken` |
| `title` | The chat is renamed by the user or named by a model (`renameEnd`, `titled`) | `title` |
| `show` | The path shown is not the one replaying the log would show: the user showed another version (`showVersion`), or a reply was asked for again | `leaf`, the last message of the path now shown |

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
  reply is not useful in chat. What came of it is kept aside instead, in
  the chat's partial (below), so a crash during a reply loses at most the
  last second of it. Before, the question and the reply were both lost.
- **Unknown types are skipped.** A newer build may add types; an older
  build reading the log ignores them. A change to the meaning of an
  existing type bumps `v` in the `chat` line.
- **Agent mode adds types without changing these.** Tool calls, tool
  results and compaction become new event types, defined with Agent mode.
  Each step is appended as it completes, so a crash loses at most the
  current step.

## Writing

**`a.save()` keeps what changed**, and the call sites stayed as they were:
`ask`, `finish`, `stop`, `showVersion`, `renameEnd`, `titled`,
`togglePin`, `keepDraft`, `remove` and `removeAll` call it. Each chat
remembers what its log holds (`logged`, `logLeaf`, `logTitle`), and
`logChanges` returns only the lines it lacks:

- a `chat` line, where the chat has no log yet;
- a `message` line for each message that is not in the log and not
  streaming, in ID order;
- a `show` line, where the last message of the path shown, not counting a
  reply streaming in, is not where replaying the log would end;
- a `title` line, where the title changed.

A chat with nothing new costs a few comparisons. A chat that is gone has
its log deleted. The index is encoded every time.

- **Writing happens on a goroutine, in order** (`logWriter`). The lines are
  encoded on the main thread, where the messages are safe to read. The
  writer applies every change in the order asked, then writes the latest
  index, so the main thread never waits for the disk.
- **One write per save.** A chat's new lines are joined and written with
  one `Write` to its log, opened with `O_APPEND`.
- **The logs come before the index.** A new message is appended before the
  index that dates it is written, and a deleted chat's log is removed
  before the index that drops it. If the app crashes between the two, the
  repair below fixes the index from the logs, and never brings back a
  chat that was deleted.
- **On quit, the app waits for the writer**, as it waits for the layout,
  the settings and the attachments, then saves once more, for what failed,
  and waits again. Nothing calls fsync on each line; a crash of the OS can
  lose the last lines, as it could lose `chats.json`.

### A write that fails

A chat counts its lines as logged as soon as they are queued, so a write
that fails (a full disk, a folder that takes no files) would otherwise
never be written again. Waku's store, which notes rows as written only
once their transaction commits, is where this rule comes from.

- **The writer lists what failed** (`takeFailed`), and the next save
  starts with it (`retryFailed`).
- **A log an append to failed is written whole again** (`rewrite`,
  `wholeLog`), through a `.tmp` file and a rename, from the chat in
  memory: the same lines a migration writes. A failed append may have
  left part of a line, which a rewrite replaces, where another append
  would go after it.
- **Until then, the appends to that log are skipped** (`broken`), and
  count as failed too, so no line goes after a broken one.
- **A delete that failed is asked again:** the chat counts as on disk
  again, so the next save, not finding it, deletes its log.
- **The index needs nothing:** every save writes it whole.

### The reply streaming in

While a reply streams, `keepPartial` writes it, as a `message` line, to
`chats/<id>.partial`, at most once a second (`partialEvery`), in place of
the last (through a `.tmp` file and a rename). Waku saves a streaming
reply each second for the same reason, as a row it updates in place. A log
that is only appended to cannot be updated, so the reply is kept in a
file of its own, which is deleted once the reply is in the log: the save
that appends it, as it ends or is stopped, also deletes the partial, after
the line, in the writer's order.

At launch, a partial whose reply the log lacks is a reply a crash cut
short (`recoverPartial`). It is added to its chat, on the path shown,
like a reply stopped there: its text as it came, `Stopped.` where none
came, and the time it was last kept. The launch then saves, which appends
it to the log and deletes the partial. A partial whose reply the log
already holds, or whose question it lacks, is deleted.

### Attachments no log names

A chat deleted takes its attachments with it (`forgetFiles`), but a crash
between the two can leave them. After loading, a goroutine sweeps
`attachments/` (`sweepAttachments`): it deletes each file that no log,
partial, `chats.json` or `chats.json.migrated` names, and that was not
changed in the last hour (`sweepGrace`). That hour covers a file just
copied for a question whose line is not written yet.

- **It reads the files on disk, as text,** not the chats as they were
  read, as Waku's sweep scans its rows: a line skipped as broken still
  keeps its files.
- **Where any of them cannot be read, it deletes nothing.**
- `chats.json` counts for a migration that failed, whose chats are not in
  logs yet, and `chats.json.migrated` for going back to a build before
  logs.

## Loading

1. **Read `index.json`.** The sidebar can show from it alone.
2. **Read every log**, as `loadFrom` read every chat before, since the
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
   - A log with no entry in the index gets one, after the chats the index
     lists: `updated` from the last question's `time`, else the `chat`
     line's; the title from the last `title` event, else `title()` of the
     first question, as `send` makes it.
   - An entry whose log is missing is dropped.
   - If `index.json` is missing or cannot be read, it is rebuilt from the
     logs this way. Pins and drafts are lost in that case.

## Migrating from chats.json

At launch, if `chats/index.json` does not exist and `chats.json` does:

1. Read `chats.json` as `loadFrom` used to, including `link()` for chats
   kept before versions existed. If it cannot be read, nothing is moved
   and it is left where it is.
2. For each chat, write `chats/<id>.jsonl`: the `chat` line, then every
   message in **replay order** (`replayOrder`), then one `title` line,
   then a `show` line whose `leaf` is the last message of `Messages`.
   Replay order is depth first from the first questions. Of the versions
   of one message, the one it showed last (its `next`) comes after the
   others, with everything that followed each. So every message comes
   after its parent, and the last line in a message's subtree runs down
   the version it showed last. Replaying the log then gives every message
   the `next` it had, even on paths not shown, without writing `next`.
3. Write `index.json`, with `next` set to the counter.
4. Rename `chats.json` to `chats.json.migrated`. It is kept, not deleted.
   Nothing reads it again, and the user can delete it.

Each log and the index are first written to `.tmp` files, then renamed.
The migration counts as done only once `index.json` exists. If it is
interrupted, it runs again at the next launch and overwrites what it
wrote.

On a copy of a development data folder (47 chats, 151 messages, 226 KB),
every chat came back the same as `chats.json` read the old way: path,
versions, `next`, text, reasoning, files, errors, usage and times.
Migrating and loading took 14 ms, a later load 3 ms.

## Tests

[store_test.go](../store_test.go):

- `TestLogReplaysVersions`: a chat sent, asked again, edited and taken
  through its versions replays to the same path, other versions and
  `next` of every message, and IDs continue past it.
- `TestLogKeepsChanges`: title, pin, draft and provider come back; saving
  twice appends nothing; HTML is not escaped.
- `TestLogKeepsQuestionBeforeReply`: the question is on disk while its
  reply streams, and the reply once it is stopped.
- `TestLogBrokenLines`: a broken last line is cut and the next append
  starts a line of its own; a broken line in the middle is skipped.
- `TestIndexRepaired`: a log with no entry, an entry with no log, and no
  index at all.
- `TestLogDeleted`: deleting one chat, then all, deletes their logs.
- `TestMigrateChatsJSON`: versions, `next` off the path shown, a failed
  reply, a pin, a draft, and a chat kept before versions.
- `TestLogsWrittenAtOnce`: two replies ending at once, under `-race`.
- `TestLogKeepsQuestionBeforeReply` also checks a crash during a reply:
  the folder copied as the crash leaves it, the reply recovered from its
  partial, logged, and the partial deleted.
- `TestLogRetriesFailedWrites`: appends and a delete into a folder that
  takes no files, then the retry.
- `TestSweepAttachments`: an old file no log names is deleted; a new one,
  and an old one a log names, are kept.

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
