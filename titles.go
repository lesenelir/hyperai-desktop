package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// A chat is named at once of its first message (title), and then, as
// docs/titles.md has it, by a model, which a few words of what the chat is
// about name better than its first 48 runes: asked as the message is sent,
// beside the reply, of the provider the chat asks, on its cheap model.

// titlePrompt asks for a chat's title, the chat's first message after it,
// closed by titlePromptEnd.
const (
	titlePrompt = "Write a title for a chat that begins with the message below. " +
		"Reply with the title alone: at most six words, in the language of the message, " +
		"with no quotes and no trailing period. Do not answer the message.\n\n<message>\n"
	titlePromptEnd = "\n</message>"
)

// titleTimeout is how long a title may take: past it, one is no use, as the
// user has moved on.
const titleTimeout = 30 * time.Second

// titleMaxRunes is how long what a model says may be, at most, to be a
// title: a longer one is an answer to the message, not a name for it.
const titleMaxRunes = 80

// nameChat asks for a title of the chat th, which first was sent first,
// as it starts, or as the user asked for one (explicit), whose failure the
// window says (titleError):
// of the model the Default model page chose (app.titleModel), where it is
// still offered, else of the chat's provider p, on the cheap model of its
// kind (titleModel), else model, the chat's; and of the chat's model again
// where the other fails, which sends first nowhere it had not gone. The
// title lands on the main thread (titled); one that fails leaves the chat
// as it is. th.naming tells one is asked for, until it lands.
func (a *app) nameChat(th *thread, p provider, model, first string, explicit bool) {
	tp, cheap := p, model
	if chosen := a.providerOf(a.titleModel); chosen != nil {
		tp, cheap = chosen, a.titleModel.Model
	} else if kind := a.kindOf(p.ID()); kind != nil && kind.titleModel != "" {
		cheap = kind.titleModel
	}
	id, local := th.ID, th.Title
	th.naming = true
	a.async(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), titleTimeout)
		defer cancel()
		name, err := askTitle(ctx, tp, cheap, first)
		if err != nil && (tp != p || cheap != model) && !errors.Is(err, context.DeadlineExceeded) {
			name, err = askTitle(ctx, p, model, first)
		}
		return func() {
			if th := a.thread(strconv.Itoa(id)); th != nil {
				th.naming = false
			}
			switch {
			case err == nil:
				a.titled(id, local, name)
			case explicit:
				a.titleError = err.Error()
			}
		}
	})
}

// retitle asks for th's title anew, as Regenerate Title does, of its first
// question, as a model is asked as it starts (nameChat), whatever its
// title, a name the user gave it too: of the provider and model it asks,
// which the Default model page's title model goes before. Nothing where it
// has no question with text, none can be asked, or one is being asked for.
func (a *app) retitle(th *thread) {
	if !a.canRetitle(th) {
		return
	}
	p, model := a.modelOf(th)
	a.nameChat(th, p, model, firstQuestion(th), true)
}

// canRetitle reports whether th's title can be asked for anew (retitle).
func (a *app) canRetitle(th *thread) bool {
	p, _ := a.modelOf(th)
	return p != nil && !th.naming && firstQuestion(th) != ""
}

// firstQuestion returns the text of th's first question with any, "" for
// none: of files alone.
func firstQuestion(th *thread) string {
	for _, m := range th.Messages {
		if m.Role == "user" && strings.TrimSpace(m.Text) != "" {
			return m.Text
		}
	}
	return ""
}

// startRename turns th's title into a field that renames it, where the
// user asked: its row of the sidebar (inSidebar), or the title bar. The
// field holds the title, chosen, to be typed over; Return or clicking
// away keeps what it holds, as the Finder's names do, and Escape what was.
// A chat renamed keeps its name: a title a model was asked for before
// lands on the title it was asked to replace alone (titled).
func (a *app) startRename(th *thread, inSidebar bool) {
	a.commitRename()
	a.renaming, a.renameDraft, a.renameInSidebar = strconv.Itoa(th.ID), th.Title, inSidebar
	a.renameSelect, a.renameFocused, a.renameBuilt = true, false, true
}

// commitRename names the chat being renamed what its field holds, its
// spaces collapsed as a title's (title), and ends the renaming: nothing
// for a field left empty, which keeps the name.
func (a *app) commitRename() {
	if a.renaming == "" {
		return
	}
	th, name := a.thread(a.renaming), title(a.renameDraft)
	a.renaming = ""
	if th == nil || name == "" || name == th.Title {
		return
	}
	th.Title = name
	a.rev++
	a.save()
}

