// Package ratex lays out TeX math with RaTeX
// (https://github.com/erweixin/RaTeX), a KaTeX-compatible engine written
// in Rust, run as WebAssembly in wazero: pure Go, one module for every
// platform, without cgo. It returns RaTeX's display list, which says
// where each glyph, line and path goes, and holds the KaTeX fonts its
// glyphs are drawn in.
//
// ratex.wasm is ratex_wasm_bg.wasm of the npm package ratex-wasm, at
// Version, checked by its SHA-256 (moduleSum): the WebAssembly that
// wasm-bindgen made for browsers, whose few imports from JavaScript
// Engine provides itself. fonts/ are the package's KaTeX fonts. LICENSE
// is the package's: RaTeX's MIT, and the fonts' SIL Open Font License.
package ratex

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// Version is the version of ratex-wasm that ratex.wasm is.
const Version = "0.1.15"

// moduleSum is ratex.wasm's SHA-256, as npm's ratex-wasm@0.1.15 has it.
const moduleSum = "2a6fcb282651bef1fa92798d7cb92694e79cbe7d53dbe95a2ce98bd670511c16"

//go:embed ratex.wasm
var module []byte

//go:embed fonts/*.ttf
var fonts embed.FS

// MaxSource is the longest TeX Layout takes, in bytes, as Waku limits it:
// longer math is no formula a reply means to show.
const MaxSource = 8 << 10

// List is a formula laid out: what to draw, in ems of the font size, from
// its box's top left, y down; Width wide, Height above its baseline and
// Depth below it.
type List struct {
	Items  []Item  `json:"items"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Depth  float64 `json:"depth"`
}

// Item is a drawing of a List, by Type:
//
//   - "GlyphPath": Char in the KaTeX font Font (as "Main-Regular", or
//     "CJK-Regular" for what the KaTeX fonts lack, which the system's draw),
//     Scale times the font size, its baseline's left at X, Y.
//   - "Line": a rule Width long and Thickness thick, from X, centered on Y,
//     dashed where Dashed.
//   - "Rect": a box Width by Height from X, Y.
//   - "Path": Commands from X, Y, filled where Fill, else stroked.
//
// Color is the formula's ink, black, or the color TeX gave it.
type Item struct {
	Type      string    `json:"type"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	Scale     float64   `json:"scale"`
	Font      string    `json:"font"`
	Char      rune      `json:"char_code"`
	Width     float64   `json:"width"`
	Height    float64   `json:"height"`
	Thickness float64   `json:"thickness"`
	Dashed    bool      `json:"dashed"`
	Commands  []Command `json:"commands"`
	Fill      bool      `json:"fill"`
	Color     Color     `json:"color"`
}

// Command is a step of a Path, by Type: "MoveTo" and "LineTo" X, Y;
// "QuadTo" X, Y bent by X1, Y1; "CubicTo" X, Y bent by X1, Y1 and X2, Y2;
// "Close".
type Command struct {
	Type   string
	X, Y   float64
	X1, Y1 float64
	X2, Y2 float64
}

// UnmarshalJSON reads a command, whose fields are named in lower case.
func (c *Command) UnmarshalJSON(b []byte) error {
	var v struct {
		Type                 string
		X, Y, X1, Y1, X2, Y2 float64
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*c = Command{v.Type, v.X, v.Y, v.X1, v.Y1, v.X2, v.Y2}
	return nil
}

// Color is a color's channels, from 0 to 1.
type Color struct {
	R, G, B, A float64
}

// Ink reports whether c is the formula's ink, opaque black, which takes
// the text's color, rather than a color TeX gave it.
func (c Color) Ink() bool { return c == Color{0, 0, 0, 1} }

// Font returns the KaTeX font name, as an Item's, in TrueType; false for
// none, as "CJK-Regular".
func Font(name string) ([]byte, bool) {
	b, err := fonts.ReadFile("fonts/KaTeX_" + name + ".ttf")
	return b, err == nil
}

// Fonts returns the names of the KaTeX fonts, as Items name them.
func Fonts() []string {
	entries, _ := fonts.ReadDir("fonts")
	var out []string
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(strings.TrimPrefix(e.Name(), "KaTeX_"), ".ttf"))
	}
	return out
}

// Engine lays out TeX. It is not safe for concurrent use: keep it to one
// goroutine.
type Engine struct {
	ctx     context.Context
	runtime wazero.Runtime
	mod     api.Module
	// errs are the values the module made for JavaScript: its errors, by
	// the index it was given.
	errs map[uint32]string
	next uint32

	malloc, free, stack, render api.Function
}

// importModule is the module wasm-bindgen imports JavaScript from.
const importModule = "./ratex_wasm_bg.js"

