package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
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
		body = locales.NotifyFailed(reply.errorText())
	}
	if r := []rune(body); len(r) > 120 {
		body = string(r[:120]) + "…"
	}
	id := strconv.Itoa(th.ID)
	a.notify(th.Title, body, func() { a.open(id) }, func(err error) {
		if errors.Is(err, errNotificationsOff) {
			a.notifyDone, a.notifyError = false, locales.NotifyOff()
		} else {
			a.notifyError = locales.NotifyLastFailed(err.Error())
		}
		a.saveLayout()
	})
}

// What the General page sets beyond the language and the switch of Open
// at login: the chat shown as the app opens, of launchTitles, and the key
// that sends, of sendTitles, Return or Cmd+Return, the other breaking the
// line.
const (
	launchNew  = 0
	launchLast = 1

	sendReturn    = 0
	sendCmdReturn = 1
)

// launchTitles name the chats On launch chooses of, in the app's language.
func launchTitles() []string { return []string{locales.NewChat(), locales.OnLaunchLast()} }

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
	langs := terms(locales.LanguageTerms())
	for _, l := range languages {
		langs = append(langs, l.name)
	}
	return []settingsTerm{
		{locales.Language(), langs},
		{locales.OpenAtLogin(), terms(locales.OpenAtLoginTerms())},
		{locales.OnLaunch(), append(terms(locales.OnLaunchTerms()), launchTitles()...)},
		{locales.SendWith(), append(terms(locales.SendWithTerms()), sendTitles[:]...)},
		{locales.Notify(), terms(locales.NotifyTerms())},
		{locales.ChatsSaved(platform()), terms(locales.ChatsTerms())},
		{locales.DeleteAll(), terms(locales.DeleteAllTerms())},
	}
}

// generalPage builds the General page: the app's language, how HyperAI
// starts, the key that sends, the notification of a reply, and where the
// chats are kept, and deleting them.
func (a *app) generalPage(c *ui.Context) {
	a.settingsCard(c, locales.LanguageCard(), func() {
		a.settingRow(c, locales.Language(), locales.LanguageAbout(), func() { a.languageButton(c) })
	})
	a.settingsCard(c, locales.Startup(), func() {
		about := locales.OpenAtLoginAbout()
		if a.loginError != "" {
			about = locales.OpenAtLoginFailed(a.loginError)
		}
		a.settingRow(c, locales.OpenAtLogin(), about, func() {
			// Off, and still, where the app cannot set it (in tests).
			on, none := &a.atLogin, false
			if a.setOpenAtLogin == nil {
				on = &none
			}
			if ui.Switch(c.Key("open at login"), on).Label(locales.OpenAtLogin()).Disabled(a.setOpenAtLogin == nil).Changed() {
				a.loginError = ""
				if err := a.setOpenAtLogin(a.atLogin); err != nil {
					a.atLogin, a.loginError = !a.atLogin, err.Error()
				}
			}
		})
		settingDivider(c)
		a.settingRow(c, locales.OnLaunch(), locales.OnLaunchAbout(), func() {
			a.choiceButton(c, locales.OnLaunch(), &a.onLaunch, launchTitles())
		})
	})
	a.settingsCard(c, locales.Composer(), func() {
		a.settingRow(c, locales.SendWith(), locales.SendWithAbout(), func() {
			a.choiceButton(c, locales.SendWith(), &a.sendWith, sendTitles[:])
		})
	})
	a.settingsCard(c, locales.Replies(), func() {
		about := locales.NotifyAbout()
		if a.notifyError != "" {
			about = a.notifyError
		}
		a.settingRow(c, locales.Notify(), about, func() {
			if ui.Switch(c.Key("notify"), &a.notifyDone).Label(locales.Notify()).Changed() {
				a.notifyError = ""
				a.saveLayout()
			}
		})
	})
	a.settingsCard(c, locales.Chats(), func() {
		about := locales.ChatsDemo()
		if a.path != "" {
			about = homeRelative(a.path)
		}
		a.settingRow(c, locales.ChatsSaved(platform()), about, func() {
			if a.path != "" && a.reveal != nil && a.settingButton(c, "show in folder", locales.ShowInFolder(platform()), false) {
				// The folder, where no chat was written yet.
				target := a.path
				if _, err := os.Stat(target); err != nil {
					target = filepath.Dir(target)
				}
				a.reveal(target)
			}
		})
		settingDivider(c)
		a.settingRow(c, locales.DeleteAll(), locales.DeleteAllRow(len(a.threads)), func() {
			if a.settingButton(c, "delete all", locales.DeleteAllButton(), len(a.threads) == 0) {
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

// settingButton builds a button of a settings row, keyed key and labeled
// label, as the pop-up buttons look, faded where disabled: it reports a
// click.
func (a *app) settingButton(c *ui.Context, key, label string, disabled bool) bool {
	t := c.Theme()
	btn := ui.ButtonBase(c.Key(key)).Padding(5, 10).Radius(8).Background(a.pal.composer).Border(1, t.Border).
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
