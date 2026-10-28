# Design

How HyperAI looks, and the rules that keep it looking like one thing.
Adapted from hyperai-next's `DESIGN.md`, the web app whose colors this one
takes: the system there is parametric OKLCH tokens in CSS and Tailwind
classes; here it is the same tokens resolved in Go ([theme.go](../theme.go))
and MyGo elements. What carries over is the palette, the text ladder, the
flat-at-rest elevation and the rules; what changes is everything that
depends on the web (classes, rem, focus rings drawn by the browser), and
the sizes, which follow a native desktop app rather than a web page.

**North star: a quiet room to think in.** The conversation is the content;
the chrome — sidebar, composer, search — stays out of its way. It is calm,
precise and warm, and it does not dress like AI hype: no neon on black, no
glow, no gradients for their own sake.

## 1. Colors

A warm near-white canvas, one red accent, and everything else a
transparency of the foreground. Light and dark are the same system with
different parameters ([`newPalette`](../theme.go)), not two palettes.

### Tokens

| Token | Light | Dark | What it is |
| --- | --- | --- | --- |
| `fg` | oklch(30% 0.01 20) | oklch(98% 0.01 4.45) | Text, and through transparency every border, wash and muted tone |
| `bg` | oklch(100% 0.01 20) | oklch(20% 0.01 4.45) | The warmest surface: the composer and cards in light |
| `ac` | oklch(50% 0.28 40) | oklch(90% 0.212 8) | The accent: a vivid red, a pink in dark |
| canvas (`Theme.Background`) | `fg` 2.5% over `bg` | `bg` | The window behind the conversation |
| `composer` | `bg` | `fg` 4.5% over `bg` | Raised surfaces: the composer, the search, cards |
| `raised` | `fg` 7% over canvas | same | The user's message bubble |
| `scrim` | `fg` 16% | oklch(10% 0.01 4.45) at 50% | Dims the window under a dialog |

The hyperai-next values are CSS's; [`cssOklch`](../theme.go) resolves
them as a browser on a Mac draws them, each channel clipped into Display
P3, so that the light accent is the same vivid red, not the brown that
mapping its chroma down gives.

### The text ladder

Every text color is a tier of `fg`. Snap to these; don't invent alphas
between them.

