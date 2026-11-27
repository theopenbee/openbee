package chat_test

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	copyJPEGName = "photo.jpg"
	copyJPEGW    = 40
	copyJPEGH    = 30
	copyFeedback = 1900 * time.Millisecond
)

func (f *fixture) clipboardPNG() []byte {
	data, err := f.T.ClipboardData().Read(transfer.PNG)
	if err != nil {
		return nil
	}
	return data
}

func (f *fixture) openImage(name string) {
	f.t.Helper()
	deadline := time.Now().Add(imageTimeout)
	for !f.shows(locales.ChatViewImage(name)) {
		f.Settle()
		f.T.Frame()
		if time.Now().After(deadline) {
			f.t.Fatalf("no thumbnail for %s; texts %q", name, f.T.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.click(locales.ChatViewImage(name))
	f.wantShown(locales.ChatCloseImage())
}

func TestLightboxCopyPutsTheOriginalPNGOnTheClipboard(t *testing.T) {
	original := pngImage(t, photoW, photoH)
	m := withAttachments(message("m1", "worker", "w1", "Here is the photo.", time.Now().Add(-time.Hour)), photoName)
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{photoName: original}, m))
	f.openImage(photoName)
	f.wantShown(locales.ArcCopyButtonCopy())

	f.click(locales.ChatCopyImage())
	f.Fix.Await(t, func() bool { return f.clipboardPNG() != nil })
	if !bytes.Equal(f.clipboardPNG(), original) {
		t.Fatal("the clipboard does not hold the original PNG")
	}
	f.Settle()
	f.T.Frame()
	f.wantShown(locales.ArcCopyButtonCopied())
	if !f.lightboxShown() {
		t.Fatal("copying closed the lightbox")
	}

	f.T.Advance(copyFeedback)
	f.T.Frame()
	f.wantHidden(locales.ArcCopyButtonCopied())
	f.wantShown(locales.ArcCopyButtonCopy())
}

func TestLightboxCmdCCopiesAnUprightPNGOfAJPEG(t *testing.T) {
	m := withAttachments(message("m1", "worker", "w1", "Here is the photo.", time.Now().Add(-time.Hour)), copyJPEGName)
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{copyJPEGName: exifJPEG(t, copyJPEGW, copyJPEGH, exifRotateCW)}, m))
	f.openImage(copyJPEGName)

	f.T.Key(ui.Cmd, ui.KeyC)
	f.Fix.Await(t, func() bool { return f.clipboardPNG() != nil })
	img, err := png.Decode(bytes.NewReader(f.clipboardPNG()))
	if err != nil {
		t.Fatalf("the clipboard does not hold a PNG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != copyJPEGH || b.Dy() != copyJPEGW {
		t.Fatalf("copied image is %d×%d, want the upright %d×%d", b.Dx(), b.Dy(), copyJPEGH, copyJPEGW)
	}
	top, bottom := rgbaAt(img, b.Dx()/2, b.Dy()/8), rgbaAt(img, b.Dx()/2, b.Dy()*7/8)
	if top.R < 150 || top.B > 70 || bottom.B < 150 || bottom.R > 70 {
		t.Fatalf("top %v, bottom %v; a photo rotated clockwise should put its red left half on top", top, bottom)
	}
}

func TestLightboxEditMenuCopyCopiesTheImage(t *testing.T) {
	f := photoChat(t)
	f.openPhoto()
	f.T.Command("copy")
	f.Fix.Await(t, func() bool { return f.clipboardPNG() != nil })

	f.T.SetClipboard("")
	f.click(locales.ChatZoomIn())
	f.T.Command("copy")
	f.Fix.Await(t, func() bool { return f.clipboardPNG() != nil })
}

func TestLightboxHasNoCopyForSVG(t *testing.T) {
	const name = "logo.svg"
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="30"><rect width="40" height="30" fill="red"/></svg>`)
	m := withAttachments(message("m1", "worker", "w1", "Here is the logo.", time.Now().Add(-time.Hour)), name)
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{name: svg}, m))
	f.openImage(name)
	f.wantShown(locales.ChatDownload())
	f.wantHidden(locales.ChatCopyImage())

	f.T.Key(ui.Cmd, ui.KeyC)
	f.T.Command("copy")
	f.settleMotion()
	if f.clipboardPNG() != nil {
		t.Fatal("⌘C copied an SVG")
	}
}

func rgbaAt(img image.Image, x, y int) struct{ R, B uint32 } {
	r, _, b, _ := img.At(x, y).RGBA()
	return struct{ R, B uint32 }{r >> 8, b >> 8}
}

func TestLightboxCopyKeepsItsWidthWhenCopied(t *testing.T) {
	f := photoChat(t)
	f.openPhoto()
	before, ok := f.T.Find(locales.ChatZoomIn())
	if !ok {
		t.Fatalf("no Zoom in button; texts %q", f.T.Texts())
	}
	f.click(locales.ChatCopyImage())
	f.Fix.Await(t, func() bool { return f.clipboardPNG() != nil })
	f.Settle()
	f.T.Frame()
	f.wantShown(locales.ArcCopyButtonCopied())
	if after, _ := f.T.Find(locales.ChatZoomIn()); after.X != before.X {
		t.Fatalf("Zoom in moved from x=%v to x=%v when Copy turned into Copied", before.X, after.X)
	}
}
