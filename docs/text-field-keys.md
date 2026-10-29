# Text field key bindings

Every key the composer answers to, what it does, and where that behaviour
comes from. The composer is MyGo's `ui.TextAreaBase`
([chat.go](../chat.go)), so nearly all of it is the framework's editor
(`ui/editor_navigation.go` in the MyGo module); HyperAI adds only what a
chat needs on top, through `OnTextKey`, and a few window shortcuts. Adapted
from Waku's `docs/text-field-keys.md`, which documents a text field Waku
draws itself; here the table is a reference for what MyGo already does, so
that a change in behaviour can be told from a bug.

The composer is a multi-line field, so most keys are defined by the unit
they move over:

| Unit | Meaning |
| --- | --- |
| character | one grapheme |
| word | a Unicode word boundary |
| row | one *rendered* row: a soft wrap counts, a hard `\n` counts |
| paragraph | the text between hard `\n` breaks; soft wraps are ignored |
| document | the whole field |

Shift with any key that moves the caret extends the selection from its
moving end instead.

## What HyperAI adds

| Key | Does | Where |
| --- | --- | --- |
| `enter` | sends the draft and clears the field; does nothing while a reply streams or the draft is blank. Where General's Send with is ⌘ Return, it breaks the line instead | `composer`, `OnTextKey`, `app.sends` |
| `cmd-enter` | sends the draft where General's Send with is ⌘ Return (Ctrl+Enter elsewhere); else does nothing | `composer`, `OnTextKey`, `app.sends` |
| `shift-enter` | breaks the line, whichever key sends: HyperAI's handler declines it and the editor inserts `\n` | `composer`, `OnTextKey` |
| `escape` | stops the reply streaming in; the editor does not take Escape, so it reaches the window shortcut | `view`, `c.Shortcut` |
| `cmd-n` | starts a new chat and focuses the composer | `view` |
| `shift-cmd-s` | shows or hides the sidebar | `view` |

`alt-enter` and `ctrl-enter` (on macOS) are deliberately unbound, as is
`cmd-enter` while Return sends: the handler declines them and the editor
takes Enter only without modifiers, so they do nothing rather than send.

**Input methods.** While an input method composes (pinyin, kana), its keys
are its own: `OnTextKey` does not see them, so Enter commits the
composition rather than sending a half-typed message. Keep it that way;
any new key handling must go through `OnTextKey`, not a window shortcut, to
inherit this.

**Focus.** Choosing a chat in the sidebar, `New chat` and `cmd-n` give the
composer the focus again (`app.focusDraft`), so typing carries on where it
was.

## macOS

The behaviour follows AppKit's `StandardKeyBinding.dict`, the table every
native text view consults; the selector column names the AppKit action a
key maps to there.

### Caret

| Key | Does | AppKit selector |
| --- | --- | --- |
| `left` / `right` | one character; with a selection, collapse to its near edge | `moveLeft:` / `moveRight:` |
| `ctrl-b` / `ctrl-f` | one character | `moveBackward:` / `moveForward:` |
| `alt-left` / `alt-right` | one word (back in a right-to-left paragraph) | `moveWordLeft:` / `moveWordRight:` |
| `up` / `down` | one row, keeping the column across a short row | `moveUp:` / `moveDown:` |
| `ctrl-p` / `ctrl-n` | one row | `moveUp:` / `moveDown:` |
| `cmd-left` / `cmd-right` | row start / end | `moveToLeftEndOfLine:` / `moveToRightEndOfLine:` |
| `ctrl-a` / `ctrl-e` | paragraph start / end | `moveToBeginningOfParagraph:` / `moveToEndOfParagraph:` |
| `cmd-up` / `cmd-down` | document start / end | `moveToBeginningOfDocument:` / `moveToEndOfDocument:` |
| `home` / `end` | row start / end. Native views only scroll here; MyGo moves the caret | — |
| `pageup` / `pagedown` | a field's height of rows | `pageUp:` / `pageDown:` |

### Deleting

| Key | Does | AppKit selector |
| --- | --- | --- |
| `backspace` / `ctrl-h` | the character before the caret, or the selection | `deleteBackward:` |
| `alt-backspace` | to the start of the word | `deleteWordBackward:` |
| `cmd-backspace` | to the start of the row | `deleteToBeginningOfLine:` |
| `delete` (`fn-backspace`) / `ctrl-d` | the character after the caret | `deleteForward:` |
| `alt-delete` | to the end of the word | `deleteWordForward:` |
| `ctrl-k` | to the end of the paragraph; at its end, joins the next | `deleteToEndOfParagraph:` |

### Editing

| Key | Does |
| --- | --- |
| `cmd-a` | select all |
| `cmd-c` / `cmd-x` / `cmd-v` | copy / cut / paste |
| `cmd-z` | undo |
| `shift-cmd-z` / `cmd-y` | redo |

## Windows and Linux

The same, with the platform's conventions in place of AppKit's:

| Key | Does |
| --- | --- |
| `ctrl-left` / `ctrl-right` | one word |
| `alt-left` / `alt-right` | not taken: they go back and forward, as in a browser |
| `home` / `end` | row start / end |
| `ctrl-home` / `ctrl-end` | document start / end |
| `ctrl-backspace` / `ctrl-delete` | to the start / end of the word |
| `ctrl-a`, `ctrl-c`, `ctrl-x`, `ctrl-v` | select all, copy, cut, paste |
| `ctrl-z` / `ctrl-y`, `shift-ctrl-z` | undo / redo |

The Emacs keys (`ctrl-a`, `ctrl-e`, `ctrl-k`, `ctrl-b`, …) are macOS only;
elsewhere `ctrl-a` selects all.
