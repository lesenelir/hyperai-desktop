package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// errNotificationsOff is what notify's failed gets where the system does
// not let the app show notifications (mygo.ErrNotificationsDenied).
var errNotificationsOff = errors.New("notifications are off")

// notifyReply shows a notification that reply in th ended, where General
// says to and the window is in the background: the chat's title over the
// reply's start, or what went wrong, which shows the chat as it is
// clicked. Where notifications are off, the setting turns off too, and
// says why.
func (a *app) notifyReply(th *thread, reply *message) {
	if !a.notifyDone || a.notify == nil || a.focused == nil || a.focused() {
		return
	}
	body := reply.previewText()
	if reply.Error != "" {
		body = "The reply failed: " + reply.Error
	}
	if r := []rune(body); len(r) > 120 {
		body = string(r[:120]) + "…"
	}
	id := strconv.Itoa(th.ID)
	a.notify(th.Title, body, func() { a.open(id) }, func(err error) {
		if errors.Is(err, errNotificationsOff) {
			a.notifyDone, a.notifyError = false, "Turned off: notifications from HyperAI are off in System Settings."
		} else {
			a.notifyError = "The last could not show: " + err.Error()
		}
		a.saveLayout()
	})
}

// What the General page sets beyond the switch of Open at login: the chat
// shown as the app opens, of launchTitles, and the key that sends, of
// sendTitles, Return or Cmd+Return, the other breaking the line.
const (
	launchNew  = 0
	launchLast = 1

	sendReturn    = 0
	sendCmdReturn = 1
)

var launchTitles = [...]string{"New chat", "Last chat"}

// cmdReturnKeys send a draft where the settings say Cmd+Return does.
var cmdReturnKeys = keys{[]string{"⌘", "Return"}, []string{"Ctrl", "Enter"}}

// sendTitles name the keys that send, as the segmented control shows them.
var sendTitles = [...]string{returnKey.String(), onMac("⌘ Return", "Ctrl+Enter")}

// sends reports whether k sends the draft, by the key the settings chose:
// Return alone, or Cmd+Return. The other Return breaks the line.
func (a *app) sends(k ui.Key, mods ui.Modifiers) bool {
	if k != ui.KeyEnter {
		return false
	}
	if a.sendWith == sendCmdReturn {
		return mods == ui.Cmd
	}
	return mods == 0
}

// generalTerms returns what the General page holds, as the settings'
// field finds it: its rows' titles, and what they offer.
func (a *app) generalTerms() []settingsTerm {
	return []settingsTerm{
		{"Open at login", []string{"Startup", "Login items", "Launch"}},
		{"On launch", append([]string{"Startup", "Open"}, launchTitles[:]...)},
		{"Send with", append([]string{"Send", "Enter", "New line"}, sendTitles[:]...)},
		{"Notify when a reply finishes", []string{"Notifications", "Background", "Replies"}},
		{chatsSavedTitle, []string{"Chats", "Finder", "Folder", "Data", "chats.json"}},
		{"Delete all chats", []string{"Delete", "Remove", "Clear", "History"}},
	}
}

// chatsSavedTitle names the row that says where the chats are kept.
var chatsSavedTitle = onMac("Saved on this Mac", "Saved on this computer")

// generalPage builds the General page: how HyperAI starts, the key that
// sends, the notification of a reply, and where the chats are kept, and
// deleting them.
func (a *app) generalPage(c *ui.Context) {
	a.settingsCard(c, "Startup", func() {
		about := "Start HyperAI as you log in."
		if a.loginError != "" {
			about = "Could not change it: " + a.loginError
		}
		a.settingRow(c, "Open at login", about, func() {
			// Off, and still, where the app cannot set it (in tests).
			on, none := &a.atLogin, false
			if a.setOpenAtLogin == nil {
				on = &none
			}
			if ui.Switch(c.Key("open at login"), on).Label("Open at login").Disabled(a.setOpenAtLogin == nil).Changed() {
				a.loginError = ""
				if err := a.setOpenAtLogin(a.atLogin); err != nil {
					a.atLogin, a.loginError = !a.atLogin, err.Error()
				}
			}
		})
		settingDivider(c)
		a.settingRow(c, "On launch", "The chat HyperAI shows as it opens.", func() {
			a.choiceButton(c, "On launch", &a.onLaunch, launchTitles[:])
		})
	})
	a.settingsCard(c, "Composer", func() {
		a.settingRow(c, "Send with", "The other breaks the line, as Shift with either does.", func() {
			a.choiceButton(c, "Send with", &a.sendWith, sendTitles[:])
		})
	})
	a.settingsCard(c, "Replies", func() {
		about := "While HyperAI is in the background. Clicking it shows the chat."
		if a.notifyError != "" {
			about = a.notifyError
		}
		a.settingRow(c, "Notify when a reply finishes", about, func() {
			if ui.Switch(c.Key("notify"), &a.notifyDone).Label("Notify when a reply finishes").Changed() {
				a.notifyError = ""
				a.saveLayout()
			}
		})
	})
	a.settingsCard(c, "Chats", func() {
		about := "Not saved: this is a demo."
		if a.path != "" {
			about = homeRelative(a.path)
		}
		a.settingRow(c, chatsSavedTitle, about, func() {
			if a.path != "" && a.reveal != nil && a.settingButton(c, onMac("Show in Finder", "Show in Folder"), false) {
				// The folder, where no chat was written yet.
				target := a.path
				if _, err := os.Stat(target); err != nil {
					target = filepath.Dir(target)
				}
				a.reveal(target)
			}
		})
		settingDivider(c)
		about = "There are none."
		switch n := len(a.threads); n {
		case 0:
		case 1:
			about = "The one chat, and its messages. HyperAI asks first."
		default:
			about = "All " + strconv.Itoa(n) + " chats, and their messages. HyperAI asks first."
		}
		a.settingRow(c, "Delete all chats", about, func() {
			if a.settingButton(c, "Delete All…", len(a.threads) == 0) {
				a.askDelete(nil)
			}
		})
	})
}

// choiceButton builds the pop-up button, labeled label, that chooses
// *chosen of titles, as the fonts' do: the one chosen shown, and checked in
// the system's menu of them. A choice is kept for the next start.
func (a *app) choiceButton(c *ui.Context, label string, chosen *int, titles []string) {
	a.popUpButton(c, label, titles[*chosen], func() []string { return titles }, func(m *ui.Menu) {
		for i, title := range titles {
			if m.Item(title).Checked(i == *chosen).Chosen() && i != *chosen {
				*chosen = i
				a.saveLayout()
			}
		}
	})
}

// homeRelative returns path from the home directory as ~ when it is in it.
func homeRelative(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rel, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return filepath.Join("~", rel)
	}
	return path
}

// settingButton builds a button of a settings row, labeled label, as the
// pop-up buttons look, faded where disabled: it reports a click.
func (a *app) settingButton(c *ui.Context, label string, disabled bool) bool {
	t := c.Theme()
	btn := ui.ButtonBase(c.Key(label)).Padding(5, 10).Radius(8).Background(a.pal.composer).Border(1, t.Border).
		Label(label).Disabled(disabled)
	switch {
	case disabled:
		btn.Opacity(0.35)
	case btn.Hovered():
		btn.Background(a.pal.fg.Alpha(0.04).Over(a.pal.composer))
	}
	btn.Children(func() { ui.Text(c, label).SingleLine() })
	return btn.Clicked()
}
