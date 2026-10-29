package main

import (
	"errors"
	"strconv"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"

	"hyperai-desktop/locales"
)

// Math in replies is TeX, which the app sets as text, as a chat app shows
// it: its symbols as Unicode (\alpha as α, \leq as ≤, \mathbb{R} as ℝ),
// its letters in italics as TeX sets them, its scripts smaller, raised
// and lowered, in the conversation's own font; a fraction as a/b within
// text and stacked in a block of its own; a root as √. Math it cannot set
// — matrices and other environments, commands it does not know, TeX
// half written as a reply streams — shows as its TeX, as MyGo shows math
// without a renderer (docs/design.md).

// mathClass is what an atom of math is to the space around it, as TeX's
// classes are.
type mathClass uint8

const (
	mOrd   mathClass = iota
	mOp              // a large operator or a function's name: ∑, sin
	mBin             // a binary operator: +, ×
	mRel             // a relation: =, ≤, →
	mOpen            // (
	mClose           // )
	mPunct           // ,
	mNone            // none before the first atom
)

// mathStyle is how math is set: its size in DIPs, and the formula's,
// which scripts shrink from; how deep in scripts; whether in a block,
// whose fractions and large operators stack; and the font of letters.
type mathStyle struct {
	size, root    float32
	level         int
	display       bool
	upright, bold bool
	variant       byte // 'b' double-struck, 'c' script, 'f' fraktur
	family        string
}

// script returns the style of a script of s: smaller, to half the
// formula's size, and never stacked.
func (s mathStyle) script() mathStyle {
	t := s
	t.level++
	t.size = max(s.size*0.7, s.root*0.5)
	t.display = false
	return t
}

// inline returns s, but with fractions and large operators inline: a
// stacked fraction's parts.
func (s mathStyle) inline() mathStyle {
	t := s
	t.display = false
	return t
}

// mathRun is text of a formula, shaped: from x, on a baseline y below the
// formula's, in the text's color, or ink's, where TeX gave one.
type mathRun struct {
	glyphs []ui.Glyph
	x, y   float32
	ink    mathInk
}

// mathInk is the color TeX gave what it draws, as \color{red}; the zero
// value is none, which draws in the text's color.
type mathInk struct {
	color ui.Color
	set   bool
}

// or returns the ink's color, or text where it has none.
func (k mathInk) or(text ui.Color) ui.Color {
	if k.set {
		return k.color
	}
	return text
}

// mathRule is a line of a formula, a fraction's or an overline: its top
// y below the formula's baseline.
type mathRule struct {
	x, y, w, h float32
	ink        mathInk
}

// mathBox is math set: w wide, reaching asc above its baseline and desc
// below it, its text and lines placed from its left on its baseline.
// text is it as plain text, scripts as ^(…) and _(…), for tests.
type mathBox struct {
	w, asc, desc float32
	runs         []mathRun
	rules        []mathRule
	strokes      []mathStroke
	text         string
}

// mathStroke is a path of a formula: a delimiter drawn as a line, as one
// taller than the text is, or what TeX draws, filled where fill: its
// path, below the formula's baseline, and how wide it strokes, its
// corners sharp where miter.
type mathStroke struct {
	cmds  []mathCmd
	width float32
	miter bool
	fill  bool
	ink   mathInk
}

// mathCmd is a step of a mathStroke's path: 'M' moves to pts[0], 'L'
// draws a line to it, 'Q' a curve to pts[1] bent by pts[0], 'C' a curve
// to pts[2] bent by pts[0] and pts[1], 'Z' closes the subpath.
type mathCmd struct {
	op  byte
	pts [3][2]float32
}

// place puts o in b, its left at x and its baseline y below b's.
func (b *mathBox) place(o mathBox, x, y float32) {
	for _, r := range o.runs {
		r.x, r.y = r.x+x, r.y+y
		b.runs = append(b.runs, r)
	}
	for _, r := range o.rules {
		r.x, r.y = r.x+x, r.y+y
		b.rules = append(b.rules, r)
	}
	for _, st := range o.strokes {
		cmds := make([]mathCmd, len(st.cmds))
		for i, c := range st.cmds {
			for j := range c.pts {
				c.pts[j][0], c.pts[j][1] = c.pts[j][0]+x, c.pts[j][1]+y
			}
			cmds[i] = c
		}
		st.cmds = cmds
		b.strokes = append(b.strokes, st)
	}
	b.w = max(b.w, x+o.w)
	b.asc = max(b.asc, o.asc-y)
	b.desc = max(b.desc, o.desc+y)
}

// draw paints b in r, its box in the window, in the theme's text color.
func (b *mathBox) draw(p *ui.Painter, r ui.Rect) {
	color := p.Theme().Text
	base := r.Y + b.asc
	for _, run := range b.runs {
		p.Glyphs(run.glyphs, r.X+run.x, base+run.y, run.ink.or(color))
	}
	for _, rule := range b.rules {
		x, y := r.X+rule.x, base+rule.y
		var path ui.Path
		path.MoveTo(x, y).LineTo(x+rule.w, y).LineTo(x+rule.w, y+rule.h).LineTo(x, y+rule.h).Close()
		p.FillPath(&path, rule.ink.or(color))
	}
	for _, st := range b.strokes {
		var path ui.Path
		path.Cap(ui.ButtCap)
		if st.miter {
			path.Join(ui.MiterJoin)
		}
		at := func(pt [2]float32) (float32, float32) { return r.X + pt[0], base + pt[1] }
		for _, c := range st.cmds {
			x0, y0 := at(c.pts[0])
			switch c.op {
			case 'M':
				path.MoveTo(x0, y0)
			case 'L':
				path.LineTo(x0, y0)
			case 'Q':
				x1, y1 := at(c.pts[1])
				path.QuadTo(x0, y0, x1, y1)
			case 'C':
				x1, y1 := at(c.pts[1])
				x2, y2 := at(c.pts[2])
				path.CubeTo(x0, y0, x1, y1, x2, y2)
			case 'Z':
				path.Close()
			}
		}
		if st.fill {
			p.FillPath(&path, st.ink.or(color))
		} else {
			p.StrokePath(&path, st.width, st.ink.or(color))
		}
	}
}

// glyphs sets text in style s, italic or not, scaled by scale: a box as
// tall as text of its size, whatever its glyphs, so that scripts and
// lines sit the same over any letter.
func (s mathStyle) glyphs(text string, italic bool, scale float32) mathBox {
	size := s.size * scale
	f := ui.Font{Family: s.family, Size: size, Italic: italic}
	if s.bold {
		f.Weight = 700
	}
	g := ui.Shape(text, f)
	var w float32
	for _, gl := range g {
		w = max(w, gl.X+gl.Advance)
	}
	return mathBox{w: w, asc: 0.75 * size, desc: 0.22 * size, runs: []mathRun{{glyphs: g}}, text: text}
}

