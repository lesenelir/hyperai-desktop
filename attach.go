package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // decodeThumb
	_ "image/jpeg" // decodeThumb
	_ "image/png"  // decodeThumb
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // decodeThumb

	"hyperai-desktop/locales"
)

// attachment is a file sent with a question, or to be sent with the draft,
// as Codex attaches them: an image, a PDF or a text file, which goes to
// the provider as its API takes it. Name is the file's name as the user
// had it, Type its media type (attachTypes), and File the name of the
// copy kept in the app's attachments folder as the question is sent
// (keepFiles), "" before and in tests. data is its bytes, read as it was
// attached, nil for one sent before the app started, until a reply reads
// them again (readFiles).
type attachment struct {
	Name string `json:"name"`
	Type string `json:"type"`
	File string `json:"file,omitempty"`
	data []byte
	// thumb is an image's picture as its chip shows it, read once
	// (thumbnail), broken where it could not be.
	thumb    *ui.Bitmap
	thumbing bool
	broken   bool
}

// The media types a file can be attached as: the images every provider
// that sees them takes, PDF, and text, of any file that is UTF-8.
const (
	typePDF  = "application/pdf"
	typeText = "text/plain"
)

var imageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

func (f *attachment) image() bool { return slices.Contains(imageTypes, f.Type) }

// The size of a file attached at most: a text file goes in the
// conversation as it is, which a megabyte of already fills.
const (
	maxAttachMB = 20
	maxTextMB   = 1
)

// attachFailure is why a file could not be attached, which the composer
// says in a toast: too large (over max MB), not of a kind that is sent,
// or not read.
type attachFailure struct {
	name        string
	max         int
	unsupported bool
}

func (e *attachFailure) Error() string { return "can't attach " + e.name }

// readAttachment reads the file at path, as it is attached; an
// *attachFailure where it cannot be.
func readAttachment(path string) (*attachment, error) {
	name := filepath.Base(path)
	fi, err := os.Stat(path)
	switch {
	case err != nil:
		return nil, &attachFailure{name: name}
	case !fi.Mode().IsRegular():
		return nil, &attachFailure{name: name, unsupported: true}
	case fi.Size() > maxAttachMB<<20:
		return nil, &attachFailure{name: name, max: maxAttachMB}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &attachFailure{name: name}
	}
	return newAttachment(name, data)
}

// newAttachment returns data, of a file called name, as it is attached;
// an *attachFailure where it cannot be.
func newAttachment(name string, data []byte) (*attachment, error) {
	if len(data) > maxAttachMB<<20 {
		return nil, &attachFailure{name: name, max: maxAttachMB}
	}
	f := &attachment{Name: name, data: data}
	switch t := http.DetectContentType(data); {
	case slices.Contains(imageTypes, t), t == typePDF:
		f.Type = t
	case utf8.Valid(data) && bytes.IndexByte(data, 0) < 0:
		if len(data) > maxTextMB<<20 {
			return nil, &attachFailure{name: name, max: maxTextMB}
		}
		f.Type = typeText
	default:
		return nil, &attachFailure{name: name, unsupported: true}
	}
	return f, nil
}

// async runs work off the main thread, and then what it returns on it:
// both at once in tests, which have no window to post to.
func (a *app) async(work func() func()) {
	if a.post == nil {
		work()()
		return
	}
	post := a.post
	go func() {
		done := work()
		post(done)
	}()
}

// pickFiles asks for files to attach to the draft, in the system's dialog.
func (a *app) pickFiles() {
	open := a.openFiles
	if open == nil {
		return
	}
	title := locales.AttachFiles()
	a.async(func() func() {
		paths := open(title)
		return func() { a.attach(paths) }
	})
}

// attach reads the files at paths, off the main thread, and adds those it
// can to the draft of the chat they were attached in, though another may
// show by then; those it cannot, the composer says why.
func (a *app) attach(paths []string) {
	if len(paths) == 0 {
		return
	}
	id := a.selected
	a.async(func() func() {
		var files []*attachment
		var failed []*attachFailure
		for _, p := range paths {
			f, err := readAttachment(p)
			var why *attachFailure
			if errors.As(err, &why) {
				failed = append(failed, why)
				continue
			}
			files = append(files, f)
		}
		return func() {
			a.attachFailed = append(a.attachFailed, failed...)
			if id != a.selected {
				if th := a.thread(id); th != nil {
					th.files = append(th.files, files...)
				} else if id == "" {
					a.newFiles = append(a.newFiles, files...)
				}
				return
			}
			a.files = append(a.files, files...)
			// Back to writing, the dialog closed.
			a.focusDraft = true
		}
	})
}