| Tier | Token | Use |
| --- | --- | --- |
| `fg` | `Theme.Text` | Titles, the conversation, menu and row labels |
| `fg/70` | `Theme.TextMuted` | Metadata (model, age, times), section titles, shortcuts, helper text — the floor for meaningful small text |
| `fg/40` | `app.placeholder` | Placeholders, and decoration next to text that says the same |
| `fg/20`–`fg/35` | | Disabled controls and resting decoration (the outline's ticks, the idle send arrow) |

### Washes and lines

Fills behind interactive things are faint `fg` too, so they keep the
theme's warmth: 4% under the pointer, 5–6% for a hovered or chosen row of
a list that is not the selection, 7% for the selection, the chosen tab
and the user's bubble. Borders are `fg` 12% over the canvas
(`Theme.Border`); dividers are borders.

### Named rules

**The parametric rule.** No hex. Every color is a token from
[theme.go](../theme.go), or a transparency of one (`fg.Alpha(0.07)`,
`ac.Alpha(0.35)`). A hex value in view code is a bug: it will be right in
one theme and wrong in the other.

**The one accent rule.** The accent marks what acts and what is chosen:
the send button, the chat shown in the sidebar, the caret and the
selection, links. Beyond those, it marks HyperAI itself — the sparkle by
the model — and inline code, as Waku colors it. Nothing else: no accent
backgrounds, no accent for flavor, no accent on what is inactive.

**The muted-text floor.** Text at 13 DIPs or under is `fg/70` or darker.
`fg/40` is for placeholders and decoration only, never for something the
user must read to act.

## 2. Typography

The system font, as macOS and Windows draw their own apps: it renders
Chinese, Japanese and Latin natively, which a brand font would have to
prove it can. Code is the system monospace.

Sizes are DIPs. The window's theme size is the platform's (13 on macOS,
14 elsewhere); the conversation is larger, for reading.

| Role | Size / weight | Where |
| --- | --- | --- |
| Display | 26 / 600 | The welcome's greeting |
| Reading | 15, line 1.6 | Messages and the composer (`bodySize`, `bodyLineHeight`), as hyperai-next's `text-[15px] leading-relaxed` |
| Title | 14 / 500 | The chat's title in the title bar; the search's field |
| UI | 13 (theme) | Rows, tabs, menus, buttons |
| Label | 12 | Metadata, section titles, shortcuts, times — at `fg/70` |
| Key | 11 | Keys drawn as keys, in the search's footer |

Markdown sizes its headings and code from the reading size
(`markdown.Theme`): code at 0.875 of it, headings from 1.6 down.

Weights are 400, 500 for emphasis (the shown chat, the chosen tab, a
match) and 600 for headings and the greeting. Never bold UI text for
attention; move it up the ladder instead.

## 3. Shape and space

- **Radii.** 4 for keys, 6 for icon buttons, 8 for rows, tabs and menus,
  12 for cards (suggestions, the outline's card), 14 for the user's
  bubble, 16 for the composer and the search. Round things are round: the
  send and scroll-to-end buttons.
- **Rows** of a list are 7–8 DIPs over and under their text, 12 to its
  sides, 8 in from the list's left, and 12 in from its right (`barClear`)
  so that their wash clears the overlay scroll bar, 2 apart (`rowGap`) so
  that two washes never touch.
- **Icons** are Lucide's, at 2-DIP strokes: 16 in rows and buttons, 14 in
  the title bar and on messages, where they repeat.
- **Measure.** The conversation and the composer are 780 wide at most
  (`maxWidth`); the sidebar is 272.

## 4. Elevation

Flat at rest; a shadow is a state, not decoration. Surfaces at rest are
told apart by a hairline (`Theme.Border`) and at most a whisper of shadow.
A shadow appears when something floats, or answers the pointer.

Shadows are `fg` with a third of `ac` mixed in ([`palette.shadow`](../theme.go)),
so that depth keeps the theme's tone instead of reading as grey smudge.

| Level | Shadow | Where |
| --- | --- | --- |
| Rest | 0 4 16 −4 at 12% | The composer |
| Intent | 0 8 24 −6 at 18% | A suggestion card under the pointer |
| Float | 0 10 24 −6 at 20%; 0 16 40 −12 at 28% | The outline's card; the search |
| Action | 0 3 8 −2, `ac` at 35% | The send button, ready |

A small button that floats over the conversation (scroll to end) rests at
0 4 6 −1 at 16% and lifts 4 DIPs to 0 8 16 −4 at 22% under the pointer.

## 5. Components

- **Icon buttons** ([`iconButton`](../app.go)): 30×28, radius 6, muted;
  under the pointer, a 6% wash and the text color. Disabled ones fade to
  35%.
- **Sidebar rows**: a title over the model and age, as the rows of every
  list. The chat shown takes a 7% wash and its title the accent, at 500.
  The sidebar shows the desktop's material where it can, tinted with its
  own warm color (`sidebarTint`) so that it sits with the canvas.
- **Composer**: on `composer`, radius 16, a hairline and the rest shadow;
  it floats over the conversation's end, on the canvas, which fades the
  conversation out above it.
- **Fields**: no box of their own inside the composer and the search,
  which are the box; no focus ring; the placeholder at `fg/40`.
- **Messages**: the user's in a `raised` bubble on the right; replies as
  Markdown across the column under a rule saying how long they took.
  Their actions (copy, time) fade in under the pointer in 150 ms; the last
  reply's stay.
- **Search**: a card near the window's top over a scrim, its field over
  tabs (All, Chats, Projects, Artifacts, Settings), titled sections of
  rows in the sidebar's look, and the keys it takes in its footer.

## 6. Motion

State changes move for 150–300 ms and then stop: actions fading in
(150 ms), the outline's ticks (200 ms), the sidebar sliding (220 ms), a
reply's words fading in (300 ms). Scrolling to a place glides, and goes
there at once where the desktop asks for less motion. Nothing loops while
the app is idle, and nothing moves for as long as a reply streams but its
words ([performance.md](performance.md)).

## 7. Do and don't

**Do**
- Take every color from the palette, muted tones and lines as `fg`
  transparencies.
- Keep small text at `fg/70` or darker, in both themes.
- Give a new list the sidebar's row: its padding, radius, washes, the
  room for the scroll bar, the gap.
- Give every control its states: under the pointer, pressed, focused,
  disabled.
- Try a layout with a long Chinese title and a long English one.

**Don't**
- Write a hex color, or a grey that is not `fg` at some alpha.
- Use the accent to decorate.
- Add a shadow that is not in the table above.
- Copy another app's sizes as they are: take its structure, and size it
  to this one (13 for the UI, 15 to read, 12 for labels).
- Animate on a loop, or for as long as a reply streams.