// mathAtom is an atom of math: its box, its class, the symbol it is when
// one (which an accent takes), and its scripts; limits stacks them over
// and under it, as a block's ∑.
type mathAtom struct {
	box      mathBox
	class    mathClass
	char     string
	italic   bool
	style    mathStyle
	sup, sub *mathBox
	limits   bool
	kern     bool // a space, box.w wide, which is no atom to the classes
}

// errTeX is TeX the app cannot set, which shows as TeX.
var errTeX = errors.New("TeX it cannot set")

// texParser sets TeX as it reads it, panicking errTeX where it cannot.
type texParser struct {
	src []rune
	pos int
}

func (p *texParser) fail() { panic(errTeX) }

// setTeX sets tex in st, reporting false where it cannot.
func setTeX(tex string, st mathStyle) (b mathBox, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if r != errTeX {
				panic(r)
			}
			ok = false
		}
	}()
	p := &texParser{src: []rune(tex)}
	atoms := p.list(st, "")
	if len(atoms) == 0 {
		return mathBox{}, false
	}
	b = hpack(atoms, st)
	// An italic letter last leans past its advance.
	if a := atoms[len(atoms)-1]; a.italic && a.sup == nil && a.sub == nil {
		b.w += 0.06 * st.size
	}
	return b, true
}

func (p *texParser) skipSpace() {
	for p.pos < len(p.src) && unicode.IsSpace(p.src[p.pos]) {
		p.pos++
	}
}

// at reports whether the command \name is next.
func (p *texParser) at(name string) bool {
	n := []rune(`\` + name)
	if p.pos+len(n) > len(p.src) || string(p.src[p.pos:p.pos+len(n)]) != string(n) {
		return false
	}
	end := p.pos + len(n)
	return end == len(p.src) || !isASCIILetter(n[len(n)-1]) || !isASCIILetter(p.src[end])
}

// list reads atoms up to end: "" for the end of the TeX, "}" or "]" for
// that character, "right" for \right, which it leaves, "cell" for the
// end of a cell of a matrix, & \\ or \end, which it leaves.
func (p *texParser) list(st mathStyle, end string) []mathAtom {
	var atoms []mathAtom
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			if end != "" {
				p.fail()
			}
			return atoms
		}
		r := p.src[p.pos]
		switch {
		case end == "right" && p.at("right"):
			return atoms
		case end == "cell" && (r == '&' || p.at(`\`) || p.at("end")):
			return atoms
		case r == '}':
			if end != "}" {
				p.fail()
			}
			p.pos++
			return atoms
		case r == ']' && end == "]":
			p.pos++
			return atoms
		case r == '^' || r == '_':
			p.pos++
			if len(atoms) == 0 || atoms[len(atoms)-1].kern {
				atoms = append(atoms, mathAtom{box: mathBox{asc: 0.75 * st.size, desc: 0.22 * st.size}})
			}
			a := &atoms[len(atoms)-1]
			s := p.arg(st.script())
			if r == '^' {
				if a.sup != nil {
					p.fail()
				}
				a.sup = &s
			} else {
				if a.sub != nil {
					p.fail()
				}
				a.sub = &s
			}
		case r == '&':
			p.fail()
		default:
			atoms = append(atoms, p.atom(st)...)
		}
	}
}

// argAtoms reads an argument: a group's atoms, or one token's.
func (p *texParser) argAtoms(st mathStyle) []mathAtom {
	p.skipSpace()
	if p.pos >= len(p.src) {
		p.fail()
	}
	if p.src[p.pos] == '{' {
		p.pos++
		return p.list(st, "}")
	}
	atoms := p.atom(st)
	if len(atoms) == 0 {
		p.fail()
	}
	return atoms
}

// arg reads an argument, set.
func (p *texParser) arg(st mathStyle) mathBox { return hpack(p.argAtoms(st), st) }

// rawGroup reads a group of text, as \text's: its braces' content, which
// holds no commands.
func (p *texParser) rawGroup() string {
	p.skipSpace()
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		p.fail()
	}
	depth, start := 0, p.pos+1
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				s := string(p.src[start:p.pos])
				p.pos++
				if strings.ContainsAny(s, `\$`) {
					p.fail()
				}
				return strings.NewReplacer("{", "", "}", "").Replace(s)
			}
		}
		p.pos++
	}
	p.fail()
	return ""
}

// asciiMath are the characters of TeX's math that stand for themselves,
// or for a symbol, by their class.
var asciiMath = map[rune]texSymbol{
	'+': {"+", mBin, false}, '-': {"−", mBin, false}, '*': {"∗", mBin, false}, '/': {"/", mOrd, false},
	'=': {"=", mRel, false}, '<': {"<", mRel, false}, '>': {">", mRel, false}, ':': {":", mRel, false},
	',': {",", mPunct, false}, ';': {";", mPunct, false}, '!': {"!", mClose, false}, '?': {"?", mClose, false},
	'(': {"(", mOpen, false}, '[': {"[", mOpen, false}, ')': {")", mClose, false}, ']': {"]", mClose, false},
	'|': {"|", mOrd, false}, '\'': {"′", mOrd, false}, '.': {".", mOrd, false}, '@': {"@", mOrd, false},
}

// atom reads the atoms of one token: a character, a group or a command.
func (p *texParser) atom(st mathStyle) []mathAtom {
	r := p.src[p.pos]
	switch {
	case r == '{':
		p.pos++
		return []mathAtom{{box: hpack(p.list(st, "}"), st), class: mOrd}}
	case r == '\\':
		return p.command(st)
	case r == '~':
		p.pos++
		return []mathAtom{kern(st, 1.0/3)}
	case isDigit(r) || r == '.' && p.pos+1 < len(p.src) && isDigit(p.src[p.pos+1]):
		start := p.pos
		for p.pos < len(p.src) && (isDigit(p.src[p.pos]) || p.src[p.pos] == '.' && p.pos+1 < len(p.src) && isDigit(p.src[p.pos+1])) {
			p.pos++
		}
		s := string(p.src[start:p.pos])
		if st.variant == 'b' {
			s = mapVariant(s, st.variant)
		}
		return []mathAtom{{box: st.glyphs(s, false, 1), class: mOrd}}
	case isASCIILetter(r):
		p.pos++
		return []mathAtom{st.letter(r)}
	}
	p.pos++
	if sym, ok := asciiMath[r]; ok {
		return []mathAtom{st.symbol(sym)}
	}
	if r < unicode.MaxASCII {
		// # $ % & " ` and the like: TeX's own, not math.
		p.fail()
	}
	// A symbol written as itself, as ≤ or α.
	if sym, ok := unicodeMath[string(r)]; ok {
		return []mathAtom{st.symbol(sym)}
	}
	return []mathAtom{{box: st.glyphs(string(r), false, 1), class: mOrd, char: string(r)}}
}

