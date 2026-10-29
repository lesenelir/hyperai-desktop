// HyperAI is an AI chat client in native UI: chats in a sidebar, replies
// streamed in as Markdown, and a composer that asks the model chosen. It
// talks to models through OpenRouter when OPENROUTER_API_KEY is set, in
// the environment or a .env file, and to a pretend model otherwise.
package main

import (
	"context"
	"errors"
	"log"
	"math/rand/v2"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"hyperai-desktop/ratex"
)

func main() {
	mygo.App.WhenReady(func() {
		// .env files set what the environment leaves unset: the project's,
		// where `mygo dev` runs the app, then the one in the user data
		// directory, which an app opened from the Finder, without the
		// shell's environment, reads too.
		envFiles := []string{".env"}
		if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
			envFiles = append(envFiles, filepath.Join(dir, ".env"))
		}
		loadEnv(envFiles...)
		bench := benchReplies()
		var a *app
		if bench > 0 {
			// Measured against the demo, paced the same every time, without
			// the user's chats.
			a = newApp(&demo{paced: true, rng: rand.New(rand.NewPCG(1, 2))})
		} else {
			a = newApp(newProvider())
			a.load()
			// The theme chosen, before the window opens in another.
			a.applyTheme = func(theme string) { mygo.Theme.SetSource(mygo.ThemeSource(theme)) }
			a.applyTheme(a.theme)
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
		startTeX(a)
		if bench > 0 {
			go a.bench(bench)
		} else {
			a.focused = a.win.IsFocused
			a.notify = func(title, body string, clicked func(), failed func(error)) {
				notifyReply(a.win, title, body, clicked, failed)
			}
		}
		// The chats are written on a goroutine: let the last write end.
		mygo.App.OnQuit(func() {
			a.saver.wait()
			a.layoutSaver.wait()
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
