package main

import "github.com/egoist/mygo/ui"

// icon parses a Lucide icon's shapes, drawn in the text color.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	logoIcon    = icon(`<path d="M9.937 15.5A2 2 0 0 0 8.5 14.063l-6.135-1.582a.5.5 0 0 1 0-.962L8.5 9.936A2 2 0 0 0 9.937 8.5l1.582-6.135a.5.5 0 0 1 .963 0L14.063 8.5A2 2 0 0 0 15.5 9.937l6.135 1.581a.5.5 0 0 1 0 .964L15.5 14.063a2 2 0 0 0-1.437 1.437l-1.582 6.135a.5.5 0 0 1-.963 0z"/>`)
	newChatIcon = icon(`<path d="M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.375 2.625a1 1 0 0 1 3 3l-9.013 9.014a2 2 0 0 1-.853.505l-2.873.84a.5.5 0 0 1-.62-.62l.84-2.873a2 2 0 0 1 .506-.852z"/>`)
	sidebarIcon = icon(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/>`)
	arrowUpIcon = icon(`<path d="m5 12 7-7 7 7"/><path d="M12 19V5"/>`)
	// arrowDownIcon is Tabler's arrow-down, as hyperai-next's agent shows.
	arrowDownIcon = icon(`<path d="M12 5l0 14"/><path d="M18 13l-6 6"/><path d="M6 13l6 6"/>`)
	stopIcon      = icon(`<rect x="7" y="7" width="10" height="10" rx="1.5" fill="currentColor"/>`)
	copyIcon      = icon(`<rect width="14" height="14" x="8" y="8" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>`)
	chevronIcon   = icon(`<path d="m6 9 6 6 6-6"/>`)
	moreIcon      = icon(`<circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/>`)
	messageIcon   = icon(`<path d="M7.9 20A9 9 0 1 0 4 16.1L2 22Z"/>`)
)
