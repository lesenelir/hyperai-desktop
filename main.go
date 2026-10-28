package main

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The Dock icon follows the appearance while the app runs; Finder and the
// disk image show resources/icon.png, the light one.
var (
	//go:embed resources/icon-light.png
	iconLight []byte
	//go:embed resources/icon-dark.png
	iconDark []byte
)

func updateIcon() {
	icon := iconLight
	if mygo.Theme.IsDark() {
		icon = iconDark
	}
	if err := mygo.App.Dock.SetIcon(icon); err != nil {
		log.Print("dock icon: ", err)
	}
}

// view builds the interface on the main thread whenever the window needs a
// frame.
func view(c *ui.Context) {
	ui.Column(c).Fill().Center().Children(func() {
		ui.Text(c, "Hello, world!").FontSize(26).Bold()
	})
}

func main() {
	mygo.App.WhenReady(func() {
		updateIcon()
		mygo.Theme.OnUpdated(updateIcon)
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "hyperai-desktop",
			Width:     480,
			Height:    360,
			MinWidth:  320,
			MinHeight: 280,
			// Opens where the user left it last time.
			StateKey: "main",
			// The window shows the interface MyGo draws, not a web page.
			Content: ui.View(view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
