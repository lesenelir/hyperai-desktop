# Chat storage

How HyperAI keeps its chats on disk, and how it moves from one
`chats.json` to a log per chat ([store.go](../store.go)). The comparison
it rests on looked at ChatWise and Alma, which use SQLite, and at Codex,
DeepSeek Harness and Claude Code, which keep an append-only JSONL file per
session ([What other apps do](#what-other-apps-do)).

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

**SQLite is not planned.** Claude Code keeps every session as JSONL
with no database at all ([What other apps do](#what-other-apps-do)), and
what could outgrow files here has answers in files first
([Later](#later)). SQLite comes in only if those are measured not to be
enough: instant full-text search over tens of thousands of chats, or more
than one process writing the chats at once; and then as an index derived
from the logs, not in their place. Moving to SQLite for good would take
features that change messages in place often, editing or deleting any one
of them, which the version tree and Agent mode do not.

## What other apps do

How six apps on one Mac kept their chats in November 2026, read from their
data folders and, for Waku, its source. These are internals, not specs,
and may change. They are the evidence for the decision above, not a
description to keep current.

| App | Source of truth | Listing and queries | Large payloads | Undoing an agent's file edits |
| --- | --- | --- | --- | --- |
| ChatWise | SQLite (`app.db`): a row per chat and per message, text as is; tool calls in a JSON `meta` column | SQL | `files/` | — |
| Alma | SQLite (`chat_threads.db`): a message as AI SDK `parts` (`text`, `reasoning`, `tool-*`) in one JSON column; a Markdown copy of each chat | SQL, FTS5, vector memory | `artifacts/` | — |
| Waku | SQLite (`app.db`, WAL): narrow `sessions` rows for the list, a row per message, the rest as JSON; a daemon owns it | SQL; a session's messages read only once it is opened | `blobs/`, by content hash, swept against the rows | Hidden git refs per turn |
| DeepSeek Harness | A JSONL event log per session, zstd-compressed, under a folder per working directory (`~/.dsh/sessions/`) | A JSON cache per session | — | — |
| Codex | A JSONL rollout per session (`~/.codex/sessions/<date>/`), zstd once finished | SQLite projected from the rollouts, which notes the byte each was read to | Inline | — |
| Claude Code | A JSONL log per session, under a folder per working directory (`~/.claude/projects/`); no database at all | Reads the files | `tool-results/` beside the log, which keeps a reference | `file-history/`: each file backed up before each edit, by version |

The pattern: **apps that drive their own agent loop keep an append-only
log per session**, as Codex, Claude Code and DeepSeek Harness do. **Apps
that hold mutable state over it use SQLite**: a chat app updating rows in
place (ChatWise, Alma), or a control plane over other agents, whose
transcripts those agents keep themselves (Waku). HyperAI is to run its own
agent loop, which puts it in the first group.

What HyperAI takes from them:

- **A log per chat, only appended to,** from Codex, Claude Code and
  DeepSeek Harness; and, like Claude Code, no database. If one is ever
  needed, it comes as Codex's does: an index derived from the logs
  ([Later](#later)).
- **Messages as a tree**, as Claude Code's `parentUuid`: an edit or a
  reply asked for again is a branch, and nothing is rewritten. HyperAI's
  `parent` and versions were already this.
- **A narrow list apart from the transcripts,** as Waku's `sessions` rows
  and Codex's `threads` table: `index.json`.
- **Writing again what failed**, a streaming reply kept each second, and
  sweeping files no chat names, from Waku's store ([Writing](#writing)).
- **Payloads out of the log** and **backups before edits**, for Agent mode
  ([Later](#later)).

## Layout

```
<user data>/
  chats/
    index.json          the chats, for the sidebar
    <id>.jsonl          one chat's log, by its ID
    <id>.partial        the reply streaming in, while it does
  attachments/          unchanged
  chats.json.migrated   the old file, kept after migrating
  chats.v1/             the logs of version 1, kept after their IDs became UUIDs
```

`<user data>` is `~/Library/Application Support/hyperai-desktop/` on macOS
(`hyperai-desktop Dev/` for `mygo dev`), as today. Every file is written
with mode 0600, as `chats.json` is.

## IDs

Every chat and message has an `ID` ([ids.go](../ids.go)): a **UUIDv7**,
as text, such as `01a197e4-20fd-7b13-b046-ef98264f5d3b`. It used to be an
integer from one counter (`a.next`) that chats and messages shared, which
two computers would both hand out: their chats could not come together,
as syncing them would need. A UUID is unique wherever it was made.

- **IDs sort in the order they were made.** A UUIDv7 begins with the time
  in milliseconds, and `google/uuid` makes the IDs of one process only
  grow, even within a millisecond. So the code that took the larger ID for
  the later message still does: the versions of a message, oldest first
  (`children`), and the order a log appends new messages in.
- **`""` is none**, where 0 was: the parent of a chat's first question,
  and no version shown after a message.
- **Old IDs are read as their digits.** `ID` reads a JSON number too
  (`UnmarshalJSON`), as `chats.json` and the logs of version 1 hold them,
  and `legacy` tells such an ID. `upgradeIDs` gives them new UUIDs, in the
  order of the numbers, so the order of the versions stays as it was, and
  rewrites each message's `parent` and `next` to match.
- **Copy as JSON** (export) writes the IDs as text too.

## The index

`chats/index.json` holds what changes often and what the sidebar shows,
without reading any log:

```json
{
  "version": 2,
  "chats": [
    {
      "id": "01a197e4-20fd-7b13-b046-ef98264f5d3b",
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
- **There is no counter.** Version 1 kept `next`, the counter of the
  integer IDs; UUIDs need none.
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
{"t":"chat","at":"…","v":2,"id":"01a197e4-…-5d3b"}
{"t":"message","at":"…","id":"01a197e4-…-2088","role":"user","text":"…","files":[{"name":"a.pdf","type":"application/pdf","file":"15f99a47b549ec16.pdf"}],"time":"…"}
{"t":"title","at":"…","title":"…"}
{"t":"message","at":"…","id":"01a197e4-…-5ccb","parent":"01a197e4-…-2088","role":"assistant","text":"…","reasoning":"…","reasoned":3.2,"usage":{"input":1200,"output":800},"firstToken":0.8,"elapsed":12.5,"time":"…"}
{"t":"message","at":"…","id":"01a197e4-…-91d0","role":"user","text":"… edited","time":"…"}
{"t":"show","at":"…","leaf":"01a197e4-…-5ccb"}
```

| Type | Written when | Fields |
| --- | --- | --- |
| `chat` | The chat is created: the first line, and only the first | `v`, the log format version (2; 1 had integer IDs); `id`, the chat's ID |
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
- **The index waits for the logs:** where any change before it failed, it
  is not written, and the next save writes it whole. So the index is never
  ahead of a log, which lets loading take a log's title over the index's
  (below).

### The reply streaming in

While a reply streams, `keepPartial` writes it, as a `message` line, to
`chats/<id>.partial`, at most once a second (`partialEvery`), in place of
the last (through a `.tmp` file and a rename). Waku saves a streaming
reply each second for the same reason, as a row it updates in place. A log
that is only appended to cannot be updated, so the reply is kept in a
file of its own, which is deleted once the reply is in the log: the save
that appends it, as it ends or is stopped, also deletes the partial, after
the line, in the writer's order. Where that append failed, the log is
broken and the partial is kept (`unless`): a crash before the log is
written again still leaves the reply, and the save that writes it again
deletes the partial.

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
   - A chat takes its title from its log's last `title` event, where there
     is one, and its date from the index or its last question, whichever
     is later. A crash between a log and the index leaves the index
     behind, and its title would otherwise come back, and be appended to
     the log again by the next save. Pins, drafts and models are in the
     index alone.
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
   kept before versions existed.
2. Give the chats and messages UUIDs (`upgradeIDs`).
3. Write the chats' folder (`writeChats`, below). For each chat,
   `chats/<id>.jsonl` holds the `chat` line, then every
   message in **replay order** (`replayOrder`), then one `title` line,
   then a `show` line whose `leaf` is the last message of `Messages`.
   Replay order is depth first from the first questions. Of the versions
   of one message, the one it showed last (its `next`) comes after the
   others, with everything that followed each. So every message comes
   after its parent, and the last line in a message's subtree runs down
   the version it showed last. Replaying the log then gives every message
   the `next` it had, even on paths not shown, without writing `next`.
   The folder's `index.json` lists them all.
4. Rename `chats.json` to `chats.json.migrated`. It is kept, not deleted.
   Nothing reads it again, and the user can delete it.

The migration counts as done once `chats/index.json` exists. If it is
interrupted before, it runs again at the next launch, with new UUIDs.

The chat shown last (`layout.json`'s `chat`) takes its new ID, and
`layout.json` is saved with it, so that it shows at this launch and the
ones after.

If the migration fails, as where `chats.json` cannot be read or the new
folder cannot be written, **nothing is saved until a launch moves the
chats**. A save would write `chats/index.json`, which says the chats were
moved: later launches would never move them, and they would not show
again. Until then, the chats show as `chats.json` has them, where it could
be read, and `chats.json` stays where it is. Deleting a chat then deletes
none of its attachments (`forgetFiles`): it comes back at the next launch,
and its files must still be there.

## Upgrading the logs of version 1

The logs written before IDs were UUIDs (version 1) are named by numbers,
hold numbers for IDs, and their index keeps the counter. At launch,
`loadFrom` reads them as any logs, their IDs as digits; if any ID is
legacy, it gives them all UUIDs (`upgradeIDs`), writes the chats' folder
anew (`writeChats`), keeping the old one as `chats.v1/`, and reads the new
one. The chat shown last (`layout.json`'s `chat`) takes its new ID, as in
the migration. A
partial of a reply a crash cut short is read first, so it goes in its
chat's new log.

If writing the new folder fails, the app runs on with the chats in memory,
but saves nothing until it starts again: saving would put logs named by
UUIDs beside those named by numbers.

### Writing the folder at once

Both the migration and the upgrade write every log, so both write a whole
new folder at once (`writeChats`): into `chats.new/`, then the old
`chats/`, where there is one, is renamed to the backup, then `chats.new/`
to `chats/`. A crash leaves one of three states, which `finishSwap` sorts
out at the next launch, before anything is read:

The new folder's `index.json` is written last, through a `.tmp` file and
a rename, so a new folder that holds one is whole.

- `chats.new/` with an index, and no `chats/`: the old folder was renamed
  away, which happens only once the new one is whole, or there was none,
  as for a migration. It takes its place.
- Any other `chats.new/`: it may not be whole (no index), or the old
  folder is still there. It is deleted, and the migration or upgrade runs
  again.
- No `chats.new/`: nothing was cut short.

On a copy of a development data folder (47 chats, 151 messages, 226 KB),
every chat came back the same as `chats.json` read the old way: path,
versions, `next`, text, reasoning, files, errors, usage and times.
Migrating and loading took 14 ms, a later load 3 ms. Upgrading the same
folder's logs of version 1 (48 chats, 156 messages) took 17 ms, and every
chat came back the same, with UUIDs.

## Tests

[store_test.go](../store_test.go) and [ids_test.go](../ids_test.go):

- `TestIDs`: new IDs grow and are unique; numbers read as legacy IDs;
  `upgradeIDs` keeps the order of the numbers (9 before 10) and the links.
- `TestUpgradeV1Logs`: logs of version 1, made from chats with versions, a
  pin and a chat shown last, upgrade to the same chats with UUIDs, keep the
  old folder, and read the same IDs at the next launch.
- `TestFinishSwap`: the states a crash can leave the folder in,
  including a new folder cut short with no old one beside it.
- `TestMigrateFails`: a `chats.json` that cannot be read, or a folder that
  takes no files, saves nothing; a chat deleted then keeps its files; and
  the next launch moves the chats, with them.
- `TestMigrateKeepsChatShown`: the chat shown last shows after the
  migration, and at the launch after.
- `TestPartialKeptUntilLogged`: a partial whose reply fails to reach the
  log is kept, and a crash then leaves the reply.
- `TestIndexBehindLog`: an index a crash left behind its log takes the
  log's title and the later date, and no title is appended again.

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

- **If loading grows slow, read logs lazily.** Measured as in
  [performance.md](performance.md) (48 chats load in 3 ms today), and once
  a launch is felt, around 100 ms: the sidebar from `index.json` alone, a
  chat's log once it is opened. Claude Code reads a session only to resume
  it.
- **If the search grows slow, scan the logs on a goroutine**, showing what
  it finds as it goes: thousands of chats, tens of megabytes, take well
  under a second to read. Tags, folders and filters go in `index.json`;
  statistics are summed from the logs once and cached.
- **Only if those are not enough, a derived SQLite index**: with
  `ncruces/go-sqlite3`, on the wazero the app has, without cgo, and FTS5;
  filled from the logs, noting the byte each was read to, as Codex's
  `thread_history` is from its rollouts; deleted and rebuilt at will. The
  logs stay the source, and are written as before.
- **Compressing old logs** with zstd, as Codex does for sessions it no
  longer writes.
- **Rewriting a log** to drop what was replaced, if logs ever grow large
  from edits. No current feature needs it: chats are deleted whole and
  messages never are.
- **For Agent mode, large payloads out of the log**: a long tool output or
  a screenshot written to a file beside it (`<id>/`, or a store by
  content hash, as Waku's `blobs/`), the log keeping a reference, as
  Claude Code's `tool-results/` does. The log stays small and quick to
  replay.
- **For Agent mode, a backup of each file before the agent edits it**, as
  Claude Code's `file-history/`, so that asking for a turn again, or
  going back to a version, can put the files back as they were before the
  turn. It works outside git, unlike Waku's refs; in a git repository,
  refs are the cheaper record of a whole tree.
