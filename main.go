// HyperAI is an AI chat client in native UI: chats in a sidebar, replies
// streamed in as Markdown, and a composer that asks the model chosen. It
// talks to models through OpenRouter when OPENROUTER_API_KEY is set, in
// the environment or a .env file, and to a pretend model otherwise.
package main

import (
	"log"
	"math/rand/v2"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
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
		if bench > 0 {
			go a.bench(bench)
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