// pastedName is what an image pasted is called, as the system calls a
// picture it holds no file of.
const pastedName = "image.png"

// paste attaches what the clipboard holds that is not text to paste, and
// reports whether it did, which leaves the field as it was: files copied
// in Finder or Explorer, whose names it holds as text too; or an image, as
// a screenshot copied, where it holds no text, since a browser's selection
// copied comes with a picture of itself.
func (a *app) paste(svc ui.Services, text string) bool {
	formats := svc.ClipboardFormats()
	if slices.Contains(formats, transfer.FileList) {
		if paths, err := svc.ReadClipboardData(transfer.FileList).Files(); err == nil && len(paths) > 0 {
			a.attach(paths)
			return true
		}
	}
	if text != "" || !slices.Contains(formats, transfer.PNG) {
		return false
	}
	data, err := svc.ReadClipboardData(transfer.PNG).Read(transfer.PNG)
	if err != nil {
		return false
	}
	f, err := newAttachment(pastedName, data)
	var why *attachFailure
	if errors.As(err, &why) {
		a.attachFailed = append(a.attachFailed, why)
		return true
	}
	a.files = append(a.files, f)
	return true
}

// detach takes f off the draft.
func (a *app) detach(f *attachment) {
	a.files = slices.DeleteFunc(slices.Clone(a.files), func(g *attachment) bool { return g == f })
}

// keepFiles keeps a copy of files in the attachments folder, where the app
// has one, for the conversation to send them again after the app restarts:
// named anew, as two may have the same name. The copies are written on
// goroutines, which the app waits for as it quits (writes).
func (a *app) keepFiles(files []*attachment) {
	if a.filesDir == "" {
		return
	}
	for _, f := range files {
		var id [8]byte
		rand.Read(id[:])
		f.File = hex.EncodeToString(id[:]) + strings.ToLower(filepath.Ext(f.Name))
		dir, name, data, writes := a.filesDir, f.File, f.data, &a.writes
		writes.Add(1)
		go func() {
			defer writes.Done()
			err := os.MkdirAll(dir, 0o700)
			if err == nil {
				err = os.WriteFile(filepath.Join(dir, name), data, 0o600)
			}
			if err != nil {
				log.Printf("keeping an attachment: %v", err)
			}
		}()
	}
}

// forgetFiles deletes the copies kept of the files of threads, which are
// deleted. Not where the chats are not saved (a.path is ""), as after a
// migration that failed: chats deleted then come back at the next launch,
// and their files must still be there.
func (a *app) forgetFiles(threads ...*thread) {
	if a.filesDir == "" || a.path == "" {
		return
	}
	var paths []string
	for _, th := range threads {
		for _, m := range th.all() {
			for _, f := range m.Files {
				if f.File != "" {
					paths = append(paths, filepath.Join(a.filesDir, f.File))
				}
			}
		}
	}
	if len(paths) == 0 {
		return
	}
	writes := &a.writes
	writes.Add(1)
	go func() {
		defer writes.Done()
		for _, p := range paths {
			os.Remove(p)
		}
	}()
}

// readFiles reads, from dir, the files of turns sent before the app
// started, which it does not hold: on the goroutine a reply streams on.
func readFiles(dir string, turns []turn) error {
	for i := range turns {
		for j := range turns[i].Files {
			f := &turns[i].Files[j]
			if f.data != nil {
				continue
			}
			if f.File == "" || dir == "" {
				return fmt.Errorf("the attachment %s was not kept", f.Name)
			}
			data, err := os.ReadFile(filepath.Join(dir, f.File))
			if err != nil {
				return fmt.Errorf("reading the attachment %s: %w", f.Name, err)
			}
			f.data = data
		}
	}
	return nil
}

// fileText returns a text file f as it goes in a message, after its name.
func fileText(f attachment) string {
	return fmt.Sprintf("<file name=%q>\n%s\n</file>", f.Name, f.data)
}

// dataURL returns f as a data URL, as OpenAI's API takes an image or a
// file.
func dataURL(f attachment) string {
	return "data:" + f.Type + ";base64," + base64.StdEncoding.EncodeToString(f.data)
}

// thumbnail returns the picture of the image f, nil until it is read, of
// its bytes or its copy kept, off the main thread.
func (a *app) thumbnail(f *attachment) *ui.Bitmap {
	if f.thumb != nil || f.thumbing || !f.image() {
		return f.thumb
	}
	f.thumbing = true
	data, path := f.data, ""
	if f.File != "" && a.filesDir != "" {
		path = filepath.Join(a.filesDir, f.File)
	}
	a.async(func() func() {
		if data == nil && path != "" {
			data, _ = os.ReadFile(path)
		}
		img, err := decodeThumb(data)
		return func() {
			// One that cannot be read shows as a file.
			if f.broken = err != nil; err == nil {
				f.thumb = ui.NewBitmap(img)
			}
		}
	})
	return f.thumb
}

