package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// testFiles writes, in a folder of the test's, an image, a text file, a
// file of no kind sent and one too large, and returns their paths.
func testFiles(t *testing.T) (img, text, binary, large string) {
	dir := t.TempDir()
	var b bytes.Buffer
	pic := image.NewRGBA(image.Rect(0, 0, 4, 4))
	pic.Set(1, 1, color.RGBA{R: 255, A: 255})
	png.Encode(&b, pic)
	img, text, binary, large = filepath.Join(dir, "shot.png"), filepath.Join(dir, "notes.md"),
		filepath.Join(dir, "clip.bin"), filepath.Join(dir, "big.txt")
	for _, f := range []struct {
		path string
		data []byte
	}{{img, b.Bytes()}, {text, []byte("# Notes\n\nCache the user.")}, {binary, []byte{0x00, 0x01, 0xff, 0xfe}}} {
		if err := os.WriteFile(f.path, f.data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(large, nil, 0o600); err != nil || os.Truncate(large, (maxAttachMB+1)<<20) != nil {
		t.Fatal("writing the large file")
	}
	return img, text, binary, large
}

// Files attached through the composer's button show over the field, which
// opens into a box; those that cannot be say why; one is taken off by its
// ✕.
func TestAttachFiles(t *testing.T) {
	img, text, binary, large := testFiles(t)
	a, tt := newTester()
	asked := ""
	a.openFiles = func(title string) []string {
		asked = title
		return []string{img, text, binary, large}
	}
	if !a.pill {
		t.Fatal("no pill on an empty chat")
	}
	if err := tt.Click(locales.AttachFiles()); err != nil {
		t.Fatal(err)
	}
	if asked != locales.AttachFiles() {
		t.Errorf("dialog titled %q", asked)
	}
	if len(a.files) != 2 || a.files[0].Name != "shot.png" || a.files[0].Type != "image/png" ||
		a.files[1].Name != "notes.md" || a.files[1].Type != typeText {
		t.Fatalf("attached %+v", a.files)
	}
	if a.pill {
		t.Error("the composer stays a pill with files")
	}
	if !tt.HasText("notes.md") || !tt.HasText("MD") {
		t.Errorf("no chip of the text file in %q", tt.Texts())
	}
	for _, want := range []string{locales.AttachFailed("clip.bin"), locales.AttachUnsupported(),
		locales.AttachFailed("big.txt"), locales.AttachTooLarge(maxAttachMB)} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if a.files[0].thumb == nil {
		t.Error("the image has no picture")
	}
	if err := tt.Click(locales.AttachRemove("notes.md")); err != nil {
		t.Fatal(err)
	}
	if len(a.files) != 1 || tt.HasText("notes.md") {
		t.Errorf("after removing: %+v", a.files)
	}
}

// Files dropped on the chat are attached; while they are dragged over it,
// the composer says it takes them.
func TestDropFiles(t *testing.T) {
	_, text, _, _ := testFiles(t)
	a, tt := newTester()
	r, ok := tt.Find(locales.ComposerPlaceholder())
	if !ok {
		t.Fatal("no composer")
	}
	// Over the greeting, above the composer: the chat takes them too.
	if !tt.DropFiles(r.X+20, r.Y-120, text) {
		t.Fatal("the chat did not take the files")
	}
	if len(a.files) != 1 || a.files[0].Name != "notes.md" {
		t.Fatalf("attached %+v", a.files)
	}
	if a.fileOver || tt.HasText(locales.AttachDrop()) {
		t.Error("the composer shows files over it once dropped")
	}
	// The sidebar does not.
	if tt.DropFiles(40, 300, text) || len(a.files) != 1 {
		t.Errorf("the sidebar took files: %+v", a.files)
	}
}

// A question goes with its files, which show over it, and the provider is
// asked them; with no text, the chat is titled by the first. The files
// attached stay with the chat they were attached in.
func TestSendFiles(t *testing.T) {
	img, text, _, _ := testFiles(t)
	p := &thinker{}
	a := newApp(p)
	tt := ui.NewTester(a.view, 1120, 760)
	a.attach([]string{img, text})
	tt.Frame()
	if err := tt.Click(locales.Send()); err != nil {
		t.Fatal(err)
	}
	th := a.current()
	if th == nil || th.Title != "shot.png" || len(a.files) != 0 {
		t.Fatalf("chat %+v, draft's files %+v", th, a.files)
	}
	q := th.Messages[0]
	if q.Text != "" || len(q.Files) != 2 {
		t.Fatalf("question %+v", q)
	}
	if len(p.asked) != 1 || len(p.asked[0][0].Files) != 2 || string(p.asked[0][0].Files[1].data) != "# Notes\n\nCache the user." {
		t.Fatalf("asked %+v", p.asked)
	}
	if !tt.HasText("notes.md") || !tt.HasText("Hi there.") {
		t.Errorf("no file or reply in %q", tt.Texts())
	}

	// Attached in this chat, then left for a new one, and back.
	a.attach([]string{text})
	a.newChat()
	tt.Frame()
	if len(a.files) != 0 {
		t.Errorf("the new chat has the other's files: %+v", a.files)
	}
	a.goBack()
	tt.Frame()
	if len(a.files) != 1 {
		t.Errorf("the files did not stay with their chat: %+v", a.files)
	}
}

// The files sent are kept in the app's folder, for the conversation to
// send them again after it restarts, and go with their chat.
func TestKeepFiles(t *testing.T) {
	_, text, _, _ := testFiles(t)
	dir := t.TempDir()
	a := newApp(&thinker{})
	a.loadFrom(dir)
	tt := ui.NewTester(a.view, 1120, 760)
	a.attach([]string{text})
	a.send("Summarize")
	a.saver.wait()
	a.writes.Wait()
	f := a.current().Messages[0].Files[0]
	kept := filepath.Join(dir, "attachments", f.File)
	if b, err := os.ReadFile(kept); err != nil || !strings.HasSuffix(f.File, ".md") || string(b) != "# Notes\n\nCache the user." {
		t.Fatalf("kept %q: %q, %v", f.File, b, err)
	}

	again := newApp(&thinker{})
	again.loadFrom(dir)
	turns := again.threads[0].turns()
	if turns[0].Files[0].data != nil {
		t.Fatal("the bytes were in memory")
	}
	if err := readFiles(again.filesDir, turns); err != nil || string(turns[0].Files[0].data) != "# Notes\n\nCache the user." {
		t.Fatalf("read %q, %v", turns[0].Files[0].data, err)
	}

	a.remove(a.current())
	a.writes.Wait()
	if _, err := os.Stat(kept); !os.IsNotExist(err) {
		t.Errorf("the copy stayed with its chat deleted: %v", err)
	}
	tt.Frame()
}

// Each API is given the files as it takes them.
func TestFilesAsProvidersTakeThem(t *testing.T) {
	q := turn{Role: "user", Content: "What is this?", Files: []attachment{
		{Name: "a.png", Type: "image/png", data: []byte("png")},
		{Name: "b.pdf", Type: typePDF, data: []byte("pdf")},
		{Name: "c.md", Type: typeText, data: []byte("# C")},
	}}
	b, _ := json.Marshal(openAIMessages([]turn{{Role: "assistant", Content: "Hi"}, q}))
	want := `[{"content":"Hi","role":"assistant"},{"content":[` +
		`{"image_url":{"url":"data:image/png;base64,cG5n"},"type":"image_url"},` +
		`{"file":{"file_data":"data:application/pdf;base64,cGRm","filename":"b.pdf"},"type":"file"},` +
		`{"text":"\u003cfile name=\"c.md\"\u003e\n# C\n\u003c/file\u003e","type":"text"},` +
		`{"text":"What is this?","type":"text"}],"role":"user"}]`
	if string(b) != want {
		t.Errorf("OpenAI's:\n%s\nwant\n%s", b, want)
	}

	b, _ = json.Marshal(anthropicBlocks(q))
	want = `[{"source":{"data":"cG5n","media_type":"image/png","type":"base64"},"type":"image"},` +
		`{"source":{"data":"cGRm","media_type":"application/pdf","type":"base64"},"title":"b.pdf","type":"document"},` +
		`{"text":"\u003cfile name=\"c.md\"\u003e\n# C\n\u003c/file\u003e","type":"text"},` +
		`{"text":"What is this?","type":"text"}]`
	if string(b) != want {
		t.Errorf("Anthropic's:\n%s\nwant\n%s", b, want)
	}

	system, prompt, files := copilotPrompt([]turn{
		{Role: "user", Content: "Look", Files: []attachment{{Name: "old.png", Type: "image/png"}}},
		{Role: "assistant", Content: "Seen."}, q})
	if !strings.HasSuffix(system, `User: <file name="old.png" type="image/png"/>`+"\n\nLook\n\nAssistant: Seen.") {
		t.Errorf("Copilot's system: %q", system)
	}
	if prompt != "<file name=\"a.png\" type=\"image/png\"/>\n\n<file name=\"b.pdf\" type=\"application/pdf\"/>\n\n<file name=\"c.md\">\n# C\n</file>\n\nWhat is this?" ||
		len(files) != 2 || files[0].Name != "a.png" || files[1].Name != "b.pdf" {
		t.Errorf("Copilot's prompt %q, files %+v", prompt, files)
	}
}

// An image's picture keeps no more pixels than its chips show, upright.
func TestThumbnail(t *testing.T) {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1000, 500)))
	img, err := decodeThumb(b.Bytes())
	if err != nil || img.Rect.Dx() != 2*thumbPixels || img.Rect.Dy() != thumbPixels {
		t.Fatalf("picture %v, %v", img.Rect, err)
	}
	// Turned a quarter clockwise, as a phone's photo held upright says.
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.Set(0, 0, color.RGBA{R: 255, A: 255})
	up := orient(src, 6)
	if up.Rect.Dx() != 1 || up.Rect.Dy() != 2 || up.RGBAAt(0, 0).R != 255 || up.RGBAAt(0, 1).R != 0 {
		t.Errorf("oriented %v: %v, %v", up.Rect, up.RGBAAt(0, 0), up.RGBAAt(0, 1))
	}
}