// renameField builds the field that renames the chat, in place of its
// title, at size and weight: on the composer's face in a hairline, out
// past the title by its padding, so that the text stays where the
// title's was. It keeps what it holds as Return is pressed or it loses
// the focus, and ends without as Escape is.
func (a *app) renameField(c *ui.Context, size float32, weight int) ui.Element {
	t := c.Theme()
	field := ui.TextInputBase(c.Key("rename "+a.renaming), &a.renameDraft).Grow(1).MinWidth(0).
		FontSize(size).FontWeight(weight).Padding(3, 6).Margin(-4, -7).Radius(6).
		Background(a.pal.composer).Border(1, t.Border).Label(locales.RenameLabel()).AutoFocus().
		OnTextKey(func(k ui.TextKey) bool {
			switch {
			case k.Key == ui.KeyEscape:
				a.renaming = ""
			case k.Key == ui.KeyEnter && k.Mods == 0:
				a.commitRename()
			default:
				return false
			}
			return true
		})
	a.renameBuilt = true
	if a.renameSelect {
		// Chosen from its end to its start, where the caret is, for a long
		// title to show from its start, as the Finder's names do.
		field.Focus().SetTextSelection(utf8.RuneCountInString(a.renameDraft), 0)
		a.renameSelect = false
	}
	// Clicking away, or another key taking the focus.
	focused := field.Focused()
	if a.renameFocused && !focused {
		a.commitRename()
	}
	a.renameFocused = focused
	return field
}

// renameEnd ends the renaming as the frame ends, keeping what the field
// holds, where its field was not built: its row scrolled away, the sidebar
// hidden, the settings shown in its place.
func (a *app) renameEnd() {
	if a.renaming != "" && !a.renameBuilt {
		a.commitRename()
	}
	a.renameBuilt = false
}

// titleToast says, in a toast, why the title asked for anew could not be
// made, as the provider said it.
func (a *app) titleToast(c *ui.Context) {
	if a.titleError == "" {
		return
	}
	c.AddToast(ui.Toast{ID: "title failed", Title: locales.TitleFailed(), Description: a.titleError})
	a.titleError = ""
}

// askTitle asks model of p for the title of a chat that begins with first,
// as normalizeTitle makes it; an error where it gave none.
func askTitle(ctx context.Context, p provider, model, first string) (string, error) {
	var b strings.Builder
	err := p.Stream(ctx, model, []turn{{Role: "user", Content: titlePrompt + first + titlePromptEnd}}, func(ch chunk) {
		b.WriteString(ch.Text) // what it reasoned is not the title
	})
	if err != nil {
		return "", err
	}
	name := normalizeTitle(b.String())
	if name == "" {
		return "", errors.New("no title")
	}
	return name, nil
}

// titled names the chat id name, where it is still there and still has its
// local title, local: not where it was deleted meanwhile, or named
// otherwise. The sidebar and the search show it at once, and the chats are
// saved, for it to stay.
func (a *app) titled(id int, local, name string) {
	th := a.thread(strconv.Itoa(id))
	if th == nil || th.Title != local || name == local {
		return
	}
	th.Title = name
	a.rev++
	a.save()
}

// normalizeTitle returns what a model said as a title: its first line,
// without the label, Markdown and quotes models put round it, nor a
// trailing period, cut as the local title is; "" where nothing is left,
// or it is too long to be a title.
func normalizeTitle(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	// Its Markdown and quotes, then its label, then those inside it:
	// **Title:** "…".
	s = trimDecoration(s)
	for _, label := range []string{"Title:", "title:", "标题：", "标题:", "タイトル：", "タイトル:", "제목:"} {
		s = strings.TrimPrefix(s, label)
	}
	// And a period after them, or in them: 「…」。, "….".
	for prev := ""; s != prev; {
		prev = s
		s = trimDecoration(s)
		s = strings.TrimRightFunc(s, func(r rune) bool { return r == '.' || r == '。' || unicode.IsSpace(r) })
	}
	if s == "" || len([]rune(s)) > titleMaxRunes {
		return ""
	}
	return title(s)
}

// trimDecoration trims the heading marks, emphasis and quotes round s.
func trimDecoration(s string) string {
	s = strings.TrimLeft(s, "# ")
	return strings.Trim(s, " *_`\"'“”‘’「」『』«»„")
}
