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
selection, links. Beyond those, it marks HyperAI itself — its mark over
an empty chat, the sparkle by the model — and inline code, as Waku colors
it. Nothing else: no accent
backgrounds, no accent for flavor, no accent on what is inactive.

**The muted-text floor.** Text at 13 DIPs or under is `fg/70` or darker.
`fg/40` is for placeholders and decoration only, never for something the
user must read to act.

## 2. Typography

The system font, as macOS and Windows draw their own apps: it renders
Chinese, Japanese and Latin natively, which a brand font would have to
prove it can. Code is the system monospace. The Appearance settings can
set another for the whole window (`Theme.Font`), and another for code
(`markdown.Theme.CodeFont`); the sizes and weights below stay as they
are.

Sizes are DIPs. The window's theme size is the platform's (13 on macOS,
14 elsewhere); the conversation is larger, for reading.

| Role | Size / weight | Where |
| --- | --- | --- |
| Display | 26 / 400 | The empty chat's greeting |
| Reading | 15, line 1.6 | Messages and the composer (`bodySize`, `bodyLineHeight`), as hyperai-next's `text-[15px] leading-relaxed` |
| Title | 14 / 500 | The chat's title in the title bar; the search's field |
| UI | 13 (theme) | Rows, tabs, menus, buttons |
| Label | 12 | Metadata, section titles, shortcuts, times — at `fg/70` |
| Key | 11 | Keys drawn as keys, in the search's footer |

Markdown sizes its headings and code from the reading size
(`markdown.Theme`): code at 0.875 of it, headings from 1.6 down.

Weights are 400, 500 for emphasis (the shown chat, the chosen tab, a
match) and 600 for Markdown's headings; the greeting, large enough to
stand out by size, is 400. Never bold UI text for
attention; move it up the ladder instead.

## 3. Shape and space

- **Radii.** 4 for keys, 6 for icon buttons, 8 for rows, tabs and menus,
  12 for cards (the outline's card), 14 for the user's
  bubble, 16 for the search, 24 for the composer — a pill one line tall,
  on an empty chat. Round things are round: the
  send and scroll-to-end buttons.
- **Rows** of a list are 7–8 DIPs over and under their text, 12 to its
  sides, 8 in from the list's left, and 12 in from its right (`barClear`)
  so that their wash clears the overlay scroll bar, 2 apart (`rowGap`) so
  that two washes never touch.
- **Icons** are Lucide's, at 2-DIP strokes: 16 in rows and buttons, 14 in
  the title bar and on messages, where they repeat.
- **Measure.** The conversation and the composer are 780 wide at most
  (`maxWidth`). The sidebar is 272 at first, and as wide as its edge is
  dragged, from 200 to 420, leaving the conversation 360 at least
  ([layout.go](../layout.go)); a double click on the edge makes it 272
  again. Its edge shows a 2-DIP bar of `fg/35` while the pointer is over
  it or drags it.

## 4. Elevation

Flat at rest; a shadow is a state, not decoration. Surfaces at rest are
told apart by a hairline (`Theme.Border`) and at most a whisper of shadow.
A shadow appears when something floats, or answers the pointer.

Shadows are `fg` with a third of `ac` mixed in ([`palette.shadow`](../theme.go)),
so that depth keeps the theme's tone instead of reading as grey smudge.

| Level | Shadow | Where |
| --- | --- | --- |
| Rest | 0 4 16 −4 at 12% | The composer |
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
- **Composer**: on `composer`, radius 24, a hairline and the rest shadow.
  Over a conversation it is a box: the field across, over a row with the
  model and the send button on the right. On an empty chat, as ChatGPT's,
  it is a pill while the draft is a line that fits beside those buttons,
  and opens into the box as the draft breaks a line or outgrows it. Its
  text is 20 in from its left, and 16 under its top. A long draft
  scrolls up into that room and fades out in it, as the conversation does
  over the composer, but keeps clear of the edge: the box's color over
  the room's first 6, then a fade to clear by the room's end. The fade
  stays in the room: text at rest under it is never greyed, however far
  the field is scrolled (a fade reaching into the text greyed the whole
  first line at the end of a long draft). The box's line is drawn last,
  over what is in it, its own border clear for the room it takes: the
  fade, drawn over a line drawn first, cut it where the corners round. The fade comes in
  with the scrolling (none at the field's top) and keeps off the rounded
  corner and the scroll bar. It is two pieces, a solid one and a plain
  fade: MyGo draws a gradient's `Start` and `Stops` as a plain fade, edge
  to edge. It has no buttons for what the app cannot
  do (attachments, voice). Over a conversation it floats over the end, on
  the canvas, which fades the conversation out above it.
- **Fields**: no box of their own inside the composer and the search,
  which are the box; no focus ring; the placeholder at `fg/40`.
- **Messages**: the user's in a `raised` bubble on the right; replies as
  Markdown across the column under a rule saying how long they took.
  Their actions (copy, time) fade in under the pointer in 150 ms; the last
  reply's stay. A question past 8 lines shows 6, fading out from 70% of
  their height, over a muted "Show more" that takes the accent under the
  pointer, as hyperai-next's agent does.
- **Empty chat**: as hyperai-next's agent page, the composer in the
  middle — the greeting over it (the HyperAI mark from
  `resources/logo.svg`, 48, in the accent as hyperai-next fills its Logo,
  over one question of what HyperAI is for, 26/400: "What are you
  building with AI today?"), prompts under it, and a word on the replies
  at the window's foot. A prompt is a row, not a card: its icon under the
  composer's text, 14 at `fg/70`; the pointer brings the text to `fg` and
  the icon to the accent. A click writes it in the composer, the caret at
  its end, to send as it is or changed. No grid of cards.
- **Settings**: a row at the sidebar's foot (⌘,) turns the sidebar into
  theirs, as Codex's: "Back to app" (muted, as Escape), a field of its
  own — 34 tall, radius 8, on `composer` with a hairline, since the
  sidebar is no box — and the pages by group, in the chat rows' look,
  the one shown on the 7% wash with its label at 500. The pane shows the
  page, 640 wide at most: its title at 20/500 over what it holds at
  `fg/70`. Showing a chat leaves them.
- **Settings pages** are groups 28 apart, each a title at 12 `fg/70`
  over a card, as macOS's: `composer`, radius 12, a hairline round it and
  between its rows, in 16 from its left. A row is what it sets at 14 over
  more of that at 12 `fg/70`, and its control on the right: MyGo's switch
  or segmented control, in the theme's colors. A control the desktop
  decides (no material to show, motion the system reduces) is disabled,
  as the desktop has it, and its row says why.
- **Appearance** offers only what the app does: the theme — System,
  Light, Dark, each a card drawing HyperAI small in its palette (System
  half light, half dark), ringed in the accent at 2 while chosen, as the
  app follows it, window controls too (`mygo.Theme.SetSource`); the
  font and the conversation's text, 14, 15 or 17, over a question and a
  reply in them, since the settings hide the conversation; the translucent
  sidebar; and reduce motion. No accent picker: there is one accent.
  The font and code's are rows of the Text card, over the size: each a
  pop-up button showing the family, which opens the system's own menu,
  as macOS's font settings — System, a separator, then every family
  installed, the one chosen checked; for code, the monospaced families,
  and the rest under Other Fonts. No sample cards: the preview under
  them, a question and a reply with code in a line and in a block,
  shows both.
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