// letter sets a Latin letter: italic, as TeX sets variables, unless the
// style is upright, bold or of another alphabet.
func (st mathStyle) letter(r rune) mathAtom {
	s := string(r)
	italic := !st.upright && st.variant == 0
	if st.variant != 0 {
		s = mapVariant(s, st.variant)
	}
	return mathAtom{box: st.glyphs(s, italic, 1), class: mOrd, char: s, italic: italic, style: st}
}

// symbol sets a symbol.
func (st mathStyle) symbol(sym texSymbol) mathAtom {
	italic := sym.italic && !st.upright
	return mathAtom{box: st.glyphs(sym.text, italic, 1), class: sym.class, char: sym.text, italic: italic, style: st}
}

// kern returns a space em wide, as a share of the style's size.
func kern(st mathStyle, em float32) mathAtom {
	return mathAtom{box: mathBox{w: em * st.size}, kern: true}
}

// name reads a command's name, after its backslash: its letters, or the
// one character that is not one.
func (p *texParser) name() string {
	p.pos++ // the backslash
	if p.pos >= len(p.src) {
		p.fail()
	}
	start := p.pos
	if !isASCIILetter(p.src[p.pos]) {
		p.pos++
		return string(p.src[start:p.pos])
	}
	for p.pos < len(p.src) && isASCIILetter(p.src[p.pos]) {
		p.pos++
	}
	// \operatorname*, as \operatorname.
	if string(p.src[start:p.pos]) == "operatorname" && p.pos < len(p.src) && p.src[p.pos] == '*' {
		p.pos++
	}
	return string(p.src[start:p.pos])
}

// command reads a command and its arguments, as atoms.
func (p *texParser) command(st mathStyle) []mathAtom {
	name := p.name()
	if em, ok := texSpaces[name]; ok {
		return []mathAtom{kern(st, em)}
	}
	if sym, ok := texSymbols[name]; ok {
		return []mathAtom{st.symbol(sym)}
	}
	if text, ok := texFunctions[name]; ok {
		a := mathAtom{box: st.inline().glyphs(text, false, 1), class: mOp, char: text}
		a.limits = st.display && st.level == 0 && texLimits[name]
		return []mathAtom{a}
	}
	if text, ok := texBigOps[name]; ok {
		return []mathAtom{bigOp(st, text, !strings.Contains(text, "∫"))}
	}
	if comb, ok := texAccents[name]; ok {
		args := p.argAtoms(st)
		if len(args) != 1 || args[0].char == "" || args[0].sup != nil || args[0].sub != nil {
			p.fail()
		}
		a := args[0]
		a.char += comb
		a.box = a.style.glyphs(a.char, a.italic, 1)
		return []mathAtom{a}
	}
	switch name {
	case "displaystyle", "textstyle", "limits", "nolimits", "nonumber", "notag",
		"big", "Big", "bigg", "Bigg", "bigl", "bigr", "Bigl", "Bigr", "biggl", "biggr", "Biggl", "Biggr", "bigm", "Bigm":
		// Sizes of delimiters, and styles, which text has no use for.
		return nil
	case "left":
		return []mathAtom{p.leftRight(st)}
	case "begin":
		env := p.envName()
		if _, ok := gridDelimiters[env]; !ok {
			p.fail()
		}
		return []mathAtom{p.grid(st, env)}
	case "frac", "dfrac", "tfrac", "cfrac":
		num := p.argAtoms(st.inline())
		den := p.argAtoms(st.inline())
		if st.display && st.level == 0 && name != "tfrac" {
			return []mathAtom{{box: stackFrac(hpack(num, st.inline()), hpack(den, st.inline()), st), class: mOrd}}
		}
		return []mathAtom{slashFrac(num, den, st)}
	case "sqrt":
		var index []mathAtom
		p.skipSpace()
		if p.pos < len(p.src) && p.src[p.pos] == '[' {
			p.pos++
			index = p.list(st.script().script(), "]")
		}
		return []mathAtom{root(st, index, p.argAtoms(st))}
	case "overline", "bar", "underline":
		b := p.arg(st)
		t := max(1, 0.05*st.size)
		out := mathBox{w: b.w, asc: b.asc, desc: b.desc}
		out.place(b, 0, 0)
		if name == "underline" {
			out.rules = append(out.rules, mathRule{x: 0, y: b.desc + 0.04*st.size, w: b.w, h: t})
			out.desc += 0.04*st.size + t
		} else {
			out.rules = append(out.rules, mathRule{x: 0.04 * st.size, y: -(b.asc + 0.06*st.size + t), w: b.w, h: t})
			out.asc += 0.06*st.size + t
		}
		out.text = b.text + "̅"
		return []mathAtom{{box: out, class: mOrd}}
	case "mathbb", "Bbb", "mathcal", "mathscr", "mathfrak":
		s := st
		s.variant = map[string]byte{"mathbb": 'b', "Bbb": 'b', "mathcal": 'c', "mathscr": 'c', "mathfrak": 'f'}[name]
		return p.argAtoms(s)
	case "mathbf", "boldsymbol", "bm", "mathrm", "mathup", "mathit", "mathnormal", "mathsf", "mathtt":
		s := st
		s.bold = name == "mathbf" || name == "boldsymbol" || name == "bm"
		s.upright = name == "mathbf" || name == "mathrm" || name == "mathup" || name == "mathsf" || name == "mathtt"
		return p.argAtoms(s)
	case "operatorname", "operatorname*":
		s := st
		s.upright = true
		return []mathAtom{{box: hpack(p.argAtoms(s), s), class: mOp}}
	case "text", "textrm", "textup", "textnormal", "mbox", "textit", "textbf", "textsf", "texttt":
		s := st.inline()
		s.bold = name == "textbf"
		return []mathAtom{{box: s.glyphs(p.rawGroup(), name == "textit", 1), class: mOrd}}
	case "not":
		p.skipSpace()
		var next string
		switch {
		case p.pos < len(p.src) && p.src[p.pos] == '\\':
			next = p.name()
		case p.pos < len(p.src):
			next = string(p.src[p.pos])
			p.pos++
		}
		if text, ok := texNegated[next]; ok {
			return []mathAtom{st.symbol(texSymbol{text, mRel, false})}
		}
		p.fail()
	case "pmod":
		arg := p.argAtoms(st)
		out := []mathAtom{kern(st, 1), st.symbol(texSymbol{"(", mOpen, false}),
			{box: st.inline().glyphs("mod", false, 1), class: mOrd}, kern(st, 1.0/3)}
		out = append(out, arg...)
		return append(out, st.symbol(texSymbol{")", mClose, false}))
	case "bmod", "mod":
		return []mathAtom{{box: st.glyphs("mod", false, 1), class: mBin}}
	}
	p.fail()
	return nil
}

