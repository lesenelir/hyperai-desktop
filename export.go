package main

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"

	"hyperai-desktop/locales"
	"hyperai-desktop/ratex"
)

// A chat is exported, from its menu, to a file of one of three formats:
// a page (HTML) that holds all it shows, its images and the fonts of its
// math in it; the same page printed as a PDF, A4, by the system's web view
// (printPDF); and JSON, for programs. What is exported is the path shown,
// as Copy as Markdown copies it, its messages with versions marked as
// such (Version 2 of 3); the JSON holds the whole tree, every version of
// every message, each with the one it follows, and the path shown, by
// their IDs. The chat is read on the main thread,
// where it is safe to, and its math laid out there, where RaTeX is
// (exportDoc); the files are read and written on a goroutine.

// exportFormat is a format a chat is exported to.
type exportFormat int

const (
	exportPDF exportFormat = iota
	exportHTML
	exportJSON
)

// ext returns the extension of a file of f, without its dot.
func (f exportFormat) ext() string {
	return [...]string{"pdf", "html", "json"}[f]
}

// exportFile is a file sent with a question, as exported: its bytes, or
// where its copy is kept, read as the page is written.
type exportFile struct {
	Name string `json:"name"`
	Type string `json:"type"`
	data []byte
	path string
}

// exportMsg is a message as exported: its ID and the one it follows, ""
// for a chat's first, by which the JSON's tree holds together. body is a
// reply's Markdown as HTML, its math drawn (exportDoc), thought what the
// fold over its reasoning says, and version which of its versions it is,
// where it has others.
type exportMsg struct {
	ID         ID           `json:"id"`
	Parent     ID           `json:"parent"`
	Role       string       `json:"role"`
	Text       string       `json:"text"`
	Time       time.Time    `json:"time,omitzero"`
	Files      []exportFile `json:"files,omitempty"`
	Reasoning  string       `json:"reasoning,omitempty"`
	Error      string       `json:"error,omitempty"`
	Elapsed    float64      `json:"elapsed,omitempty"`
	FirstToken float64      `json:"firstToken,omitempty"`
	Usage      *usage       `json:"usage,omitempty"`
	body       string
	thought    string
	version    string
}

// exportDoc is a chat as exported, with what its page says in the app's
// language, and the KaTeX fonts its math uses. Messages are the path shown,
// for a page; for the JSON, every version of every message, oldest first,
// Path the IDs of those shown, in order.
type exportDoc struct {
	Title    string      `json:"title"`
	Model    string      `json:"model,omitempty"`
	Provider string      `json:"provider,omitempty"`
	Updated  time.Time   `json:"updated,omitzero"`
	Exported time.Time   `json:"exported"`
	Path     []ID        `json:"path"`
	Messages []exportMsg `json:"messages"`
	lang     string
	meta     string
	footer   string
	fonts    []string
	pal      *palette
}

// exportDoc returns th as exported, now: where page, the path shown, its
// replies' Markdown as HTML, its math laid out by RaTeX, where it started
// and the settings set math, and its messages' versions marked; else
// every version of every message, the tree the JSON holds.
func (a *app) exportDoc(th *thread, page bool) *exportDoc {
	now := time.Now()
	lang := i18n.Locale()
	when := i18n.FormatDate(lang, now, i18n.DateOptions{Date: i18n.LongStyle, Time: i18n.ShortStyle})
	d := &exportDoc{Title: th.Title, Model: th.Model, Provider: th.Provider, Updated: th.Updated, Exported: now,
		lang: lang, footer: locales.ExportFooter(when), pal: lightPalette}
	d.meta = th.Model
	if !th.Updated.IsZero() {
		updated := i18n.FormatDate(lang, th.Updated, i18n.DateOptions{Date: i18n.LongStyle, Time: i18n.ShortStyle})
		d.meta = strings.TrimPrefix(th.Model+" · "+updated, " · ")
	}
	fonts := map[string]bool{}
	msgs := th.Messages
	if !page {
		msgs = th.all()
		slices.SortFunc(msgs, func(x, y *message) int { return cmp.Compare(x.ID, y.ID) })
	}
	for _, m := range th.Messages {
		if !m.streaming {
			d.Path = append(d.Path, m.ID)
		}
	}
	for _, m := range msgs {
		if m.streaming {
			continue
		}
		e := exportMsg{ID: m.ID, Parent: m.Parent, Role: m.Role, Text: m.Text, Time: m.Time, Reasoning: strings.TrimSpace(m.Reasoning),
			Elapsed: m.Elapsed, FirstToken: m.FirstToken, Usage: m.Usage}
		if m.Error != "" {
			e.Error = m.errorText()
		}
		for _, f := range m.Files {
			ef := exportFile{Name: f.Name, Type: f.Type, data: f.data}
			if ef.data == nil && f.File != "" && a.filesDir != "" {
				ef.path = filepath.Join(a.filesDir, f.File)
			}
			e.Files = append(e.Files, ef)
		}
		if page && m.Role == "assistant" {
			e.body = a.markdownHTML(m.Text, fonts)
			if e.Reasoning != "" {
				e.thought = locales.ThoughtFor(seconds(m.Reasoned))
			}
		}
		if vs := th.versions(m); page && len(vs) > 1 {
			e.version = locales.VersionOf(slices.Index(vs, m)+1, len(vs))
		}
		d.Messages = append(d.Messages, e)
	}
	for f := range fonts {
		d.fonts = append(d.fonts, f)
	}
	slices.Sort(d.fonts)
	return d
}

