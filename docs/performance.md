# Streaming render performance

How HyperAI keeps the CPU low while a reply streams in, what each piece of
the pipeline is allowed to cost, and how to measure before changing any of
it. Adapted from Waku's `docs/performance.md`: Waku is Rust on GPUI, and
most of its document is about GPUI's views, caches and animation frames;
what carries over is the model, the two cadences, and the measuring
discipline, restated for MyGo. It records the results of the 2026-10-28
streaming investigation, which took a three-reply stream from 2.82 s of CPU
to 1.89 s (−33%), and a frame with 500 chats from 1.74 ms to 0.18 ms.

The one-line model: **CPU ≈ frames per second × work per frame.** MyGo's
native UI is immediate mode: every frame calls `app.view`, which builds the
whole tree again — sidebar, title bar, the visible rows of the
conversation, composer — then lays it out and paints it. Every rule below
either bounds how often frames happen or how much one frame does.

## Who causes a frame

MyGo draws a frame only when something changes; an idle window draws
nothing. What asks for one:

| Trigger | Cost | Allowed users |
| --- | --- | --- |
| Input | one frame per event (a pointer move over a hover target, a key) | the user |
| `Window.Update(fn)` | runs `fn` on the main thread, then one frame. Updates asked between two frames **coalesce into one frame**, but each still runs its `fn` | goroutines publishing results: the reply stream's commits |
| Animations | a frame per display refresh (120 Hz on ProMotion) for as long as something moves | `ui.Spinner` while thinking, the sidebar slide (220 ms), the glide of **Scroll to end** (once, on a click), `markdown.Stream`'s fade-in |

`markdown.Stream` reveals the reply a word at a time
(`markdown.DefaultReveal`), lets text wait at most 250 ms, and fades each
word in over 300 ms. While a word is fading, the window draws at the
display's rate, whatever the provider's chunk rate; that is most of a
stream. So a frame must build, lay out and paint well within 8.3 ms — it
takes about 1.3 ms today.

**Nothing may move for as long as a reply grows.** The conversation's list
follows its end by jumping to it, not gliding (`FollowAnimation` unset,
[chat.go](../chat.go)): a reply grows all the time it streams, so a glide
never settles, and `ui.Smooth` alone kept the window at 120 Hz for the
whole stream. Dropping it cut the stream's CPU by about a quarter. The
same goes for any repeating animation (a pulsing dot, a shimmer): one in
Waku pinned its window at display rate for a whole turn, ~36% CPU by
itself.

## The two cadences

**Commits, ≤ ~20 Hz.** Provider chunks arrive on the stream's goroutine
([`app.send`](../app.go)), every token or few, often tens a second (the
demo sends one every 8–38 ms). A [`batcher`](../stream.go) gathers them and
commits through `Window.Update` at most every `commitInterval` (50 ms): the
first chunk of a batch commits at once if the last commit is older than
that, the end of the stream commits what is left at once, and commits are
made in order, under the batcher's lock. The reveal paces words out of what
came, so the batching does not show.

Measured, batching changed the stream's CPU by less than the noise between
runs, since the fade sets the frame rate; it cut allocations by 7% and text
layouts by 14%. It stays because it makes the main thread's work per second
independent of how chatty a provider is.

**Animation frames, ≤ display rate.** Keep what moves small and
short-lived: the spinner only while no text has come, the fade only while a
word is coming in.

## Bounding the work in a frame

- `ui.List` builds only the visible rows of a conversation; a long chat
  costs what a screenful does. Keep the row builder (`app.message`) free of
  per-chat work.
- `markdown.View` and `markdown.Stream` parse again only where the source
  changed since the last frame, and `Stream` parses appended text alone.
  Switching a message from `Stream` to `View` in the same place when it
  completes keeps the parse: do not key the two differently.
- The sidebar is built every frame too, streaming or not, so it is a
  `ui.List` as well: a frame builds only the rows that show. Its rows —
  chats grouped by day — are computed by `chatRows` only when `app.rev`
  changes (a chat made, updated or removed) or the day does. Anything that
  changes a chat's place in the sidebar must bump `rev`.
- The search ([search.go](../search.go)) looks through every message of
  every chat, so it does so once per change of its query (or of `rev`),
  not once per frame, and lists 50 chats at most (12 on All).
  `BenchmarkFrame` ([bench_test.go](../bench_test.go)) measures this: with
  500 chats a frame took 1.74 ms when the sidebar built every row and
  grouped them each frame, and takes 0.18 ms now, near the 0.11 ms of 5
  chats.
- The outline beside the conversation ([outline.go](../outline.go)) shows
  at most 20 ticks, and keeps each message's preview once the message is
  complete, rather than collapsing every reply's text every frame.
- `view` reads state; it never does I/O. No file reads, no network, no
  `os.Getenv` in a frame.

## Off the main thread

The main thread builds frames, so anything slow there drops them:

- The provider streams on a goroutine of its own and publishes through
  `Window.Update`.
- `save` encodes the chats on the main thread, where they are safe to
  read, and a [`saver`](../store.go) writes them on a goroutine, one write
  at a time, only the latest of those waiting. The app waits for the last
  write as it quits (`mygo.App.OnQuit`).

## Measuring

Measure before and after, the same stream each time:

1. **`HYPERAI_BENCH=n`** runs the app against the demo, paced by a seeded
   random source so that every run streams the same chunks at the same
   times, sends n suggestions one after another, and quits. It leaves the
   user's chats alone. Build the app and time it whole:

   ```sh
   go build -o /tmp/hyperai . && /usr/bin/time -p env HYPERAI_BENCH=3 /tmp/hyperai
   ```

   `user` + `sys` is the CPU the stream cost. Run it at least twice, keep
   the window in front (macOS does not draw a hidden window, and a run
   with it covered reads far too low), and compare against a build of the
   code before the change, measured the same way.
2. **`MYGO_FRAME_STATS`.** `MYGO_FRAME_STATS=all` logs every frame: build
   (and its passes), layout, paint, present, the allocations and text
   layouts meanwhile, and GC. Count the lines for the frame rate, and read
   build time for the cost of `view`. This is MyGo's version of the
   per-second counters that found Waku's worst bugs where profiles showed
   nothing but generic layout. `MYGO_FRAME_STATS=1` logs only frames over
   8 ms.
3. **`go test -bench Frame`** for the cost of a frame without a window.
4. **The inspector's Performance tab** (F12 or `alt-cmd-i` in development
   builds) draws the last frames' times against the display's refresh.
5. **`sample <pid> 5`** during a stream, for *what* is expensive (layout vs
   text shaping vs app code); useless for *how often*.

Do not compare `ps -o %cpu` across runs: on macOS it is a decaying average,
and it made an unchanged stream read 14% in one run and 22% in the next.

## Results, 2026-10-28

Three demo replies, `HYPERAI_BENCH=3`, CPU as `user` + `sys`, averaged
over runs on an M1 Pro MacBook Pro at 120 Hz:

| Build | Frames | CPU |
| --- | --- | --- |
| Before (per-chunk commits, smooth follow, fade) | 1044 | 2.82 s |
| Batched commits | 1045 | 2.94 s |
| Batched commits, follow jumps (shipped) | 831 | 1.89 s |
| … and no fade (`Reveal{Fade: -1}`) | 251 | 1.20 s |
| … and no reveal (`Reveal{Unit: AtOnce}`) | 232 | 1.15 s |

The fade is kept: it is the look we want, and what remains of the 120 Hz
frames. If the stream's CPU has to come down further, turning it off is
the next lever — it costs no pacing, as words still come one at a time,
and takes the frame rate down to the commits'.