// leftRight reads \left's delimiter, what it holds up to \right, and
// \right's, as an atom: the delimiters as tall as what they hold, which
// only a block's fractions make taller than a line.
func (p *texParser) leftRight(st mathStyle) mathAtom {
	open := p.delimiter()
	inner := p.list(st, "right")
	p.pos += len(`\right`)
	shut := p.delimiter()
	return mathAtom{box: delimit(open, hpack(inner, st), shut, 0, st), class: mOrd}
}

// delimit returns body between the delimiters open and shut ("" for
// none), pad apart from it, each as tall as it.
func delimit(open string, body mathBox, shut string, pad float32, st mathStyle) mathBox {
	var out mathBox
	x := float32(0)
	if open != "" {
		d := delimiterBox(open, body.asc, body.desc, st)
		out.place(d, x, 0)
		out.text += open
		x += d.w + pad
	}
	out.place(body, x, 0)
	out.text += body.text
	x += body.w
	if shut != "" {
		x += pad
		d := delimiterBox(shut, body.asc, body.desc, st)
		out.place(d, x, 0)
		out.text += shut
		x += d.w
	}
	out.w = x
	return out
}

// tallDelimiter is how much taller than a line of text what a delimiter
// holds is, at least, for the delimiter to be drawn rather than set.
const tallDelimiter = 1.15

// delimiterBox returns the delimiter d as tall as what reaches asc above
// the baseline and desc below it: the text's own where that is a line of
// text, and drawn, its stroke the text's, where taller, as KaTeX draws
// its tall ones: a glyph made larger thickens as it grows.
func delimiterBox(d string, asc, desc float32, st mathStyle) mathBox {
	s := st.size
	h := asc + desc
	if h <= tallDelimiter*0.97*s {
		return st.glyphs(d, false, 1)
	}
	// A little past what it holds, as TeX's.
	asc, desc = asc+0.08*s, desc+0.08*s
	h = asc + desc
	top, bot, mid := -asc, desc, (desc-asc)/2
	t := max(1, 0.07*s)
	pad := 0.08 * s
	var w float32
	var cmds []mathCmd
	move := func(x, y float32) { cmds = append(cmds, mathCmd{op: 'M', pts: [3][2]float32{{x, y}}}) }
	line := func(x, y float32) { cmds = append(cmds, mathCmd{op: 'L', pts: [3][2]float32{{x, y}}}) }
	curve := func(cx, cy, x, y float32) {
		cmds = append(cmds, mathCmd{op: 'Q', pts: [3][2]float32{{cx, cy}, {x, y}}})
	}
	miter := false
	// Each is drawn as it opens, on the left; one that closes mirrors it.
	shape := d
	if mirror, ok := delimiterMirrors[d]; ok {
		shape = mirror
	}
	switch shape {
	case "(":
		bulge := min(0.3*s, 0.12*s+0.05*h)
		w = bulge + t + 2*pad
		xr, xl := w-pad-t/2, w-pad-t/2-bulge
		a := min(0.35*h, 0.9*s)
		move(xr, top)
		curve(xl, top+0.3*a, xl, top+a)
		line(xl, bot-a)
		curve(xl, bot-0.3*a, xr, bot)
	case "[", "⌈", "⌊":
		w = 0.3*s + t + 2*pad
		xr, xl := w-pad, pad+t/2
		miter = true
		switch shape {
		case "[":
			move(xr, top)
			line(xl, top)
			line(xl, bot)
			line(xr, bot)
		case "⌈":
			move(xr, top)
			line(xl, top)
			line(xl, bot)
		case "⌊":
			move(xl, top)
			line(xl, bot)
			line(xr, bot)
		}
	case "{":
		w = 0.5*s + t + 2*pad
		xr, xl := w-pad-t/2, pad+t/2
		xm := (xr + xl) / 2
		r := min(0.25*s, h/8)
		move(xr, top)
		curve(xm, top, xm, top+r)
		line(xm, mid-r)
		curve(xm, mid, xl, mid)
		curve(xm, mid, xm, mid+r)
		line(xm, bot-r)
		curve(xm, bot, xr, bot)
	case "⟨":
		w = 0.35*s + t + 2*pad
		xr, xl := w-pad-t/2, pad+t/2
		miter = true
		move(xr, top)
		line(xl, mid)
		line(xr, bot)
	case "|":
		w = t + 2*pad
		move(w/2, top)
		line(w/2, bot)
	case "‖":
		gap := 0.15 * s
		w = gap + t + 2*pad
		move(pad+t/2, top)
		line(pad+t/2, bot)
		move(pad+t/2+gap, top)
		line(pad+t/2+gap, bot)
	default:
		// No drawing of its own: the glyph, as large as it needs,
		// centered on what it holds.
		k := h / (0.97 * s)
		b := st.glyphs(d, false, k)
		dy := mid + (0.75-0.22)*k*s/2
		var out mathBox
		out.place(b, 0, dy)
		out.text = d
		return out
	}
	if shape != d {
		for i := range cmds {
			for j := range cmds[i].pts {
				cmds[i].pts[j][0] = w - cmds[i].pts[j][0]
			}
		}
	}
	return mathBox{w: w, asc: asc, desc: desc, strokes: []mathStroke{{cmds: cmds, width: t, miter: miter}}, text: d}
}

// delimiterMirrors are the delimiters that close, by the one that opens
// as they do, which they are drawn as, mirrored.
var delimiterMirrors = map[string]string{")": "(", "]": "[", "⌉": "⌈", "⌋": "⌊", "}": "{", "⟩": "⟨"}

// gridDelimiters are the delimiters of the environments that set a grid:
// matrices, which center their cells, and cases, which set them from the
// left, a quad apart.
var gridDelimiters = map[string][2]string{
	"matrix": {"", ""}, "smallmatrix": {"", ""}, "pmatrix": {"(", ")"}, "bmatrix": {"[", "]"},
	"Bmatrix": {"{", "}"}, "vmatrix": {"|", "|"}, "Vmatrix": {"‖", "‖"},
	"cases": {"{", ""}, "dcases": {"{", ""}, "rcases": {"", "}"},
}