// mathHTML finds the math the Markdown parser writes: within text, as
// spans, and its blocks.
var mathHTML = regexp.MustCompile(`(?s)<span class="math (inline|display)">(.*?)</span>|<div class="math display">(.*?)</div>`)

// markdownHTML returns src, Markdown as the replies are, as HTML: its math
// as SVG, laid out by RaTeX as the app lays it out, whose fonts it adds to
// fonts; or as its TeX, in code, where RaTeX did not start, the settings
// show LaTeX, or it cannot lay it out.
func (a *app) markdownHTML(src string, fonts map[string]bool) string {
	d := parser.New(a.md.Extensions)
	d.Set(src)
	return mathHTML.ReplaceAllStringFunc(d.HTML(), func(s string) string {
		g := mathHTML.FindStringSubmatch(s)
		escaped, display := g[2], g[1] == "display"
		if g[3] != "" || strings.HasPrefix(s, "<div") {
			escaped, display = g[3], true
		}
		tex := html.UnescapeString(escaped)
		if a.tex != nil && a.renderMath {
			if l, err := a.tex.Layout(tex, display); err == nil {
				svg := mathSVG(l, fonts)
				if display {
					return `<div class="math-display">` + svg + `</div>`
				}
				return svg
			}
		}
		if display {
			return `<pre class="tex"><code>` + escaped + `</code></pre>`
		}
		return `<code class="tex">` + escaped + `</code>`
	})
}

// mathSVG returns l as an SVG, in ems of the text around it, texScale of
// it as the app sets math, on the text's baseline: its glyphs in the KaTeX
// fonts, whose names it adds to fonts, but those they lack, in the page's;
// its rules and paths; in the text's color, or the color TeX gave them.
func mathSVG(l *ratex.List, fonts map[string]bool) string {
	n := func(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }
	ink := func(c ratex.Color) string {
		if c.Ink() {
			return "currentColor"
		}
		channel := func(v float64) int { return int(max(0, min(v, 1))*255 + 0.5) }
		return fmt.Sprintf("rgba(%d,%d,%d,%s)", channel(c.R), channel(c.G), channel(c.B), n(c.A))
	}
	h := l.Height + l.Depth
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="tex" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" style="width:%sem;height:%sem;vertical-align:-%sem">`,
		n(l.Width), n(h), n(l.Width*texScale), n(h*texScale), n(l.Depth*texScale))
	for _, it := range l.Items {
		switch it.Type {
		case "GlyphPath":
			family := ""
			if _, ok := ratex.Font(it.Font); ok {
				fonts[it.Font] = true
				family = fmt.Sprintf(` font-family="KaTeX_%s"`, it.Font)
			}
			fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="%s"%s fill="%s">%s</text>`,
				n(it.X), n(it.Y), n(it.Scale), family, ink(it.Color), html.EscapeString(string(it.Char)))
		case "Line":
			dash := ""
			if it.Dashed {
				dash = fmt.Sprintf(` stroke-dasharray="%s"`, n(it.Thickness*3))
			}
			fmt.Fprintf(&b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"%s/>`,
				n(it.X), n(it.Y), n(it.X+it.Width), n(it.Y), ink(it.Color), n(it.Thickness), dash)
		case "Rect":
			fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`,
				n(it.X), n(it.Y), n(it.Width), n(it.Height), ink(it.Color))
		case "Path":
			var d strings.Builder
			pt := func(x, y float64) string { return n(it.X+x) + " " + n(it.Y+y) }
			for _, c := range it.Commands {
				switch c.Type {
				case "MoveTo":
					d.WriteString("M" + pt(c.X, c.Y))
				case "LineTo":
					d.WriteString("L" + pt(c.X, c.Y))
				case "QuadTo":
					d.WriteString("Q" + pt(c.X1, c.Y1) + " " + pt(c.X, c.Y))
				case "CubicTo":
					d.WriteString("C" + pt(c.X1, c.Y1) + " " + pt(c.X2, c.Y2) + " " + pt(c.X, c.Y))
				case "Close":
					d.WriteString("Z")
				}
			}
			if it.Fill {
				fmt.Fprintf(&b, `<path d="%s" fill="%s"/>`, d.String(), ink(it.Color))
			} else {
				fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-width="0.04"/>`, d.String(), ink(it.Color))
			}
		}
	}
	b.WriteString("</svg>")
	return b.String()
}

