# Streaming render performance

How HyperAI keeps the CPU low while a reply streams in, what each piece of
the pipeline is allowed to cost, and how to measure before changing any of
it. Adapted from Waku's `docs/performance.md`: Waku is Rust on GPUI, and
most of its document is about GPUI's views, caches and animation frames;
what carries over is the model, the two cadences, and the measuring
discipline, restated for MyGo.

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
| `Window.Update(fn)` | runs `fn` on the main thread, then one frame. Updates asked between two frames **coalesce into one frame**, but each still runs its `fn` | goroutines publishing results: the reply stream, saves finishing |
| Animations | a frame per display refresh (120 Hz on ProMotion) for as long as something moves | `ui.Spinner` while thinking, the sidebar slide (220 ms), `ui.List`'s smooth follow, `markdown.Stream`'s reveal |

`markdown.Stream` animates on its own: it reveals the reply a word at a
time (`markdown.DefaultReveal`), lets text wait at most 250 ms, and fades
each word in over 300 ms. So **while a reply streams, the window draws at
the display's rate anyway**, whatever the provider's chunk rate. That sets
the budget: a frame must build, lay out and paint well within 8.3 ms.

Never add a repeating animation that runs while nothing changes (a pulsing
dot, a shimmer): it pins the window at display rate for as long as it is
mounted, as one did in Waku for a whole turn at ~36% CPU by itself.

## The two cadences

**Commits.** Provider chunks arrive on the stream's goroutine
([`app.send`](../app.go)) and reach the state through `Window.Update`.
A provider sends a chunk every token or few, often tens a second, and a
chunk is tiny (the demo sends one every 8–38 ms). Chunks must be **batched**: appended to a buffer on the goroutine and
committed at most every ~50 ms, so that the main thread appends to the
reply and `markdown.Stream` parses new text a bounded number of times a
second rather than once per chunk. The reveal hides the batching — it
paces words out of what has arrived — so a coarser commit costs nothing
visible. The stream's end and errors commit at once.

**Animation frames, ≤ display rate.** Whatever moves draws every refresh.
Keep what moves small and short-lived: the spinner only while no text has
come, the reveal only while text is waiting.

## Bounding the work in a frame

- `ui.List` builds only the visible rows of a conversation; a long chat
  costs what a screenful does. Keep the row builder (`app.message`) free of
  per-chat work.
- `markdown.View` and `markdown.Stream` parse again only where the source
  changed since the last frame, and `Stream` parses appended text alone.
  Switching a message from `Stream` to `View` in the same place when it
  completes keeps the parse: do not key the two differently.
- The sidebar is built every frame too, streaming or not. What it computes
  must be cheap and proportional to what shows, and anything derived from
  all chats (grouping by day, sorting) should be computed when the chats
  change, not per frame.
- `view` reads state; it never does I/O. No file reads, no network, no
  `os.Getenv` in a frame.

## Off the main thread

The main thread builds frames, so anything slow there drops them:

- The provider streams on a goroutine of its own and publishes through
  `Window.Update`.
- Saving the chats writes a file. Encode on the main thread (the state is
  only safe to read there), write on a goroutine, one write at a time, the
  latest winning.

## Measuring

Measure before and after; in order of usefulness:

1. **`MYGO_FRAME_STATS`.** `MYGO_FRAME_STATS=all go tool mygo dev` logs
   every frame: build (and its passes), layout, paint, present, the
   allocations and text layouts meanwhile, and GC. `MYGO_FRAME_STATS=1`
   logs only frames over 8 ms. Count the lines a second for the frame rate,
   and read build time for the cost of `view`. This is MyGo's version of
   the per-second counters that found Waku's worst bugs where profiles
   showed nothing but generic layout.
2. **The inspector's Performance tab** (F12 or `alt-cmd-i` in development
   builds) draws the last frames' times against the display's refresh.
3. **`sample <pid> 5`** during a stream, for *what* is expensive (layout vs
   text shaping vs app code); useless for *how often*.
4. **CPU across a whole reply**: poll `ps -o %cpu= -p <pid>` every 500 ms
   from send until the reply settles. Report the average and the peak; an
   average hides plateaus.

Development builds (`mygo dev`) carry the inspector and development
features a release leaves out; treat their numbers as structure, and
confirm a user-facing claim on a `mygo build`.

## Known gaps

What the code does not yet do of the above, in the order to fix it:

- The stream commits every chunk with its own `Window.Update`.
- `save` encodes and writes `chats.json` on the main thread when a reply
  ends.
- The sidebar groups and sorts all chats by day every frame.