// thumbPixels is how many pixels the short side of an image's picture
// keeps: as many as its chip in a message shows on a display of 3×.
const thumbPixels = 3 * sentImageSize

// decodeThumb decodes the image data as its chips show it: upright, as a
// JPEG's EXIF says, and no larger than they show it, where a photo kept
// whole would hold tens of megabytes of pixels to show a few thousand.
func decodeThumb(data []byte) (*image.RGBA, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if short := min(w, h); short > thumbPixels {
		w, h = max(1, w*thumbPixels/short), max(1, h*thumbPixels/short)
	}
	small := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(small, small.Bounds(), img, b, draw.Src, nil)
	return orient(small, exifOrientation(data)), nil
}

// exifOrientation returns the orientation of a JPEG's EXIF data, 1 to 8,
// or 0 without one, as MyGo's DecodeBitmap reads it.
func exifOrientation(jpg []byte) int {
	if len(jpg) < 4 || jpg[0] != 0xff || jpg[1] != 0xd8 {
		return 0
	}
	for i := 2; i+4 <= len(jpg); {
		if jpg[i] != 0xff {
			return 0
		}
		marker, size := jpg[i+1], int(binary.BigEndian.Uint16(jpg[i+2:]))
		if marker == 0xda || size < 2 || i+2+size > len(jpg) { // the image data starts
			return 0
		}
		if seg := jpg[i+4 : i+2+size]; marker == 0xe1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 0
}

// tiffOrientation reads the Orientation tag of the first IFD of TIFF data.
func tiffOrientation(t []byte) int {
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	ifd := int(bo.Uint32(t[4:]))
	if ifd < 8 || ifd+2 > len(t) {
		return 0
	}
	n := int(bo.Uint16(t[ifd:]))
	for e := ifd + 2; e+12 <= len(t) && n > 0; e, n = e+12, n-1 {
		if bo.Uint16(t[e:]) == 0x0112 && bo.Uint16(t[e+2:]) == 3 { // Orientation, a SHORT
			if o := int(bo.Uint16(t[e+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 0
		}
	}
	return 0
}

// orient turns and flips img as EXIF orientation o says it is to show.
func orient(img *image.RGBA, o int) *image.RGBA {
	if o <= 1 {
		return img
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		for x := range dw {
			sx, sy := x, y
			switch o {
			case 2:
				sx, sy = w-1-x, y
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			}
			copy(out.Pix[out.PixOffset(x, y):out.PixOffset(x, y)+4], img.Pix[img.PixOffset(sx, sy):])
		}
	}
	return out
}

// The chips of files attached are chipSize tall, an image's as wide, as
// Codex's are, as far in from the box's top as from its left, where the
// field's text starts, and about as far over the text (filesPadTop); a
// message's images show larger, sentImageSize square.
const (
	chipSize      = 56
	chipsPadTop   = fieldPadX
	sentImageSize = 120
	chipRadius    = 12
)

// attachButton builds the composer's button that attaches files: round as
// the send button, muted as the model's, a wash under the pointer.
func (a *app) attachButton(c *ui.Context) ui.Element {
	t := c.Theme()
	label := locales.AttachFiles()
	b := tooltip(c, ui.ButtonBase(c.Key("attach")).Size(sendSize, sendSize).Radius(sendSize/2).
		TextColor(t.TextMuted).Label(label), label)
	if b.Hovered() {
		b.Background(a.pal.fg.Alpha(0.06)).TextColor(t.Text)
	}
	b.Children(func() { ui.Icon(c, plusIcon).Size(16, 16) })
	if b.Clicked() {
		a.pickFiles()
	}
	return b
}

// draftFiles builds the files attached to the draft, over the field, in a
// row that scrolls sideways where they are more than fit: always, empty
// with none, so that the field after it keeps its place in the tree.
func (a *app) draftFiles(c *ui.Context) {
	row := ui.ScrollHorizontal(c)
	if len(a.files) == 0 {
		return
	}
	row.Padding(chipsPadTop, fieldPadX, 0, fieldPadX).Gap(8).Label(locales.AttachFiles())
	row.Children(func() {
		for _, f := range a.files {
			chip := ui.Box(c.Key(f)).Shrink(0)
			hover := chip.Hovered()
			chip.Children(func() {
				a.chipFace(c, f, chipSize)
				// Under the pointer: a round ✕ over its corner, clear of its
				// name, in the row's room over the chips.
				label := locales.AttachRemove(f.Name)
				x := tooltip(c, ui.ButtonBase(c.Key("remove")).Size(removeSize, removeSize).Radius(removeSize/2).
					Absolute().Top(-removeOut).Right(-removeOut).
					Background(a.pal.fg.Alpha(0.75)).TextColor(a.pal.bg).Label(label), label)
				x.Opacity(x.Animate("opacity", shown(hover), revealDuration))
				x.Children(func() { ui.Icon(c, closeIcon).Size(9, 9) })
				if x.Clicked() {
					a.detach(f)
				}
			})
		}
	})
}

// sentFiles builds the files sent with the question m, over its bubble on
// the right: each opens as clicked, in the app the system opens it in.
func (a *app) sentFiles(c *ui.Context, m *message) {
	row := ui.Row(c).Wrap().Justify(ui.End).AlignItems(ui.End).Gap(8)
	if m.Text != "" {
		row.Margin(0, 0, 8, 0)
	}
	row.Children(func() {
		for _, f := range m.Files {
			chip := ui.ButtonBase(c.Key(f)).Shrink(0).Label(f.Name)
			if chip.Clicked() && f.File != "" && a.openPath != nil && a.filesDir != "" {
				a.openPath(filepath.Join(a.filesDir, f.File))
			}
			chip.Children(func() { a.chipFace(c, f, sentImageSize) })
		}
	})
}

// The ✕ that takes a file off the draft is a circle removeSize across,
// removeOut past its chip's top and right.
const (
	removeSize = 16
	removeOut  = 5
)

// chipFace builds f as its chip shows it: an image as its picture, size
// square, its corners rounded; another file as its icon on a tile, by its
// name over its kind, its extension, as Codex's are; each in a hairline.
func (a *app) chipFace(c *ui.Context, f *attachment, size float32) {
	t := c.Theme()
	if b := a.thumbnail(f); b != nil {
		ui.Image(c, b).Size(size, size).Fit(ui.Cover).Radius(chipRadius).Label(f.Name)
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(chipRadius).Border(1, t.Border).PassThrough()
		return
	}
	if f.image() && !f.broken {
		// Until it is read.
		ui.Box(c).Size(size, size).Radius(chipRadius).Background(a.pal.fg.Alpha(0.05)).Border(1, t.Border)
		return
	}
	ui.Row(c).Height(chipSize).MaxWidth(240).Padding(0, 14, 0, 8).Gap(10).AlignItems(ui.Center).Radius(chipRadius).
		Background(a.pal.fg.Alpha(0.04)).Border(1, t.Border).Children(func() {
		ui.Box(c).Size(40, 40).Shrink(0).Radius(8).Background(a.pal.fg.Alpha(0.07)).Justify(ui.Center).AlignItems(ui.Center).
			Children(func() { ui.Icon(c, fileTextIcon).Size(16, 16).TextColor(t.TextMuted) })
		ui.Column(c).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, f.Name).FontSize(13).SingleLine()
			if ext := strings.TrimPrefix(filepath.Ext(f.Name), "."); ext != "" {
				ui.Text(c, strings.ToUpper(ext)).FontSize(12).TextColor(t.TextMuted).SingleLine()
			}
		})
	})
}

// dropOverlay builds, over the composer's box, what it shows while files
// dragged from another app are over the chat: that dropping attaches
// them, ringed in the accent, as what takes them.
func (a *app) dropOverlay(c *ui.Context) {
	ui.Row(c).Absolute().Top(-1).Left(-1).Right(-1).Bottom(-1).Radius(composerRadius).
		Background(a.pal.composer).Border(1.5, a.pal.ac).Justify(ui.Center).AlignItems(ui.Center).Gap(8).PassThrough().
		Children(func() {
			ui.Icon(c, paperclipIcon).Size(16, 16).TextColor(a.pal.ac)
			ui.Text(c, locales.AttachDrop()).FontSize(14)
		})
}

// attachToasts says, in a toast each, why the files that could not be
// attached were not.
func (a *app) attachToasts(c *ui.Context) {
	for _, why := range a.attachFailed {
		about := locales.AttachUnreadable()
		switch {
		case why.unsupported:
			about = locales.AttachUnsupported()
		case why.max > 0:
			about = locales.AttachTooLarge(why.max)
		}
		c.AddToast(ui.Toast{ID: "attach " + why.name, Title: locales.AttachFailed(why.name), Description: about})
	}
	a.attachFailed = nil
}
