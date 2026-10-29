package ratex

import (
	"context"
	"strings"
	"sync"
	"testing"
)

var (
	engineOnce sync.Once
	engine     *Engine
	engineErr  error
)

// testEngine returns one engine for the package's tests: compiling takes
// a second.
func testEngine(t *testing.T) *Engine {
	t.Helper()
	engineOnce.Do(func() { engine, engineErr = New(context.Background(), "") })
	if engineErr != nil {
		t.Fatal(engineErr)
	}
	return engine
}

// TestLayout lays out math as models write it, and reports TeX's errors
// for what is malformed or half written, as a reply streams it.
func TestLayout(t *testing.T) {
	e := testEngine(t)
	l, err := e.Layout(`x^2`, false)
	if err != nil {
		t.Fatal(err)
	}
	// x, then a 2 raised and smaller: KaTeX's metrics, in ems.
	if len(l.Items) != 2 || l.Items[0].Font != "Math-Italic" || l.Items[0].Char != 'x' ||
		l.Items[1].Scale != 0.7 || l.Items[1].Y >= l.Items[0].Y || l.Width <= 0 || l.Height <= 0 {
		t.Errorf("x^2: %+v", l)
	}
	for _, tex := range []string{
		`\int_{-\infty}^{\infty} e^{-x^2}\,dx = \sqrt{\pi}`,
		`\begin{pmatrix} a & b \\ c & d \end{pmatrix}`,
		`f(x) = \begin{cases} x & x \geq 0 \\ -x & \text{otherwise} \end{cases}`,
		`\begin{aligned} (a+b)^2 &= a^2 + 2ab + b^2 \\ &\geq 4ab \end{aligned}`,
		`\binom{n}{k} \quad \underbrace{a+b}_{n} \quad \xrightarrow{f} \quad \color{red}{x}`,
		`\text{中文}`, `\ce{2H2 + O2 -> 2H2O}`,
	} {
		if _, err := e.Layout(tex, true); err != nil {
			t.Errorf("%s: %v", tex, err)
		}
	}
	for _, tex := range []string{`\frac{a}{`, `x^`, `\begin{pmatrix} a & b`, `\unknown{x}`, ``, strings.Repeat("x", MaxSource+1)} {
		if l, err := e.Layout(tex, false); err == nil {
			t.Errorf("%.20q laid out: %+v", tex, l)
		}
	}
	if _, err := e.Layout(`\unknown`, false); err == nil || !strings.Contains(err.Error(), "Undefined control sequence") {
		t.Errorf("the error of an unknown command: %v", err)
	}
	// The engine carries on after errors.
	if _, err := e.Layout(`a+b`, false); err != nil {
		t.Error(err)
	}
}

// TestItems reads each kind of item: rules, paths (what stretches, as an
// arrow over text; radicals and tall delimiters are the Size fonts'
// glyphs), colors, and fonts the
// KaTeX fonts lack.
func TestItems(t *testing.T) {
	e := testEngine(t)
	kinds := map[string]Item{}
	for _, tex := range []string{`\frac{a}{b}`, `\xrightarrow{f}`, `\colorbox{yellow}{x}`, `\color{red}{y}`, `\text{中}`} {
		l, err := e.Layout(tex, true)
		if err != nil {
			t.Fatalf("%s: %v", tex, err)
		}
		for _, it := range l.Items {
			if it.Type == "GlyphPath" && !it.Color.Ink() {
				kinds["colored"] = it
			}
			if it.Font == "CJK-Regular" {
				kinds["cjk"] = it
			}
			kinds[it.Type] = it
		}
	}
	if l := kinds["Line"]; l.Width <= 0 || l.Thickness <= 0 {
		t.Errorf("a fraction's rule: %+v", l)
	}
	if p := kinds["Path"]; len(p.Commands) < 2 || p.Commands[0].Type != "MoveTo" {
		t.Errorf("a stretched arrow's path: %+v", p)
	}
	if r := kinds["Rect"]; r.Width <= 0 || r.Height <= 0 {
		t.Errorf("a color box: %+v", r)
	}
	if c := kinds["colored"]; c.Color.R != 1 || c.Color.G != 0 {
		t.Errorf("red: %+v", c.Color)
	}
	if _, ok := kinds["cjk"]; !ok {
		t.Error("no glyph left to the system's fonts")
	}
}

// TestFonts holds every font an item can name but the system's.
func TestFonts(t *testing.T) {
	names := Fonts()
	if len(names) != 20 {
		t.Errorf("fonts %q", names)
	}
	for _, n := range []string{"Main-Regular", "Math-Italic", "Size4-Regular", "AMS-Regular"} {
		if b, ok := Font(n); !ok || len(b) < 1000 {
			t.Errorf("no font %s", n)
		}
	}
	if _, ok := Font("CJK-Regular"); ok {
		t.Error("a font for CJK, which the system's draw")
	}
}