// New compiles RaTeX and starts an engine. Compiling takes a second or
// so the first time; with cacheDir, where wazero keeps what it compiled,
// later engines start in a few tens of milliseconds. "" keeps nothing.
func New(ctx context.Context, cacheDir string) (*Engine, error) {
	if sum := sha256.Sum256(module); hex.EncodeToString(sum[:]) != moduleSum {
		return nil, errors.New("ratex: ratex.wasm is not ratex-wasm@" + Version)
	}
	config := wazero.NewRuntimeConfig()
	if cacheDir != "" {
		if cache, err := wazero.NewCompilationCacheWithDir(cacheDir); err == nil {
			config = config.WithCompilationCache(cache)
		}
	}
	r := wazero.NewRuntimeWithConfig(ctx, config)
	e := &Engine{ctx: ctx, runtime: r, errs: map[uint32]string{}, next: 1 << 16}
	compiled, err := r.CompileModule(ctx, module)
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("ratex: %w", err)
	}
	// What the module imports from JavaScript. Laying out calls only
	// three: making a JavaScript string of its error, dropping it, and
	// throwing; the others, for the options object of the API this does
	// not use, fail if called. Their names end in a hash of wasm-bindgen's,
	// which another version changes, so they are matched by what is
	// before it.
	host := r.NewHostModuleBuilder(importModule)
	for _, imp := range compiled.ImportedFunctions() {
		mod, name, _ := imp.Import()
		if mod != importModule {
			r.Close(ctx)
			return nil, fmt.Errorf("ratex: unexpected import from %q", mod)
		}
		fn := e.imported(name)
		host.NewFunctionBuilder().WithGoModuleFunction(fn, imp.ParamTypes(), imp.ResultTypes()).Export(name)
	}
	if _, err := host.Instantiate(ctx); err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("ratex: %w", err)
	}
	e.mod, err = r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("ratex"))
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("ratex: %w", err)
	}
	e.malloc = e.mod.ExportedFunction("__wbindgen_export")
	e.free = e.mod.ExportedFunction("__wbindgen_export4")
	e.stack = e.mod.ExportedFunction("__wbindgen_add_to_stack_pointer")
	e.render = e.mod.ExportedFunction("renderLatex")
	if e.malloc == nil || e.free == nil || e.stack == nil || e.render == nil {
		r.Close(ctx)
		return nil, errors.New("ratex: the module lacks wasm-bindgen's exports")
	}
	return e, nil
}

// errThrown is a panic of the module's throw, which a call returns as its
// error.
type errThrown string

// imported returns the function of the import name.
func (e *Engine) imported(name string) api.GoModuleFunction {
	return api.GoModuleFunc(func(ctx context.Context, m api.Module, stack []uint64) {
		switch {
		case strings.HasPrefix(name, "__wbindgen_generic"):
			// A JavaScript string of the module's, its error.
			s, _ := m.Memory().Read(uint32(stack[0]), uint32(stack[1]))
			e.next++
			e.errs[e.next] = string(s)
			stack[0] = uint64(e.next)
		case name == "__wbindgen_object_drop_ref":
			delete(e.errs, uint32(stack[0]))
		case strings.HasPrefix(name, "__wbg___wbindgen_throw"):
			s, _ := m.Memory().Read(uint32(stack[0]), uint32(stack[1]))
			panic(errThrown(s))
		default:
			panic(errThrown("ratex: unexpected call of " + name))
		}
	})
}

// Layout lays out tex, in display style (a block of its own: larger
// operators, fractions in full) or text style (within a line). It
// returns TeX's error, as "Undefined control sequence: \foo", for TeX it
// cannot lay out.
func (e *Engine) Layout(tex string, display bool) (list *List, err error) {
	if strings.TrimSpace(tex) == "" || len(tex) > MaxSource {
		return nil, errors.New("ratex: no TeX, or too long")
	}
	defer func() {
		if r := recover(); r != nil {
			if s, ok := r.(errThrown); ok {
				list, err = nil, errors.New(string(s))
				return
			}
			panic(r)
		}
	}()
	res, err := e.stack.Call(e.ctx, api.EncodeI32(-16))
	if err != nil {
		return nil, err
	}
	sp := uint32(res[0])
	defer e.stack.Call(e.ctx, api.EncodeI32(16))
	res, err = e.malloc.Call(e.ctx, uint64(len(tex)), 1)
	if err != nil {
		return nil, err
	}
	ptr := uint32(res[0])
	mem := e.mod.Memory()
	mem.Write(ptr, []byte(tex))
	var mode uint64
	if display {
		mode = 1
	}
	// renderLatex(retptr, latex, color, displayMode): the color none, the
	// ink black, as Item.Color has it.
	if _, err := e.render.Call(e.ctx, uint64(sp), uint64(ptr), uint64(len(tex)), 0, 0, mode); err != nil {
		return nil, err
	}
	words, ok := mem.Read(sp, 16)
	if !ok {
		return nil, errors.New("ratex: no result")
	}
	word := func(i int) uint32 { return binary.LittleEndian.Uint32(words[i*4:]) }
	if word(3) != 0 {
		msg := e.errs[word(2)]
		delete(e.errs, word(2))
		return nil, errors.New(msg)
	}
	b, ok := mem.Read(word(0), word(1))
	if !ok {
		return nil, errors.New("ratex: no result")
	}
	list = new(List)
	err = json.Unmarshal(b, list)
	e.free.Call(e.ctx, uint64(word(0)), uint64(word(1)), 1)
	if err != nil {
		return nil, fmt.Errorf("ratex: %w", err)
	}
	return list, nil
}

// Close stops the engine.
func (e *Engine) Close() error { return e.runtime.Close(e.ctx) }