// json returns d as JSON, indented, its files by name and type.
func (d *exportDoc) json() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

// css returns c as CSS.
func css(c ui.Color) string {
	return fmt.Sprintf("rgba(%d,%d,%d,%s)", c.R, c.G, c.B, strconv.FormatFloat(float64(c.A)/255, 'f', 3, 64))
}

// html returns d as a page that holds all it shows: its images as data, the
// KaTeX fonts of its math. It reads the files kept, on the goroutine it
// runs on. It runs no script and loads nothing (its Content Security
// Policy): what a reply wrote as HTML shows as the page lays it out, and
// no more.
func (d *exportDoc) html() string {
	p := d.pal
	fg := func(a float32) string { return css(p.fg.Alpha(a).Over(p.bg)) }
	var b strings.Builder
	fmt.Fprintf(&b, `<!DOCTYPE html>
<html lang="%s">
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; font-src data:; style-src 'unsafe-inline'">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
`, html.EscapeString(d.lang), html.EscapeString(d.Title))
	for _, f := range d.fonts {
		if ttf, ok := ratex.Font(f); ok {
			fmt.Fprintf(&b, "@font-face{font-family:\"KaTeX_%s\";src:url(data:font/ttf;base64,%s) format(\"truetype\")}\n",
				f, base64.StdEncoding.EncodeToString(ttf))
		}
	}
	fmt.Fprintf(&b, `:root{color-scheme:light}
body{margin:0;background:%[1]s;color:%[2]s;font:15px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Hiragino Sans","Noto Sans",sans-serif}
main{max-width:780px;margin:0 auto;padding:40px 24px}
header{margin-bottom:32px}
h1{font-size:24px;font-weight:600;margin:0 0 6px;line-height:1.3}
.meta,footer{font-size:12px;color:%[3]s}
footer{margin-top:40px;padding-top:16px;border-top:1px solid %[4]s}
.question{display:flex;flex-direction:column;align-items:flex-end;gap:8px;margin:28px 0 12px}
.bubble{background:%[5]s;border-radius:14px;padding:9px 14px;max-width:560px;white-space:pre-wrap;overflow-wrap:anywhere}
.files{display:flex;flex-wrap:wrap;justify-content:flex-end;gap:8px}
.files img{max-width:240px;max-height:240px;border-radius:12px;border:1px solid %[4]s}
.file{font-size:13px;padding:8px 12px;border-radius:12px;border:1px solid %[4]s;background:%[6]s}
.reply{margin:12px 0 28px;overflow-wrap:anywhere}
.reply>:first-child{margin-top:0}
.thoughts{margin:0 0 12px;font-size:13px;color:%[3]s}
.thoughts summary{cursor:default}
.thoughts div{white-space:pre-wrap;border-left:2px solid %[4]s;padding-left:12px;margin-top:8px}
.error{color:%[7]s;font-size:14px}
.version{font-size:12px;color:%[3]s;margin:0}
.reply .version{margin-top:8px}
a{color:%[8]s}
code{font:0.875em ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;color:%[8]s}
pre{background:%[6]s;border:1px solid %[4]s;border-radius:10px;padding:12px 14px;overflow-x:auto}
pre code{color:inherit}
blockquote{margin:0;padding-left:14px;border-left:2px solid %[4]s;color:%[3]s}
table{border-collapse:collapse}
th,td{border:1px solid %[4]s;padding:6px 10px}
img{max-width:100%%}
.math-display{text-align:center;overflow-x:auto;margin:12px 0}
svg.tex{overflow:visible}
@media print{body{background:none}main{padding:0}pre,.bubble,.files img,.math-display{break-inside:avoid}}
</style>
</head>
<body>
<main>
<header><h1>%[9]s</h1>`,
		css(p.fg.Alpha(0.025).Over(p.bg)), css(p.fg), fg(0.7), fg(0.12), css(p.raised), css(p.composer),
		css(p.theme.Danger), css(p.ac), html.EscapeString(d.Title))
	if d.meta != "" {
		fmt.Fprintf(&b, `<p class="meta">%s</p>`, html.EscapeString(d.meta))
	}
	b.WriteString("</header>\n")
	for _, m := range d.Messages {
		if m.Role == "user" {
			b.WriteString(`<section class="question">`)
			if len(m.Files) > 0 {
				b.WriteString(`<div class="files">`)
				for _, f := range m.Files {
					b.WriteString(f.html())
				}
				b.WriteString(`</div>`)
			}
			if m.Text != "" {
				fmt.Fprintf(&b, `<div class="bubble">%s</div>`, html.EscapeString(m.Text))
			}
			if m.version != "" {
				fmt.Fprintf(&b, `<p class="version">%s</p>`, html.EscapeString(m.version))
			}
			b.WriteString("</section>\n")
			continue
		}
		b.WriteString(`<section class="reply">`)
		if m.Reasoning != "" {
			fmt.Fprintf(&b, `<details class="thoughts"><summary>%s</summary><div>%s</div></details>`,
				html.EscapeString(m.thought), html.EscapeString(m.Reasoning))
		}
		b.WriteString(m.body)
		if m.Error != "" {
			fmt.Fprintf(&b, `<p class="error">%s</p>`, html.EscapeString(m.Error))
		}
		if m.version != "" {
			fmt.Fprintf(&b, `<p class="version">%s</p>`, html.EscapeString(m.version))
		}
		b.WriteString("</section>\n")
	}
	fmt.Fprintf(&b, "<footer>%s</footer>\n</main>\n</body>\n</html>\n", html.EscapeString(d.footer))
	return b.String()
}

