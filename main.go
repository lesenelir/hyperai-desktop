// HyperAI is an AI chat client in native UI: chats in a sidebar, replies
// streamed in as Markdown, and a composer that asks the model chosen. It
// talks to the providers the user set up on the Providers page: those
// they gave an API key (OpenAI, Anthropic, Google, OpenRouter, DeepSeek),
// the servers on their computer they turned on (Ollama, LM Studio), and
// those they added; until one is, the composer asks for one.
package main

import (
	"context"
	"errors"
	"log"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"hyperai-desktop/ratex"
)

func main() {
	mygo.App.WhenReady(func() {
		bench := benchReplies()
		var a *app
		if bench > 0 {
			// Measured against the demo, paced the same every time, without
			// the user's chats.
			a = newApp(&demo{paced: true, rng: rand.New(rand.NewPCG(1, 2))})
		} else {
			a = newApp(nil)
			// The Copilot CLI, where it is installed, to ask Copilot through.
			a.copilot.cli = findCopilotCLI()
			a.load()
			// The API keys: from the system's credential store, or a file
			// for a development build.
			var k keyring
			k, a.keysFile = appKeyring()
			a.loadKeys(k)
			// The theme chosen, before the window opens in another.
			a.applyTheme = func(theme string) { mygo.Theme.SetSource(mygo.ThemeSource(theme)) }
			a.applyTheme(a.theme)
			// The Dock's icon chosen, where it is not the app's own, which
			// the Dock shows already.
			a.applyDockIcon = func(png []byte) {
				if err := mygo.App.Dock.SetIcon(png); err != nil {
					log.Printf("setting the Dock icon: %v", err)
				}
			}
			if a.dockIcon != dockIcons[0].name {
				a.applyDockIcon(dockIconPNG(a.dockIcon))
			}
			// The language chosen MyGo keeps itself, and shows from the
			// start: the app only changes it.
			a.applyLanguage = mygo.App.SetLocale
			a.version = mygo.App.Version()
			a.openAtLogin, a.setOpenAtLogin = mygo.App.OpenAtLogin, mygo.App.SetOpenAtLogin
			a.reveal = mygo.Shell.ShowItemInFolder
		}
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "HyperAI",
			Width:     1120,
			Height:    760,
			MinWidth:  720,
			MinHeight: 480,
			// Opens where the user left it last time.
			StateKey: "main",
			// Hide the title bar and inset the traffic lights over the
			// sidebar (macOS); elsewhere the window controls sit in the
			// view's title bar.
			TitleBarStyle:  mygo.TitleBarHiddenInset,
			TitleBarHeight: titleBarHeight,
			// The sidebar shows the desktop's material where it can (macOS,
			// Windows 11), and draws a background of its own elsewhere.
			Vibrancy: mygo.VibrancySidebar,
			Content:  ui.View(a.view),
		})
		a.post = a.win.Update
		// Files to attach, as the system's dialog picks them, over the
		// window; and those sent, opened in the apps the system opens them in.
		win := a.win
		a.openFiles = func(title string) []string {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: title, Multiple: true})
			if err != nil {
				log.Printf("attaching files: %v", err)
			}
			return paths
		}
		// Chats exported, where the system's dialog says, from Documents;
		// as a PDF, printed by the system's web view.
		a.exportDir, _ = mygo.App.Path(mygo.PathDocuments)
		a.saveFile = func(title, path, ext string) string {
			to, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: win, Title: title, DefaultPath: path,
				Filters: []mygo.FileFilter{{Name: strings.ToUpper(ext), Extensions: []string{ext}}}})
			if err != nil {
				log.Printf("exporting a chat: %v", err)
			}
			return to
		}
		a.printPDF = func(page string) ([]byte, error) { return printPDF(win, page) }
		a.openPath = func(path string) {
			if err := mygo.Shell.OpenPath(path); err != nil {
				log.Printf("opening an attachment: %v", err)
			}
		}
		startTeX(a)
		if bench > 0 {
			go a.bench(bench)
		} else {
			a.focused = a.win.IsFocused
			a.nameChats = true
			// The menu bar's menus, with Settings…, and HyperAI's icon in
			// it, both speaking the app's language; and the app out of the
			// Dock, where the settings say.
			if hasDock {
				mygo.App.SetMenu(appMenu(a))
			}
			tray := newMenuBar(a)
			a.applyShowDock = func(show bool) { applyDock(a, tray, show) }
			if !a.showDock {
				a.applyShowDock(false)
			}
			setLocale := a.applyLanguage
			a.applyLanguage = func(tag string) {
				setLocale(tag)
				if hasDock {
					mygo.App.SetMenu(appMenu(a))
				}
				if tray != nil {
					tray.SetMenu(menuBarMenu(a))
				}
			}
			// The models of the providers that listed none yet, as they
			// have them now, and whom Copilot asks as.
			a.listUnlisted()
			if a.copilotSignedIn() {
				a.checkCopilot(false)
			}
			a.notify = func(title, body string, clicked func(), failed func(error)) {
				notifyReply(a.win, title, body, clicked, failed)
			}
		}
		// The chats are written on a goroutine: let the last write end.
		mygo.App.OnQuit(func() {
			// The draft of the chat shown, kept with it.
			a.keepDraft()
			a.saver.wait()
			a.layoutSaver.wait()
			a.confSaver.wait()
			a.writes.Wait()
			// The Copilot CLI, if it runs, with it.
			a.closeCopilot()
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// startTeX starts RaTeX, which lays math out as KaTeX does, for a's
// replies: on a goroutine, as compiling it takes a second the first time,
// and keeps what it compiled in the cache, for the next launches to start
// it in tens of milliseconds. Until it is ready, and where it cannot
// start, the app sets math as text.
func startTeX(a *app) {
	cache := ""
	if dir, err := mygo.App.Path(mygo.PathCache); err == nil {
		cache = filepath.Join(dir, "ratex-"+ratex.Version)
	}
	go func() {
		e, err := ratex.New(context.Background(), cache)
		if err != nil {
			log.Printf("math: %v", err)
			return
		}
		a.win.Update(func() { a.tex = e })
	}()
}

// printPDF prints page, a whole HTML document, as an A4 PDF, its
// backgrounds too: in a window that never shows, made on the main thread
// through win, which loads it and then prints it, and goes. It waits, on
// the goroutine it is called on, for the page to load, at most a while.
func printPDF(win *mygo.Window, page string) ([]byte, error) {
	loaded := make(chan error, 1)
	done := func(err error) {
		select {
		case loaded <- err:
		default:
		}
	}
	made := make(chan *mygo.Window, 1)
	win.Update(func() {
		w := mygo.NewWindow(mygo.WindowOptions{Hidden: true, Width: 800, Height: 1100})
		w.Page().OnDidFinishLoad(func() { done(nil) })
		w.Page().OnDidFailLoad(func(e *mygo.LoadError) { done(e) })
		w.Page().LoadHTML(page, "")
		made <- w
	})
	w := <-made
	defer win.Update(w.Destroy)
	select {
	case err := <-loaded:
		if err != nil {
			return nil, err
		}
	case <-time.After(30 * time.Second):
		return nil, errors.New("the page to print did not load")
	}
	return w.Page().PrintToPDF(mygo.PDFOptions{PageSize: mygo.PageA4, Background: true})
}

// notifyReply shows a notification titled title, saying body, which brings
// win to the front and calls clicked as it is clicked; failed hears why it
// could not show, errNotificationsOff where the user does not allow them.
// Both run on the main thread. Showing waits for the system, and the first
// time for the user to allow notifications, so it waits on a goroutine.
func notifyReply(win *mygo.Window, title, body string, clicked func(), failed func(error)) {
	n := mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body, Group: "replies"})
	n.OnClick(func() {
		win.Update(func() {
			mygo.App.Focus()
			win.Show()
			win.Focus()
			clicked()
		})
	})
	go func() {
		err := n.Show()
		if errors.Is(err, mygo.ErrNotificationsDenied) {
			err = errNotificationsOff
		}
		if err != nil {
			win.Update(func() { failed(err) })
		}
	}()
}
