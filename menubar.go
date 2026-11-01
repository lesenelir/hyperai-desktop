package main

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"

	"hyperai-desktop/locales"
)

// trayPNG is the mark for the menu bar, black on clear, which macOS tints
// as it does its own icons: 18 points, the mark 16 of them, as
// resources/logo.svg draws it.
//
//go:embed resources/trayTemplate.png
var trayPNG []byte

// newMenuBar shows HyperAI in the menu bar (macOS), whose menu brings the
// window to the front, starts a new chat, or quits: the way to the app
// while the Dock does not show it. It returns nil where it could not show,
// and elsewhere than macOS, whose taskbars show the window already.
func newMenuBar(a *app) *mygo.Tray {
	if !hasDock {
		return nil
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: "main", Icon: trayPNG, IconIsTemplate: true,
		ToolTip: "HyperAI", Menu: menuBarMenu(a)})
	if err != nil {
		log.Printf("menu bar: %v", err)
		return nil
	}
	return tray
}

// menuBarMenu returns the menu bar's menu, in the app's language.
func menuBarMenu(a *app) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.MenuBarOpen(), Click: func(*mygo.MenuItem, *mygo.Window) { toFront(a) }},
		{Label: locales.NewChat(), Click: func(*mygo.MenuItem, *mygo.Window) { menuNewChat(a) }},
		mygo.Separator(),
		{Label: locales.MenuBarQuit(), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

// toFront brings the app and its window to the front, from a menu.
func toFront(a *app) {
	mygo.App.Focus()
	a.win.Show()
	a.win.Focus()
}

// menuNewChat shows a new chat, in front, as a menu chose.
func menuNewChat(a *app) {
	toFront(a)
	a.win.Update(a.newChat)
}

// appMenu returns the app's menu bar (macOS): MyGo's, with Settings… in
// the app's menu, as macOS apps have it, which settingsKeys choose, as
// they do in the window, New chat in File, which ⌘N chooses, and Help,
// with the system's search field, the keyboard shortcuts and the About
// page's links. Chosen while the window is in the background, Settings…
// shows the settings rather than close them.
func appMenu(a *app) *mygo.Menu {
	settings := func(*mygo.MenuItem, *mygo.Window) {
		shown := a.win.IsFocused()
		toFront(a)
		a.win.Update(func() {
			if shown {
				a.toggleSettings()
			} else {
				a.openSettings()
			}
		})
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			mygo.Separator(),
			{Label: locales.Settings() + "…", Accelerator: "CmdOrCtrl+,", Click: settings},
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleFileMenu, Submenu: []*mygo.MenuItem{
			{Label: locales.NewChat(), Accelerator: "CmdOrCtrl+N", Click: func(*mygo.MenuItem, *mygo.Window) { menuNewChat(a) }},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu},
		{Role: mygo.RoleWindowMenu},
		{Role: mygo.RoleHelp, Submenu: []*mygo.MenuItem{
			{Label: locales.PageShortcuts(), Click: func(*mygo.MenuItem, *mygo.Window) {
				toFront(a)
				a.win.Update(func() {
					a.openSettings()
					a.settingsPage = "shortcuts"
				})
			}},
			mygo.Separator(),
			{Label: locales.HelpWebsite(), Click: func(*mygo.MenuItem, *mygo.Window) { openLink(websiteURL) }},
			{Label: ai4sTitle, Click: func(*mygo.MenuItem, *mygo.Window) { openLink(ai4sURL) }},
		}},
	})
}

// openLink opens url in the browser, from a menu.
func openLink(url string) {
	if err := mygo.Shell.OpenExternal(url); err != nil {
		log.Printf("opening %s: %v", url, err)
	}
}

// setShowDock shows the app in the Dock, or in the menu bar alone, as the
// app does from then on.
func (a *app) setShowDock(show bool) {
	if show == a.showDock {
		return
	}
	a.showDock = show
	if a.applyShowDock != nil {
		a.applyShowDock(show)
	}
	a.saveLayout()
}

// applyDock shows the app in the Dock, by the icon chosen, or takes it out,
// tray being where the app is found then: where there is none, the Dock
// keeps it. Its window stays in front, as macOS sends an app that leaves
// the Dock behind the others.
func applyDock(a *app, tray *mygo.Tray, show bool) {
	if !show && tray == nil {
		return
	}
	if show {
		mygo.App.Dock.Show()
		if a.dockIcon != dockIcons[0].name {
			a.applyDockIcon(dockIconPNG(a.dockIcon))
		}
	} else {
		mygo.App.Dock.Hide()
	}
	mygo.App.Focus()
	a.win.Focus()
}