// Pasting attaches a screenshot copied, and files copied in Finder, but
// pastes text that comes with a picture of itself as text.
func TestPasteFiles(t *testing.T) {
	img, text, _, _ := testFiles(t)
	shot, _ := os.ReadFile(img)
	a, tt := newTester()
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, shot))))
	tt.Key(ui.Cmd, ui.KeyV)
	if len(a.files) != 1 || a.files[0].Name != pastedName || a.files[0].Type != "image/png" || a.draft != "" {
		t.Fatalf("pasted the image: %+v, draft %q", a.files, a.draft)
	}
	uri := filepath.ToSlash(text)
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri // C:/…, on Windows
	}
	tt.SetClipboardData(transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.FileList, []byte("file://"+uri)),
		transfer.Bytes(transfer.Text, []byte("notes.md")))))
	tt.Key(ui.Cmd, ui.KeyV)
	if len(a.files) != 2 || a.files[1].Name != "notes.md" || a.draft != "" {
		t.Fatalf("pasted the file: %+v, draft %q", a.files, a.draft)
	}
	tt.SetClipboardData(transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.Text, []byte("Cache it")), transfer.Bytes(transfer.PNG, shot))))
	tt.Key(ui.Cmd, ui.KeyV)
	if len(a.files) != 2 || a.draft != "Cache it" {
		t.Errorf("pasted the text: %+v, draft %q", a.files, a.draft)
	}
}
