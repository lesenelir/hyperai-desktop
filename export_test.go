package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// exportChat is a chat to export: a question with an image and markup in
// it, a reply of Markdown, math and HTML, with its reasoning and usage.
func exportChat(t *testing.T) (*app, *ui.Tester, *thread, string) {
	img, _, _, _ := testFiles(t)
	a := newApp(&counter{})
	tt := ui.NewTester(a.view, 1120, 760)
	a.attach([]string{img})
	a.send(`What is <b>this</b>?`)
	th := a.current()
	th.Title = `A/B "test"`
	reply := th.Messages[1]
	reply.Text = "It is **bold**, with $x^2$ and\n\n$$\\frac{a}{b}$$\n\n<script>alert(1)</script>"
	reply.Reasoning, reply.Reasoned = "Look at it.", 2
	reply.Usage = &usage{Input: 10, Output: 5}
	dir := t.TempDir()
	a.exportDir = dir
	return a, tt, th, dir
}

// The file's name is the chat's title, of what file systems take.
func TestExportName(t *testing.T) {
	for title, want := range map[string]string{
		`A/B "test"`:  `A-B -test-.pdf`,
		"  ":          "Chat.pdf",
		"缓存设计":        "缓存设计.pdf",
		"..hidden..":  "hidden.pdf",
		"line\nbreak": "line-break.pdf",
	} {
		if got := exportName(title, "pdf"); got != want {
			t.Errorf("exportName(%q) = %q, want %q", title, got, want)
		}
	}
}

// Exported as JSON: the path shown, its files by name and type, and what
// each reply took.
func TestExportJSON(t *testing.T) {
	a, tt, th, dir := exportChat(t)
	asked := ""
	to := filepath.Join(dir, "out.json")
	a.saveFile = func(title, path, ext string) string {
		asked = title + " | " + filepath.Base(path) + " | " + ext
		return to
	}
	a.exportChat(th, exportJSON)
	if asked != locales.ExportTitle()+" | A-B -test-.json | json" {
		t.Errorf("asked %q", asked)
	}
	b, err := os.ReadFile(to)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Title    string
		Messages []struct {
			Role, Text, Reasoning string
			Files                 []struct{ Name, Type string }
			Usage                 *usage
		}
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != `A/B "test"` || len(got.Messages) != 2 || got.Messages[0].Text != "What is <b>this</b>?" ||
		len(got.Messages[0].Files) != 1 || got.Messages[0].Files[0].Type != "image/png" ||
		got.Messages[1].Reasoning != "Look at it." || got.Messages[1].Usage == nil || got.Messages[1].Usage.Output != 5 {
		t.Errorf("exported %s", b)
	}
	if strings.Contains(string(b), "base64") {
		t.Error("the JSON holds the files' bytes")
	}
	tt.Frame()
	if !tt.HasText(locales.ExportDone()) || !tt.HasText("out.json") {
		t.Errorf("no toast in %q", tt.Texts())
	}
}