// envName reads the name of an environment, in braces.
func (p *texParser) envName() string {
	p.skipSpace()
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		p.fail()
	}
	end := p.pos + 1
	for end < len(p.src) && p.src[end] != '}' {
		end++
	}
	if end >= len(p.src) {
		p.fail()
	}
	name := string(p.src[p.pos+1 : end])
	p.pos = end + 1
	return name
}

// grid reads the cells of the environment env up to its \end, and sets
// them: rows as far apart as a matrix's, a quad between columns, centered
// on the math axis, between the environment's delimiters.
func (p *texParser) grid(st mathStyle, env string) mathAtom {
	delims := gridDelimiters[env]
	cases := strings.HasSuffix(env, "cases")
	cell := st.inline()
	switch env {
	case "smallmatrix":
		cell = st.script()
	case "dcases":
		cell = st
	}
	var rows [][]mathBox
	var row []mathBox
	for {
		atoms := p.list(cell, "cell")
		row = append(row, hpack(atoms, cell))
		switch {
		case p.src[p.pos] == '&':
			p.pos++
			continue
		case p.at(`\`):
			p.pos += 2
			// \\[4pt]: a row's extra room, which the grid does not take.
			if p.pos < len(p.src) && p.src[p.pos] == '[' {
				for p.pos < len(p.src) && p.src[p.pos] != ']' {
					p.pos++
				}
				p.pos++
			}
			rows, row = append(rows, row), nil
			continue
		}
		// \end, after the last cell: a row left empty by a last \\ is none.
		p.pos += len(`\end`)
		if p.envName() != env {
			p.fail()
		}
		if len(row) > 1 || len(atoms) > 0 || len(rows) == 0 {
			rows = append(rows, row)
		}
		break
	}

	s, cs := st.size, cell.size
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	widths := make([]float32, cols)
	for _, r := range rows {
		for j, c := range r {
			widths[j] = max(widths[j], c.w)
		}
	}
	gap := s // a quad
	// Each row at least as tall as a line of the cells' text, as TeX's
	// strut makes it.
	var h float32
	ascs, descs := make([]float32, len(rows)), make([]float32, len(rows))
	for i, r := range rows {
		ascs[i], descs[i] = 0.85*cs, 0.35*cs
		for _, c := range r {
			ascs[i], descs[i] = max(ascs[i], c.asc), max(descs[i], c.desc)
		}
		h += ascs[i] + descs[i]
	}
	axis := 0.25 * s
	var g mathBox
	y := -(axis + h/2)
	texts := make([]string, len(rows))
	for i, r := range rows {
		y += ascs[i]
		x := float32(0)
		cells := make([]string, len(r))
		for j, c := range r {
			at := x + (widths[j]-c.w)/2
			if cases {
				at = x
			}
			g.place(c, at, y)
			cells[j] = c.text
			x += widths[j] + gap
		}
		texts[i] = strings.Join(cells, ",")
		y += descs[i]
	}
	g.w = gap * float32(max(cols-1, 0))
	for _, w := range widths {
		g.w += w
	}
	g.asc, g.desc = axis+h/2, h/2-axis
	g.text = strings.Join(texts, ";")
	pad := 0.12 * s
	if cases {
		pad = 0.2 * s
	}
	return mathAtom{box: delimit(delims[0], g, delims[1], pad, st), class: mOrd}
}

// delimiter reads a delimiter of \left or \right: "" for \left. and
// \right.
func (p *texParser) delimiter() string {
	p.skipSpace()
	if p.pos >= len(p.src) {
		p.fail()
	}
	r := p.src[p.pos]
	switch {
	case r == '.':
		p.pos++
		return ""
	case r == '\\':
		name := p.name()
		if d, ok := texDelimiters[name]; ok {
			return d
		}
	case strings.ContainsRune("()[]|/", r):
		p.pos++
		return string(r)
	case r == '<':
		p.pos++
		return "⟨"
	case r == '>':
		p.pos++
		return "⟩"
	}
	p.fail()
	return ""
}

// bigOp sets a large operator: larger than the text, and larger still in
// a block, where sums and products stack their limits; centered on the
// math axis, as TeX does.
func bigOp(st mathStyle, text string, stacks bool) mathAtom {
	k := float32(1)
	switch {
	case st.display && st.level == 0:
		k = 1.5
	case st.level == 0:
		k = 1.1
	}
	b := st.glyphs(text, false, k)
	dy := (0.35*k - 0.25) * st.size
	if dy > 0 {
		for i := range b.runs {
			b.runs[i].y += dy
		}
		b.asc -= dy
		b.desc += dy
	}
	return mathAtom{box: b, class: mOp, char: text, limits: stacks && st.display && st.level == 0}
}

// slashFrac sets a fraction within text: num/den, each in parentheses
// where it would read otherwise, as (a+b)/2 and 1/(2π).
func slashFrac(num, den []mathAtom, st mathStyle) mathAtom {
	paren := func(atoms []mathAtom, den bool) []mathAtom {
		if !hasBinRel(atoms) && (!den || countAtoms(atoms) <= 1) {
			return atoms
		}
		out := []mathAtom{st.symbol(texSymbol{"(", mOpen, false})}
		out = append(out, atoms...)
		return append(out, st.symbol(texSymbol{")", mClose, false}))
	}
	atoms := paren(num, false)
	atoms = append(atoms, st.symbol(texSymbol{"/", mOrd, false}))
	atoms = append(atoms, paren(den, true)...)
	return mathAtom{box: hpack(atoms, st), class: mOrd}
}

// stackFrac sets a fraction of a block: num over den, centered, a line
// between them on the math axis.
func stackFrac(num, den mathBox, st mathStyle) mathBox {
	s := st.size
	t := max(1, 0.05*s)
	axis, gap, pad := 0.25*s, 0.14*s, 0.12*s
	w := max(num.w, den.w) + 2*pad
	var b mathBox
	b.place(num, (w-num.w)/2, -(axis + t/2 + gap + num.desc))
	b.place(den, (w-den.w)/2, -axis+t/2+gap+den.asc)
	b.rules = append(b.rules, mathRule{x: pad / 2, y: -axis - t/2, w: w - pad, h: t})
	b.w = w
	b.text = "(" + num.text + ")/(" + den.text + ")"
	return b
}

// root sets a square root, or the root index says, as √x, ∛x or √(x+1).
func root(st mathStyle, index, radicand []mathAtom) mathAtom {
	sign := "√"
	var idx *mathBox
	if len(index) > 0 {
		ib := hpack(index, st.script().script())
		switch ib.text {
		case "3":
			sign = "∛"
		case "4":
			sign = "∜"
		default:
			idx = &ib
		}
	}
	atoms := []mathAtom{}
	if idx != nil {
		atoms = append(atoms, mathAtom{box: raise(*idx, 0.5*st.size), class: mOrd})
	}
	atoms = append(atoms, st.symbol(texSymbol{sign, mOrd, false}))
	if countAtoms(radicand) > 1 {
		atoms = append(atoms, st.symbol(texSymbol{"(", mOpen, false}))
		atoms = append(atoms, radicand...)
		atoms = append(atoms, st.symbol(texSymbol{")", mClose, false}))
	} else {
		atoms = append(atoms, radicand...)
	}
	return mathAtom{box: hpack(atoms, st), class: mOrd}
}

// raise returns b raised by dy.
func raise(b mathBox, dy float32) mathBox {
	var out mathBox
	out.place(b, 0, -dy)
	out.text = b.text
	return out
}

// hasBinRel reports whether atoms hold a binary operator or a relation
// outside parentheses, which a fraction's part needs parentheses for:
// a+b does, n(n+1) does not.
func hasBinRel(atoms []mathAtom) bool {
	depth := 0
	for i, a := range atoms {
		switch {
		case a.class == mOpen:
			depth++
		case a.class == mClose:
			depth--
		case depth == 0 && (a.class == mRel || a.class == mBin && i > 0):
			return true
		}
	}
	return false
}

// countAtoms counts atoms, but spaces.
func countAtoms(atoms []mathAtom) int {
	n := 0
	for _, a := range atoms {
		if !a.kern {
			n++
		}
	}
	return n
}

// hpack sets atoms in a row, as TeX spaces them by their classes: a
// binary operator where it has two sides, unary otherwise (−x); in
// scripts, spaced only by large operators.
func hpack(atoms []mathAtom, st mathStyle) mathBox {
	var b mathBox
	x := float32(0)
	prev := mNone
	for i := range atoms {
		a := &atoms[i]
		if a.kern {
			x += a.box.w
			continue
		}
		class := a.class
		if class == mBin {
			next := mNone
			for _, n := range atoms[i+1:] {
				if !n.kern {
					next = n.class
					break
				}
			}
			switch prev {
			case mNone, mBin, mOp, mRel, mOpen, mPunct:
				class = mOrd
			}
			switch next {
			case mNone, mRel, mClose, mPunct:
				class = mOrd
			}
		}
		if prev != mNone {
			x += mathSpace(prev, class, st)
		}
		ab := a.withScripts(st)
		b.place(ab, x, 0)
		b.text += ab.text
		x += ab.w
		prev = class
	}
	b.w = max(0, x)
	if len(b.runs) == 0 && len(b.rules) == 0 {
		b.asc, b.desc = 0.75*st.size, 0.22*st.size
	}
	return b
}

// mathSpace is the space between atoms of classes left and right, as
// TeX's table has it: thin, medium and thick, as sixths, two ninths and
// five eighteenths of an em.
func mathSpace(left, right mathClass, st mathStyle) float32 {
	thin, med, thick := st.size/6, st.size*2/9, st.size*5/18
	script := st.level > 0
	switch {
	case left == mOp && (right == mOrd || right == mOp), right == mOp && (left == mOrd || left == mClose):
		return thin
	case script:
		return 0
	case left == mBin || right == mBin:
		return med
	case left == mRel && right == mRel, left == mOpen, right == mClose, right == mPunct:
		return 0
	case left == mRel || right == mRel:
		if left == mPunct {
			return thin
		}
		return thick
	case left == mPunct:
		return thin
	}
	return 0
}

// withScripts returns the atom's box with its scripts: beside it, the
// superscript raised and past an italic letter's lean, the subscript
// lowered, apart; or over and under it, centered, where it has limits.
func (a *mathAtom) withScripts(st mathStyle) mathBox {
	if a.sup == nil && a.sub == nil {
		return a.box
	}
	s := st.size
	var b mathBox
	if a.limits {
		w := a.box.w
		if a.sup != nil {
			w = max(w, a.sup.w)
		}
		if a.sub != nil {
			w = max(w, a.sub.w)
		}
		b.place(a.box, (w-a.box.w)/2, 0)
		text := a.box.text
		if a.sub != nil {
			b.place(*a.sub, (w-a.sub.w)/2, a.box.desc+0.12*s+a.sub.asc)
			text += "_(" + a.sub.text + ")"
		}
		if a.sup != nil {
			b.place(*a.sup, (w-a.sup.w)/2, -(a.box.asc + 0.12*s + a.sup.desc))
			text += "^(" + a.sup.text + ")"
		}
		b.w, b.text = w, text
		return b
	}
	b.place(a.box, 0, 0)
	x := a.box.w
	lean := float32(0)
	if a.italic {
		lean = 0.08 * s
	}
	up := max(0.42*s, a.box.asc-0.3*s)
	down := max(0.2*s, a.box.desc-0.05*s)
	if a.sup != nil && a.sub != nil {
		down = max(down, 0.25*s)
		// The two apart by a little at least.
		if gap := (down - a.sub.asc) - (-up + a.sup.desc); gap < 0.12*s {
			up += (0.12*s - gap) / 2
			down += (0.12*s - gap) / 2
		}
	}
	w := x
	text := a.box.text
	if a.sub != nil {
		b.place(*a.sub, x, down)
		w = max(w, x+a.sub.w)
		text += "_(" + a.sub.text + ")"
	}
	if a.sup != nil {
		b.place(*a.sup, x+lean, -up)
		w = max(w, x+lean+a.sup.w)
		text += "^(" + a.sup.text + ")"
	}
	b.w, b.text = w+0.04*s, text
	return b
}

func isASCIILetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }
func isDigit(r rune) bool       { return r >= '0' && r <= '9' }

// mapVariant writes s's letters and digits in another alphabet of
// Unicode's: double-struck (ℝ), script (ℒ) or fraktur (𝔤).
func mapVariant(s string, variant byte) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(variantRune(r, variant))
	}
	return b.String()
}

func variantRune(r rune, variant byte) rune {
	special := map[byte]map[rune]rune{
		'b': {'C': 'ℂ', 'H': 'ℍ', 'N': 'ℕ', 'P': 'ℙ', 'Q': 'ℚ', 'R': 'ℝ', 'Z': 'ℤ'},
		'c': {'B': 'ℬ', 'E': 'ℰ', 'F': 'ℱ', 'H': 'ℋ', 'I': 'ℐ', 'L': 'ℒ', 'M': 'ℳ', 'R': 'ℛ', 'e': 'ℯ', 'g': 'ℊ', 'o': 'ℴ'},
		'f': {'C': 'ℭ', 'H': 'ℌ', 'I': 'ℑ', 'R': 'ℜ', 'Z': 'ℨ'},
	}
	if s, ok := special[variant][r]; ok {
		return s
	}
	upper := map[byte]rune{'b': 0x1D538, 'c': 0x1D49C, 'f': 0x1D504}[variant]
	lower := map[byte]rune{'b': 0x1D552, 'c': 0x1D4B6, 'f': 0x1D51E}[variant]
	switch {
	case r >= 'A' && r <= 'Z':
		return upper + r - 'A'
	case r >= 'a' && r <= 'z':
		return lower + r - 'a'
	case r >= '0' && r <= '9' && variant == 'b':
		return 0x1D7D8 + r - '0'
	}
	return r
}

// texSymbol is what a command or a character stands for: its text, its
// class, and whether it is italic, as TeX sets lower-case Greek.
type texSymbol struct {
	text   string
	class  mathClass
	italic bool
}

// texSymbols are TeX's symbols by their commands' names.
var texSymbols = func() map[string]texSymbol {
	m := map[string]texSymbol{}
	add := func(class mathClass, italic bool, pairs ...string) {
		for i := 0; i < len(pairs); i += 2 {
			m[pairs[i]] = texSymbol{pairs[i+1], class, italic}
		}
	}
	add(mOrd, true,
		"alpha", "α", "beta", "β", "gamma", "γ", "delta", "δ", "epsilon", "ϵ", "varepsilon", "ε", "zeta", "ζ",
		"eta", "η", "theta", "θ", "vartheta", "ϑ", "iota", "ι", "kappa", "κ", "lambda", "λ", "mu", "μ", "nu", "ν",
		"xi", "ξ", "omicron", "ο", "pi", "π", "varpi", "ϖ", "rho", "ρ", "varrho", "ϱ", "sigma", "σ", "varsigma", "ς",
		"tau", "τ", "upsilon", "υ", "phi", "ϕ", "varphi", "φ", "chi", "χ", "psi", "ψ", "omega", "ω", "ell", "ℓ")
	add(mOrd, false,
		"Gamma", "Γ", "Delta", "Δ", "Theta", "Θ", "Lambda", "Λ", "Xi", "Ξ", "Pi", "Π", "Sigma", "Σ",
		"Upsilon", "Υ", "Phi", "Φ", "Psi", "Ψ", "Omega", "Ω",
		"infty", "∞", "partial", "∂", "nabla", "∇", "forall", "∀", "exists", "∃", "nexists", "∄",
		"emptyset", "∅", "varnothing", "∅", "neg", "¬", "lnot", "¬", "hbar", "ℏ", "Re", "ℜ", "Im", "ℑ",
		"aleph", "ℵ", "wp", "℘", "angle", "∠", "triangle", "△", "top", "⊤", "bot", "⊥", "prime", "′",
		"degree", "°", "dagger", "†", "ddagger", "‡", "checkmark", "✓", "S", "§", "surd", "√",
		"imath", "ı", "jmath", "ȷ", "dots", "…", "ldots", "…", "cdots", "⋯", "vdots", "⋮", "ddots", "⋱",
		"%", "%", "$", "$", "&", "&", "#", "#", "_", "_", "|", "‖", "vert", "|", "Vert", "‖", "backslash", "∖",
		"square", "□", "Box", "□", "diamondsuit", "♢", "heartsuit", "♡", "clubsuit", "♣", "spadesuit", "♠")
	add(mBin, false,
		"pm", "±", "mp", "∓", "times", "×", "div", "÷", "cdot", "⋅", "ast", "∗", "star", "⋆", "circ", "∘",
		"bullet", "∙", "oplus", "⊕", "ominus", "⊖", "otimes", "⊗", "oslash", "⊘", "odot", "⊙", "cup", "∪",
		"cap", "∩", "setminus", "∖", "wedge", "∧", "vee", "∨", "land", "∧", "lor", "∨", "sqcup", "⊔", "sqcap", "⊓",
		"uplus", "⊎", "amalg", "⨿", "dagger", "†")
	add(mRel, false,
		"leq", "≤", "le", "≤", "geq", "≥", "ge", "≥", "neq", "≠", "ne", "≠", "approx", "≈", "equiv", "≡",
		"sim", "∼", "simeq", "≃", "cong", "≅", "propto", "∝", "ll", "≪", "gg", "≫", "in", "∈", "notin", "∉",
		"ni", "∋", "subset", "⊂", "supset", "⊃", "subseteq", "⊆", "supseteq", "⊇", "subsetneq", "⊊", "supsetneq", "⊋",
		"to", "→", "rightarrow", "→", "leftarrow", "←", "gets", "←", "Rightarrow", "⇒", "Leftarrow", "⇐",
		"Leftrightarrow", "⇔", "iff", "⟺", "implies", "⟹", "impliedby", "⟸", "leftrightarrow", "↔",
		"mapsto", "↦", "longrightarrow", "⟶", "longleftarrow", "⟵", "Longrightarrow", "⟹", "Longleftarrow", "⟸",
		"longleftrightarrow", "⟷", "longmapsto", "⟼", "uparrow", "↑", "downarrow", "↓", "updownarrow", "↕",
		"Uparrow", "⇑", "Downarrow", "⇓", "hookrightarrow", "↪", "hookleftarrow", "↩", "rightharpoonup", "⇀",
		"perp", "⊥", "parallel", "∥", "mid", "∣", "nmid", "∤", "models", "⊨", "vdash", "⊢", "dashv", "⊣",
		"prec", "≺", "succ", "≻", "preceq", "⪯", "succeq", "⪰", "coloneqq", "≔", "doteq", "≐", "triangleq", "≜",
		"asymp", "≍", "lesssim", "≲", "gtrsim", "≳", "leqslant", "⩽", "geqslant", "⩾", "nleq", "≰", "ngeq", "≱")
	add(mOpen, false, "langle", "⟨", "lfloor", "⌊", "lceil", "⌈", "lvert", "|", "lVert", "‖", "{", "{")
	add(mClose, false, "rangle", "⟩", "rfloor", "⌋", "rceil", "⌉", "rvert", "|", "rVert", "‖", "}", "}")
	return m
}()

// unicodeMath are the symbols of texSymbols written as themselves, by
// their text: ≤ is a relation as \leq is.
var unicodeMath = func() map[string]texSymbol {
	m := map[string]texSymbol{}
	for _, s := range texSymbols {
		if s.class != mOrd || s.italic {
			if _, ok := m[s.text]; !ok {
				m[s.text] = s
			}
		}
	}
	return m
}()

// texDelimiters are the commands \left and \right take.
var texDelimiters = map[string]string{
	"{": "{", "}": "}", "|": "‖", "langle": "⟨", "rangle": "⟩", "lfloor": "⌊", "rfloor": "⌋",
	"lceil": "⌈", "rceil": "⌉", "vert": "|", "Vert": "‖", "lvert": "|", "rvert": "|", "lVert": "‖", "rVert": "‖",
	"backslash": "∖",
}

// texSpaces are TeX's spaces, in ems.
var texSpaces = map[string]float32{
	",": 1.0 / 6, "thinspace": 1.0 / 6, ":": 2.0 / 9, ">": 2.0 / 9, "medspace": 2.0 / 9, ";": 5.0 / 18,
	"thickspace": 5.0 / 18, "!": -1.0 / 6, "quad": 1, "qquad": 2, " ": 1.0 / 3, "enspace": 0.5,
}

// texFunctions are the functions TeX sets upright, as their names, and
// texLimits those whose scripts a block stacks, as \lim's.
var (
	texFunctions = func() map[string]string {
		m := map[string]string{"limsup": "lim sup", "liminf": "lim inf"}
		for _, f := range strings.Fields("sin cos tan cot sec csc arcsin arccos arctan sinh cosh tanh coth " +
			"log ln lg exp lim max min sup inf det dim ker deg gcd lcm arg Pr hom") {
			m[f] = f
		}
		return m
	}()
	texLimits = map[string]bool{"lim": true, "limsup": true, "liminf": true, "max": true, "min": true,
		"sup": true, "inf": true, "det": true, "Pr": true, "gcd": true}
)

// texBigOps are TeX's large operators.
var texBigOps = map[string]string{
	"sum": "∑", "prod": "∏", "coprod": "∐", "bigcup": "⋃", "bigcap": "⋂", "bigoplus": "⨁", "bigotimes": "⨂",
	"bigodot": "⨀", "bigvee": "⋁", "bigwedge": "⋀", "bigsqcup": "⨆",
	"int": "∫", "iint": "∬", "iiint": "∭", "oint": "∮",
}

// texAccents are TeX's accents, as the combining marks a symbol takes.
var texAccents = map[string]string{
	"hat": "̂", "widehat": "̂", "tilde": "̃", "widetilde": "̃", "vec": "⃗",
	"dot": "̇", "ddot": "̈", "acute": "́", "grave": "̀", "check": "̌",
	"breve": "̆", "mathring": "̊",
}

// texNegated are the relations \not makes of others.
var texNegated = map[string]string{
	"=": "≠", "in": "∉", "subset": "⊄", "supset": "⊅", "subseteq": "⊈", "supseteq": "⊉", "equiv": "≢",
	"sim": "≁", "approx": "≉", "cong": "≇", "mid": "∤", "parallel": "∦", "<": "≮", ">": "≯", "leq": "≰", "geq": "≱",
}

// maxFormulas is how many formulas the app keeps set, before it sets
// them again: a reply streaming asks for its paragraph's every frame.
const maxFormulas = 512

// formula returns tex set for text of size DIPs around it, in a block of
// its own or within text, kept; nil where the app cannot set it, which
// shows as TeX. RaTeX sets it once its engine is ready (katex.go), as
// KaTeX does; until then, the app sets it as text, in the conversation's
// font, a block larger than the text.
func (a *app) formula(tex string, size float32, display bool) *mathBox {
	engine := "text"
	if a.tex != nil {
		engine = "ratex"
	}
	key := engine + "\x00" + tex + "\x00" + strconv.FormatFloat(float64(size), 'f', 2, 32) + "\x00" + a.font
	if display {
		key += "\x00display"
	}
	if f, ok := a.formulas[key]; ok {
		return f
	}
	if a.formulas == nil || len(a.formulas) >= maxFormulas {
		a.formulas = map[string]*mathBox{}
	}
	var f *mathBox
	switch {
	case a.tex != nil:
		f = a.typeset(tex, size, display)
	case display:
		size *= displayMathScale
		fallthrough
	default:
		if b, ok := setTeX(tex, mathStyle{size: size, root: size, display: display, family: a.font}); ok {
			f = &b
		}
	}
	a.formulas[key] = f
	return f
}

// displayMathScale is how much larger than the text a block of math the
// app sets as text is.
const displayMathScale = 1.15

// setMath makes the replies' Markdown set math: within text as objects
// of the line, and blocks of it, once whole, centered, their menu copying
// their LaTeX; what the app cannot set shows as TeX, and so does all of it
// where General turns renderMath off. Math within text is copied as its
// TeX as MyGo copies it, with its delimiters, as part of the selection.
func (a *app) setMath() {
	a.md.InlineMath = func(tex string, size float32) (markdown.InlineObject, bool) {
		if !a.renderMath {
			return markdown.InlineObject{}, false
		}
		f := a.formula(tex, size, false)
		if f == nil {
			return markdown.InlineObject{}, false
		}
		return markdown.InlineObject{Width: f.w, Height: f.asc, Depth: f.desc, Draw: f.draw}, true
	}
	a.md.Block = func(c *ui.Context, d *parser.Document, i int) bool {
		b := d.Block(i)
		if b.Kind != parser.MathBlock || b.Open || !a.renderMath {
			return false
		}
		tex := strings.TrimSpace(d.Lines(i))
		f := a.formula(tex, a.body, true)
		if f == nil {
			return false
		}
		// Centered in the column, as the column was laid out last; where
		// wider, from its left, scrolling sideways.
		col := ui.Column(c).AlignSelf(ui.Stretch).Padding(4, 0)
		wide := col.Bounds().W > 0 && f.w > col.Bounds().W
		col.Children(func() {
			formula := func() {
				ui.Box(c).Width(f.w).Height(f.asc + f.desc).Shrink(0).Label(tex).Draw(f.draw).ContextMenu(func(m *ui.Menu) {
					if m.Item(locales.CopyExpression()).Chosen() {
						c.Services().WriteClipboard(tex)
						c.Toast(locales.Copied())
					}
					m.Separator()
					m.EditItems()
				})
			}
			if wide {
				ui.ScrollHorizontal(c).AlignSelf(ui.Stretch).Children(formula)
			} else {
				ui.Row(c).Justify(ui.Center).Children(formula)
			}
		})
		return true
	}
}
