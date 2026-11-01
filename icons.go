package main

import (
	_ "embed"
	"strings"

	"github.com/egoist/mygo/ui"
)

// logoSVG is the HyperAI mark, as resources/logo.svg draws it in its pink.
//
//go:embed resources/logo.svg
var logoSVG string

// iconLightPNG and iconDarkPNG are the app's icon, light, as the app is
// bundled with (resources/icon.png is it), and dark: the icons the Dock
// shows the app by, as Appearance chooses.
var (
	//go:embed resources/icon-light.png
	iconLightPNG []byte
	//go:embed resources/icon-dark.png
	iconDarkPNG []byte
)

// markIcon is the mark in the text color, as hyperai-next fills its Logo
// with the accent: the light theme's red, the dark one's pink.
var markIcon = ui.MustParseSVG([]byte(strings.Replace(logoSVG, `fill="#FF6096"`, `fill="currentColor"`, 1)))

// icon parses a Lucide icon's shapes, drawn in the text color.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

// titleIconSize is the size of the title bar's icons, by the window's
// controls: smaller than the others, as Waku's.
const titleIconSize = 14

var (
	logoIcon        = icon(`<path d="M9.937 15.5A2 2 0 0 0 8.5 14.063l-6.135-1.582a.5.5 0 0 1 0-.962L8.5 9.936A2 2 0 0 0 9.937 8.5l1.582-6.135a.5.5 0 0 1 .963 0L14.063 8.5A2 2 0 0 0 15.5 9.937l6.135 1.581a.5.5 0 0 1 0 .964L15.5 14.063a2 2 0 0 0-1.437 1.437l-1.582 6.135a.5.5 0 0 1-.963 0z"/>`)
	newChatIcon     = icon(`<path d="M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.375 2.625a1 1 0 0 1 3 3l-9.013 9.014a2 2 0 0 1-.853.505l-2.873.84a.5.5 0 0 1-.62-.62l.84-2.873a2 2 0 0 1 .506-.852z"/>`)
	codeIcon        = icon(`<path d="m16 18 6-6-6-6"/><path d="m8 6-6 6 6 6"/>`)
	planIcon        = icon(`<path d="m3 17 2 2 4-4"/><path d="m3 7 2 2 4-4"/><path d="M13 6h8"/><path d="M13 12h8"/><path d="M13 18h8"/>`)
	ideaIcon        = icon(`<path d="M15 14c.2-1 .7-1.7 1.5-2.5 1-.9 1.5-2.2 1.5-3.5A6 6 0 0 0 6 8c0 1 .2 2.2 1.5 3.5.7.7 1.3 1.5 1.5 2.5"/><path d="M9 18h6"/><path d="M10 22h4"/>`)
	plugIcon        = icon(`<path d="M12 22v-5"/><path d="M9 8V2"/><path d="M15 8V2"/><path d="M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8Z"/>`)
	closeIcon       = icon(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	enterIcon       = icon(`<path d="M20 4v7a4 4 0 0 1-4 4H4"/><path d="m9 10-5 5 5 5"/>`)
	newChatPlusIcon = icon(`<path d="M7.9 20A9 9 0 1 0 4 16.1L2 22Z"/><path d="M8 12h8"/><path d="M12 8v8"/>`)
	folderIcon      = icon(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)
	artifactIcon    = icon(`<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/>`)
	searchIcon      = icon(`<path d="m21 21-4.34-4.34"/><circle cx="11" cy="11" r="8"/>`)
	sidebarIcon     = icon(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/>`)
	arrowLeftIcon   = icon(`<path d="m12 19-7-7 7-7"/><path d="M19 12H5"/>`)
	arrowRightIcon  = icon(`<path d="M5 12h14"/><path d="m12 5 7 7-7 7"/>`)
	arrowUpIcon     = icon(`<path d="m5 12 7-7 7 7"/><path d="M12 19V5"/>`)
	// arrowDownIcon is Tabler's arrow-down, as hyperai-next's agent shows.
	arrowDownIcon    = icon(`<path d="M12 5l0 14"/><path d="M18 13l-6 6"/><path d="M6 13l6 6"/>`)
	stopIcon         = icon(`<rect x="7" y="7" width="10" height="10" rx="1.5" fill="currentColor"/>`)
	copyIcon         = icon(`<rect width="14" height="14" x="8" y="8" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>`)
	checkIcon        = icon(`<path d="M20 6 9 17l-5-5"/>`)
	chevronIcon      = icon(`<path d="m6 9 6 6 6-6"/>`)
	chevronUpIcon    = icon(`<path d="m18 15-6-6-6 6"/>`)
	moreIcon         = icon(`<circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/>`)
	messageIcon      = icon(`<path d="M7.9 20A9 9 0 1 0 4 16.1L2 22Z"/>`)
	settingsIcon     = icon(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>`)
	sunIcon          = icon(`<circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/>`)
	keyboardIcon     = icon(`<path d="M10 8h.01"/><path d="M12 12h.01"/><path d="M14 8h.01"/><path d="M16 12h.01"/><path d="M18 8h.01"/><path d="M6 8h.01"/><path d="M7 16h10"/><path d="M8 12h.01"/><rect width="20" height="16" x="2" y="4" rx="2"/>`)
	infoIcon         = icon(`<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>`)
	instructionsIcon = icon(`<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/><path d="M13 8H7"/><path d="M17 12H7"/>`)
	effortIcon       = icon(`<path d="m12 14 4-4"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/>`)
	usageIcon        = icon(`<path d="M3 3v16a2 2 0 0 0 2 2h16"/><path d="M18 17V9"/><path d="M13 17V5"/><path d="M8 17v-3"/>`)
	pencilIcon       = icon(`<path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z"/><path d="m15 5 4 4"/>`)
	rotateIcon       = icon(`<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/>`)
	chevronLeftIcon  = icon(`<path d="m15 18-6-6 6-6"/>`)
	chevronRightIcon = icon(`<path d="m9 18 6-6-6-6"/>`)
	plusIcon         = icon(`<path d="M5 12h14"/><path d="M12 5v14"/>`)
	paperclipIcon    = icon(`<path d="m16 6-8.414 8.586a2 2 0 0 0 2.829 2.829l8.414-8.586a4 4 0 1 0-5.657-5.657l-8.379 8.551a6 6 0 1 0 8.485 8.485l8.379-8.551"/>`)
	fileTextIcon     = icon(`<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/>`)
)
