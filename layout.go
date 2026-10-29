package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// The sidebar's width is the user's to drag, as Waku's is: from sidebarMin
// to sidebarMax, leaving the conversation paneMin at least; sidebarWidth
// at first, and again on a double click of its edge. It is kept in
// layout.json, beside the chats.
const (
	sidebarMin = 200 // Waku's 180, and room for the title bar's buttons over it
	sidebarMax = 420
	paneMin    = 360
)

// layout is what layout.json holds: the sidebar's width, and the
// appearance and the behavior chosen in the settings.
type layout struct {
	SidebarWidth  float32 `json:"sidebarWidth,omitempty"`
	Theme         string  `json:"theme,omitempty"`
	TextSize      float32 `json:"textSize,omitempty"`
	OpaqueSidebar bool    `json:"opaqueSidebar,omitempty"`
	ReduceMotion  bool    `json:"reduceMotion,omitempty"`
	Font          string  `json:"font,omitempty"`
	CodeFont      string  `json:"codeFont,omitempty"`
	CodeTheme     string  `json:"codeTheme,omitempty"`
	// OnLaunch is "last" to show Chat, the last chat shown, as the app
	// opens; SendWith "cmd" to send with Cmd+Return.
	OnLaunch string `json:"onLaunch,omitempty"`
	Chat     string `json:"chat,omitempty"`
	SendWith string `json:"sendWith,omitempty"`
	// Notify shows a notification as a reply ends in the background.
	Notify bool `json:"notify,omitempty"`
	// Language is the tag of the language chosen, "" for the system's.
	Language string `json:"language,omitempty"`
	// MathSource shows math in replies as its LaTeX, rather than set.
	MathSource bool `json:"mathSource,omitempty"`
}

// loadLayout reads the layout kept in dir, if any.
func (a *app) loadLayout(dir string) {
	a.layoutPath = filepath.Join(dir, "layout.json")
	b, err := os.ReadFile(a.layoutPath)
	if err != nil {
		return
	}
	var l layout
	if json.Unmarshal(b, &l) != nil {
		return
	}
	if l.SidebarWidth > 0 {
		a.sideWidth = l.SidebarWidth
	}
	if slices.Contains(themes[:], l.Theme) {
		a.theme = l.Theme
	}
	if i := slices.Index(textSizes[:], l.TextSize); i >= 0 {
		a.textSize = i
	}
	a.translucent, a.reduceMotion = !l.OpaqueSidebar, l.ReduceMotion
	a.font, a.codeFont = l.Font, l.CodeFont
	if codeThemeNamed(l.CodeTheme) != nil {
		a.codeTheme = l.CodeTheme
	}
	if l.OnLaunch == "last" {
		a.onLaunch = launchLast
	}
	if l.SendWith == "cmd" {
		a.sendWith = sendCmdReturn
	}
	a.keptChat, a.notifyDone, a.renderMath = l.Chat, l.Notify, !l.MathSource
	if slices.ContainsFunc(languages, func(lang struct{ tag, name string }) bool { return lang.tag == l.Language }) {
		a.language = l.Language
	}
}

// saveLayout keeps the layout, where the app has a place for it (not in
// tests), on a goroutine, as save does the chats.
func (a *app) saveLayout() {
	if a.layoutPath == "" {
		return
	}
	l := layout{SidebarWidth: a.sideWidth, Theme: a.theme, TextSize: textSizes[a.textSize],
		OpaqueSidebar: !a.translucent, ReduceMotion: a.reduceMotion,
		Font: a.font, CodeFont: a.codeFont, CodeTheme: a.codeTheme, Chat: a.keptChat, Notify: a.notifyDone, Language: a.language,
		MathSource: !a.renderMath}
	if a.onLaunch == launchLast {
		l.OnLaunch = "last"
	}
	if a.sendWith == sendCmdReturn {
		l.SendWith = "cmd"
	}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	a.layoutSaver.save(a.layoutPath, b)
}

// sidebarBounds returns the widths the sidebar may take in a window w wide
// whose title bar leaves left free of its controls: room for the toggle,
// Back and Forward over it, and the conversation's room beside it.
func sidebarBounds(w, left float32) (lo, hi float32) {
	lo = max(sidebarMin, left+8+30+6+30+2+30+16)
	return lo, max(lo, min(sidebarMax, w-paneMin))
}

// resetSidebar makes the sidebar as wide as at first.
func (a *app) resetSidebar() {
	a.sideWidth = sidebarWidth
	a.saveLayout()
}

// resizer builds the sidebar's right edge, at x, which the pointer drags to
// resize the sidebar between lo and hi, as Waku's: a strip 10 DIPs wide
// over the line, which the pointer turns into a resizing one, and where a
// bar shows while the pointer is over it or drags it. A drag resizes by how
// far the pointer went since it pressed, so that the edge comes back with
// the pointer from past the bounds; it keeps the width as it ends.
func (a *app) resizer(c *ui.Context, x, lo, hi float32) {
	edge := ui.Box(c.Key("resize sidebar")).Absolute().Left(x - 5).Top(0).Bottom(0).Width(10).
		Cursor(ui.CursorResizeEW).Label(locales.ResizeSidebar())
	// Pressing, wherever the pointer is: Pressed tells only while the
	// pointer is over the edge, which it leaves as it drags.
	dx, _, pressing := edge.Dragged()
	switch {
	case pressing:
		if !a.resizing {
			a.resizing, a.resizeFrom, a.resizeBy = true, max(lo, min(a.sideWidth, hi)), 0
		}
		a.resizeBy += dx
		a.sideWidth = max(lo, min(a.resizeFrom+a.resizeBy, hi))
	case a.resizing:
		a.resizing = false
		a.saveLayout()
	}
	if edge.DoubleClicked() {
		a.resetSidebar()
	}
	show := a.resizing || edge.Hovered()
	edge.Children(func() {
		bar := ui.Box(c).Absolute().Left(4).Top(0).Bottom(0).Width(2)
		if show {
			// fg, as the line it lies over, but darker: the accent, tried
			// here, stood apart from the sidebar.
			bar.Background(a.pal.fg.Alpha(0.35))
		}
	})
}