// Exported as a page: whole, its image in it, its question as written, its
// reply's Markdown laid out, its math drawn in the fonts it carries, and
// what the reply wrote as HTML kept from running.
func TestExportHTML(t *testing.T) {
	a, _, th, dir := exportChat(t)
	a.tex = startedTeX(t)
	to := filepath.Join(dir, "out.html")
	a.saveFile = func(title, path, ext string) string { return to }
	a.exportChat(th, exportHTML)
	b, err := os.ReadFile(to)
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	for _, want := range []string{
		`<!DOCTYPE html>`,
		`<title>A/B &#34;test&#34;</title>`,
		`content="default-src 'none'; img-src data:; font-src data:; style-src 'unsafe-inline'"`,
		`<img src="data:image/png;base64,`,
		`What is &lt;b&gt;this&lt;/b&gt;?`,
		`<strong>bold</strong>`,
		`<svg class="tex"`,
		`<div class="math-display"><svg class="tex"`,
		`@font-face{font-family:"KaTeX_Main-Regular";src:url(data:font/ttf;base64,`,
		`<summary>` + locales.ThoughtFor(seconds(2)) + `</summary>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("no %q in the page", want)
		}
	}
	if strings.Contains(page, "$x^2$") || strings.Contains(page, `class="math inline"`) {
		t.Error("the math shows as TeX")
	}

	// Math as LaTeX, as the settings may show it.
	a.renderMath = false
	a.exportChat(th, exportHTML)
	b, _ = os.ReadFile(to)
	if !strings.Contains(string(b), `<code class="tex">x^2</code>`) || strings.Contains(string(b), "@font-face") {
		t.Error("math as LaTeX is not code alone")
	}
}

// Exported as a PDF: the page, printed; where nothing prints it, a toast
// says why. A dialog closed exports nothing.
func TestExportPDF(t *testing.T) {
	a, tt, th, dir := exportChat(t)
	to := filepath.Join(dir, "out.pdf")
	a.saveFile = func(title, path, ext string) string { return to }
	var printed string
	a.printPDF = func(page string) ([]byte, error) { printed = page; return []byte("%PDF-1.7"), nil }
	a.exportChat(th, exportPDF)
	if b, err := os.ReadFile(to); err != nil || string(b) != "%PDF-1.7" || !strings.Contains(printed, "<strong>bold</strong>") {
		t.Errorf("wrote %q, %v; printed %d bytes", b, err, len(printed))
	}

	a.printPDF = nil
	os.Remove(to)
	a.exportChat(th, exportPDF)
	tt.Frame()
	if _, err := os.Stat(to); err == nil || !tt.HasText(locales.ExportFailed()) {
		t.Errorf("without a printer: file %v; texts %q", err, tt.Texts())
	}

	a.saveFile = func(title, path, ext string) string { return "" }
	a.exportChat(th, exportJSON)
	if _, err := os.Stat(filepath.Join(dir, "out.json")); err == nil {
		t.Error("exported with the dialog closed")
	}
}

// Export is in a chat's menu, its formats under it.
func TestExportMenu(t *testing.T) {
	a, tt, th, dir := exportChat(t)
	to := filepath.Join(dir, "menu.json")
	a.saveFile = func(title, path, ext string) string { return to }
	tt.Frame()
	if err := tt.Click(locales.More()); err != nil {
		t.Fatal(err)
	}
	if got := tt.Menu(); !slices.Contains(got, locales.ExportChat()) {
		t.Fatalf("menu %q", got)
	}
	if err := tt.ChooseMenuItem(locales.ExportChat(), locales.ExportJSON()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if _, err := os.Stat(to); err != nil {
		t.Errorf("not exported from the menu: %v", err)
	}
	_ = th
}

// A chat of versions: its JSON holds the whole tree, every version, each by
// the one it follows, and the path shown; its page, the path shown, its
// messages of versions marked as such.
func TestExportVersions(t *testing.T) {
	a := newApp(&counter{})
	a.send("Q1")
	th := a.current()
	a.regenerate(th, 1)         // a reply beside the first
	a.resend(th, 0, "Q1 again") // a question beside the first, and its reply
	dir := t.TempDir()
	to := filepath.Join(dir, "tree.json")
	a.saveFile = func(title, path, ext string) string { return to }
	a.exportChat(th, exportJSON)
	b, err := os.ReadFile(to)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Path     []int
		Messages []struct {
			ID, Parent int
			Text       string
		}
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	var tree []string
	for _, m := range got.Messages {
		tree = append(tree, fmt.Sprintf("%d<%d %s", m.ID, m.Parent, m.Text))
	}
	if want := "2<0 Q1, 3<2 Answer 1, 4<2 Answer 2, 5<0 Q1 again, 6<5 Answer 3"; strings.Join(tree, ", ") != want {
		t.Errorf("tree %q, want %q", strings.Join(tree, ", "), want)
	}
	if !slices.Equal(got.Path, []int{5, 6}) {
		t.Errorf("path %v", got.Path)
	}

	page := a.exportDoc(th, true).html()
	if !strings.Contains(page, `<div class="bubble">Q1 again</div><p class="version">`+locales.VersionOf(2, 2)+`</p>`) ||
		strings.Count(page, `class="version"`) != 1 || strings.Contains(page, "Answer 1") {
		t.Errorf("the page of the second question: %s", page[strings.Index(page, "<main>"):])
	}
	// Back to the first question: its version, and its reply's, the second
	// asked for, shown last.
	a.showVersion(th, 0, th.versions(th.Messages[0])[0])
	page = a.exportDoc(th, true).html()
	if !strings.Contains(page, `<div class="bubble">Q1</div><p class="version">`+locales.VersionOf(1, 2)+`</p>`) ||
		!strings.Contains(page, "Answer 2") || !strings.Contains(page, `<p class="version">`+locales.VersionOf(2, 2)+`</p></section>`) {
		t.Errorf("the page of the first question: %s", page[strings.Index(page, "<main>"):])
	}
}
