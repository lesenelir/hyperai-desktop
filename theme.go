package main

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// palette is the app's look in one appearance, built of hyperai-next's
// three colors (src/styles/schemes.css): fg for text, and for every border
// and muted tone as a transparency of it; bg for the page; and ac, the one
// voice of color, for the primary action, the selection and links.
type palette struct {
	fg, bg, ac ui.Color
	// composer is the face of the composer and the cards, raised off the
	// canvas; raised that of the user's messages.
	composer, raised ui.Color
	theme            *ui.Theme
	md               *markdown.Theme
}

var lightPalette, darkPalette = newPalette(false), newPalette(true)

// paletteFor returns the palette for the appearance t is in.
func paletteFor(t *ui.Theme) *palette {
	if t.Dark {
		return darkPalette
	}
	return lightPalette
}

func newPalette(dark bool) *palette {
	// The light theme's values, and the dark theme's with its lightness
	// offset of 20% applied, as hyperai-next's tokens resolve.
	p := &palette{
		fg: cssOklch(0.30, 0.01, 20),
		bg: cssOklch(1.00, 0.01, 20),
		ac: cssOklch(0.50, 0.28, 40),
	}
	if dark {
		p.fg = cssOklch(0.98, 0.01, 4.45)
		p.bg = cssOklch(0.20, 0.01, 4.45)
		p.ac = cssOklch(0.90, 0.212, 8)
	}
	fg := func(a float32) ui.Color { return p.fg.Alpha(a).Over(p.bg) }
	// The light canvas is a whisper off white, so that the composer, white,
	// stands off it; the dark canvas is bg, and what is raised is lighter.
	canvas, composer := fg(0.025), p.bg
	if dark {
		canvas, composer = p.bg, fg(0.045)
	}
	p.composer, p.raised = composer, p.fg.Alpha(0.07).Over(canvas)
	on := func(a float32) ui.Color { return p.fg.Alpha(a).Over(canvas) }

	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	t.Background = canvas
	t.Surface, t.SurfaceHover, t.SurfacePressed = on(0.06), on(0.09), on(0.13)
	t.Border = on(0.12)
	t.Text = p.fg
	// fg at 70% is the floor for small muted text that still reads (AA).
	t.TextMuted = on(0.70)
	t.Accent, t.AccentText = p.ac, p.bg
	t.AccentHover, t.AccentPressed = p.ac.Alpha(0.85).Over(canvas), p.ac.Alpha(0.75).Over(canvas)
	t.Selection, t.Focus = p.ac.Alpha(0.22), p.ac.Alpha(0.45)
	t.Inverse, t.InverseText = p.fg, p.bg
	t.Scrollbar = p.fg.Alpha(0.3)
	p.theme = t

	md := markdown.DefaultTheme(t)
	md.Link, md.Checkbox = p.ac, p.ac
	// Inline code takes the accent, over a wash of fg, as in Waku.
	md.Code, md.CodeBackground = p.ac, p.fg.Alpha(0.07)
	md.CodeBlockBackground, md.CodeBlockBorder = composer, on(0.10)
	md.QuoteBar, md.Rule, md.TableBorder = on(0.2), on(0.12), on(0.12)
	md.TableHeader = p.fg.Alpha(0.03)
	md.FontSize, md.LineHeight = bodySize, bodyLineHeight
	p.md = md
	return p
}

// shadow returns the color of a shadow of strength a: fg with a trace of
// the accent, so that depth keeps the theme's tone, as hyperai-next's do.
func (p *palette) shadow(a float32) ui.Color {
	return p.fg.Mix(p.ac, 0.33).Alpha(a)
}

// cssOklch returns the color of CSS's oklch(l c h) as the browser draws
// hyperai-next on a Mac: a color Display P3 cannot show has each of its
// channels clipped into P3, where ui.Oklch would take chroma off it, and
// turn the light accent, oklch(50% 0.28 40), from red to brown. The
// clipped color is inside P3, so ui.Oklch draws it as it is on a screen
// that shows P3, and as the nearest sRGB color on one that does not.
func cssOklch(l, c, h float64) ui.Color {
	r, g, b := p3(l, c, h)
	clip := func(v float64) float64 { return max(0, min(v, 1)) }
	l, c, h = oklchOfP3(clip(r), clip(g), clip(b))
	return ui.Oklch(float32(l), float32(c), float32(h))
}

// p3 returns oklch(l c h) in linear Display P3, beyond 0 to 1 when P3
// cannot show it.
func p3(l, c, h float64) (r, g, b float64) {
	h *= math.Pi / 180
	A, B := c*math.Cos(h), c*math.Sin(h)
	lc := math.Pow(l+0.3963377774*A+0.2158037573*B, 3)
	mc := math.Pow(l-0.1055613458*A-0.0638541728*B, 3)
	sc := math.Pow(l-0.0894841775*A-1.2914855480*B, 3)
	x := 1.2268798758459243*lc - 0.5578149944602171*mc + 0.2813910456659647*sc
	y := -0.0405757452148008*lc + 1.1122868032803170*mc - 0.0717110580655164*sc
	z := -0.0763729366746601*lc - 0.4214933324022432*mc + 1.5869240198367816*sc
	return 2.4934969119414254*x - 0.9313836179191239*y - 0.40271078445071684*z,
		-0.8294889695615747*x + 1.7626640603183463*y + 0.023624685841943577*z,
		0.03584583024378447*x - 0.07617238926804182*y + 0.9568845240076872*z
}

// oklchOfP3 returns the linear Display P3 color r, g, b in OKLCH.
func oklchOfP3(r, g, b float64) (l, c, h float64) {
	x := 0.4865709486482162*r + 0.26566769316909306*g + 0.1982172852343625*b
	y := 0.2289745640697488*r + 0.6917385218365064*g + 0.079286914093745*b
	z := 0.04511338185890264*g + 1.043944368900976*b
	lc := math.Cbrt(0.8190224379967030*x + 0.3619062600528904*y - 0.1288737815209879*z)
	mc := math.Cbrt(0.0329836539323885*x + 0.9292868615863434*y + 0.0361446663506424*z)
	sc := math.Cbrt(0.0481771893596242*x + 0.2642395317527308*y + 0.6335478284694309*z)
	l = 0.2104542553*lc + 0.7936177850*mc - 0.0040720468*sc
	A := 1.9779984951*lc - 2.4285922050*mc + 0.4505937099*sc
	B := 0.0259040371*lc + 0.7827717662*mc - 0.8086757660*sc
	return l, math.Hypot(A, B), math.Atan2(B, A) * 180 / math.Pi
}
