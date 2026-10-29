package main

import (
	"log"
	"sync"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/ratex"
)

// Math as KaTeX sets it: RaTeX (package ratex) lays the TeX out, as
// KaTeX's metrics and rules have it, into a list of glyphs of the KaTeX
// fonts, rules and paths, which the app draws as a mathBox, in the text's
// color where TeX gave none (docs/design.md).

// texScale is how much larger than the text around it math is, as KaTeX
// sets it (font-size: 1.21em): the x-height of its Computer Modern is
// smaller than an interface font's.
const texScale = 1.21

// typeset lays tex out for text of size DIPs around it, nil where TeX
// cannot.
func (a *app) typeset(tex string, size float32, display bool) *mathBox {
	l, err := a.tex.Layout(tex, display)
	if err != nil {
		return nil
	}
	b := texBox(l, size*texScale, a.font)
	return &b
}

// texBox returns l drawn at size DIPs to the em: its glyphs in the KaTeX
// fonts, but those they lack (CJK in \text) in the app's font family.
func texBox(l *ratex.List, size float32, family string) mathBox {
	height := float32(l.Height)
	b := mathBox{w: float32(l.Width) * size, asc: height * size, desc: float32(l.Depth) * size}
	for _, it := range l.Items {
		ink := texInk(it.Color)
		// From the box's top, in ems, to the baseline's, in DIPs.
		x, y := float32(it.X)*size, (float32(it.Y)-height)*size
		switch it.Type {
		case "GlyphPath":
			b.texGlyph(it, x, y, size, family, ink)
		case "Line":
			t := max(float32(it.Thickness)*size, 0.6)
			b.rules = append(b.rules, mathRule{x: x, y: y - t/2, w: float32(it.Width) * size, h: t, ink: ink})
		case "Rect":
			b.rules = append(b.rules, mathRule{x: x, y: y, w: float32(it.Width) * size, h: float32(it.Height) * size, ink: ink})
		case "Path":
			b.strokes = append(b.strokes, mathStroke{cmds: texPath(it.Commands, x, y, size), width: max(0.04*size, 1),
				fill: it.Fill, ink: ink})
		}
	}
	return b
}

// texInk returns the ink of a color of RaTeX's: none for its black, which
// takes the text's color.
func texInk(c ratex.Color) mathInk {
	if c.Ink() {
		return mathInk{}
	}
	channel := func(v float64) uint8 { return uint8(max(0, min(v, 1))*255 + 0.5) }
	return mathInk{color: ui.RGBA(channel(c.R), channel(c.G), channel(c.B), float32(c.A)), set: true}
}

// texPath returns RaTeX's path commands as a mathStroke's, from x, y, in
// ems of size DIPs.
func texPath(cmds []ratex.Command, x, y, size float32) []mathCmd {
	pt := func(px, py float64) [2]float32 { return [2]float32{x + float32(px)*size, y + float32(py)*size} }
	out := make([]mathCmd, 0, len(cmds))
	for _, c := range cmds {
		switch c.Type {
		case "MoveTo":
			out = append(out, mathCmd{op: 'M', pts: [3][2]float32{pt(c.X, c.Y)}})
		case "LineTo":
			out = append(out, mathCmd{op: 'L', pts: [3][2]float32{pt(c.X, c.Y)}})
		case "QuadTo":
			out = append(out, mathCmd{op: 'Q', pts: [3][2]float32{pt(c.X1, c.Y1), pt(c.X, c.Y)}})
		case "CubicTo":
			out = append(out, mathCmd{op: 'C', pts: [3][2]float32{pt(c.X1, c.Y1), pt(c.X2, c.Y2), pt(c.X, c.Y)}})
		case "Close":
			out = append(out, mathCmd{op: 'Z'})
		}
	}
	return out
}

// texGlyph adds a glyph of l at x, y in its KaTeX font, or in family
// where no KaTeX font has it. The glyphs are text, drawn as the system
// draws text, from its cache of glyphs, but without its font smoothing,
// which thickens them, as browsers draw KaTeX: drawn as their outlines
// instead, they weighed the same, and took six times as long a frame.
func (b *mathBox) texGlyph(it ratex.Item, x, y, size float32, family string, ink mathInk) {
	if f := texFamily(it.Font); f != "" {
		family = f
	}
	g := ui.Shape(string(it.Char), ui.Font{Family: family, Size: size * float32(it.Scale), Antialiased: true})
	b.runs = append(b.runs, mathRun{glyphs: g, x: x, y: y, ink: ink})
	b.text += string(it.Char)
}

var (
	texFontsOnce sync.Once
	texFamilies  map[string]string
)

// texFamily returns the family the KaTeX font name is registered as,
// registering them the first time; "" for none.
func texFamily(name string) string {
	texFontsOnce.Do(func() {
		texFamilies = map[string]string{}
		for _, n := range ratex.Fonts() {
			b, _ := ratex.Font(n)
			family := "KaTeX " + n
			if err := ui.RegisterFont(b, family); err != nil {
				log.Printf("registering the KaTeX font %s: %v", n, err)
				continue
			}
			texFamilies[n] = family
		}
	})
	return texFamilies[name]
}