// html returns f as the page shows it: an image as itself, its bytes as
// data, those of its copy kept read; another file by its name.
func (f exportFile) html() string {
	if slices.Contains(imageTypes, f.Type) {
		data := f.data
		if data == nil && f.path != "" {
			data, _ = os.ReadFile(f.path)
		}
		if data != nil {
			return fmt.Sprintf(`<img src="data:%s;base64,%s" alt="%s">`, f.Type, base64.StdEncoding.EncodeToString(data), html.EscapeString(f.Name))
		}
	}
	return `<span class="file">` + html.EscapeString(f.Name) + `</span>`
}

// exportName returns a file's name for a chat titled title, of ext: the
// title, of the characters a file system takes, else "Chat".
func exportName(title, ext string) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return '-'
		}
		return r
	}, strings.TrimSpace(title))
	name = strings.Trim(name, ". ")
	if r := []rune(name); len(r) > 80 {
		name = string(r[:80])
	}
	if name == "" {
		name = "Chat"
	}
	return name + "." + ext
}

// errNoPrinter is why a PDF cannot be made: no web view prints it (tests).
var errNoPrinter = errors.New("no web view to print the PDF")

// exportChat asks where to export th as f, in the system's dialog, from
// the Documents folder, named by its title; then writes it there, off the
// main thread, and says so in a toast, or why it could not.
func (a *app) exportChat(th *thread, f exportFormat) {
	save := a.saveFile
	if save == nil {
		return
	}
	doc := a.exportDoc(th, f != exportJSON)
	title, path, printPDF := locales.ExportTitle(), filepath.Join(a.exportDir, exportName(th.Title, f.ext())), a.printPDF
	a.async(func() func() {
		to := save(title, path, f.ext())
		if to == "" {
			return func() {}
		}
		var b []byte
		var err error
		switch f {
		case exportJSON:
			b, err = doc.json()
		case exportHTML:
			b = []byte(doc.html())
		case exportPDF:
			err = errNoPrinter
			if printPDF != nil {
				b, err = printPDF(doc.html())
			}
		}
		if err == nil {
			err = os.WriteFile(to, b, 0o644)
		}
		return func() {
			if err != nil {
				a.exportError, a.exported = err.Error(), ""
				return
			}
			a.exported, a.exportError = to, ""
		}
	})
}

// exportToast says, in a toast, where the chat exported went, with the
// button that shows it in the file manager, or why it could not be.
func (a *app) exportToast(c *ui.Context) {
	switch {
	case a.exportError != "":
		c.AddToast(ui.Toast{ID: "export", Title: locales.ExportFailed(), Description: a.exportError})
	case a.exported != "":
		to, reveal := a.exported, a.reveal
		toast := ui.Toast{ID: "export", Title: locales.ExportDone(), Description: filepath.Base(to)}
		if reveal != nil {
			toast.Action, toast.OnAction = locales.ShowInFolder(platform()), func() { reveal(to) }
		}
		c.AddToast(toast)
	default:
		return
	}
	a.exported, a.exportError = "", ""
}

// exportMenu fills m, a chat's menu, with Export and the formats under it.
func (a *app) exportMenu(m *ui.Menu, th *thread) {
	m.Submenu(locales.ExportChat(), func(sub *ui.Menu) {
		for _, f := range []struct {
			label  string
			format exportFormat
		}{{locales.ExportPDF(), exportPDF}, {locales.ExportHTML(), exportHTML}, {locales.ExportJSON(), exportJSON}} {
			if sub.Item(f.label).Chosen() {
				a.exportChat(th, f.format)
			}
		}
	}).Disabled(a.saveFile == nil || len(th.Messages) == 0)
}
