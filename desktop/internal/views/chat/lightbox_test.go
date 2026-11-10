package chat_test

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	stageX       = wideW / 2
	probeX       = 100
	probeY       = 50
	dragX        = 60
	dragY        = 40
	longName     = "long.png"
	longW        = 1200
	longH        = 5000
	longWindowW  = 700
	longStageTop = barHeight
	longScroll   = 1000
	longFit      = float32(longWindowW-2*48-2) / longW
	longLeft     = 48 + 1
	wheelNotchDY = 100
)

func (f *fixture) openPhoto() {
	f.t.Helper()
	f.click(locales.ChatViewImage(photoName))
	f.wantShown(locales.ChatCloseImage(), "100%")
}

func photoColor(x, y int) color.RGBA {
	return color.RGBA{R: uint8(x), G: uint8(y), B: 160, A: 255}
}

func (f *fixture) wantPixel(x, y int, want color.RGBA, what string) {
	f.t.Helper()
	if got := f.pixel(x, y); !near(got, want) {
		f.t.Fatalf("%s: pixel at %d,%d = %v, want %v", what, x, y, got, want)
	}
}

func TestLightboxZoomButtons(t *testing.T) {
	f := photoChat(t)
	f.openPhoto()
	f.wantShown(locales.ChatZoomIn(), locales.ChatZoomOut(), locales.ChatZoomFit(), locales.ChatDownload(), locales.ChatOpenOriginal())
	f.wantPixel(stageX+probeX, stageMiddle, photoColor(photoW/2+probeX, photoH/2), "at 100%")

	f.click(locales.ChatZoomIn())
	f.wantShown("125%")
	f.click(locales.ChatZoomIn())
	f.wantShown("150%")
	f.click(locales.ChatZoomIn())
	f.wantShown("200%")
	f.wantPixel(stageX, stageMiddle, photoColor(photoW/2, photoH/2), "centre at 200%")
	f.wantPixel(stageX+probeX, stageMiddle, photoColor(photoW/2+probeX/2, photoH/2), "at 200%")

	f.click(locales.ChatZoomOut())
	f.wantShown("150%")
	f.click(locales.ChatZoomFit())
	f.wantShown("100%")
	f.wantPixel(stageX+probeX, stageMiddle, photoColor(photoW/2+probeX, photoH/2), "after fitting")
}

func TestLightboxKeysZoom(t *testing.T) {
	f := photoChat(t)
	f.openPhoto()
	f.press(ui.KeyEqual)
	f.wantShown("125%")
	f.press(ui.KeyMinus)
	f.wantShown("100%")
	f.T.Key(ui.Shift, ui.KeyEqual)
	f.settleMotion()
	f.press(ui.KeyEqual)
	f.wantShown("150%")
	f.press(ui.Key0)
	f.wantShown("100%")
	if !f.lightboxShown() {
		t.Fatal("zoom keys closed the lightbox")
	}
}

func TestLightboxPinchZoomsAroundThePointer(t *testing.T) {
	f := photoChat(t)
	f.openPhoto()
	px, py := float32(stageX+probeX), float32(stageMiddle+probeY)
	f.T.Pinch(px, py, 8)
	f.settleMotion()
	f.wantShown("800%")
	f.wantPixel(int(px), int(py), photoColor(photoW/2+probeX, photoH/2+probeY), "under the pointer")
	f.wantPixel(int(px)+40, int(py), photoColor(photoW/2+probeX+5, photoH/2+probeY), "beside the pointer")
}

func TestLightboxCtrlWheelZooms(t *testing.T) {
	f := photoChat(t)
	f.openPhoto()
	px, py := float32(stageX), float32(stageMiddle-barHeight)
	f.T.Wheel(stageX, stageMiddle, 0, wheelNotchDY)
	f.settleMotion()
	f.wantShown("100%")

	for _, mods := range []ui.Modifiers{ui.Ctrl, ui.Super} {
		if !f.view.LightboxInput(ui.InputEvent{Kind: ui.InputScroll, Mods: mods, X: px, Y: py, DY: -wheelNotchDY}) {
			t.Fatalf("a wheel notch with %v was not taken", mods)
		}
		f.settleMotion()
		f.wantShown("125%")
		f.view.LightboxInput(ui.InputEvent{Kind: ui.InputScroll, Mods: mods, X: px, Y: py, DY: wheelNotchDY})
		f.settleMotion()
		f.wantShown("100%")
	}
	if f.view.LightboxInput(ui.InputEvent{Kind: ui.InputScroll, X: px, Y: py, DY: wheelNotchDY}) {
		t.Fatal("a plain wheel notch was taken, want it to scroll")
	}
	f.view.LightboxInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Ctrl, X: px, Y: py, DY: -10, Precise: true})
	f.settleMotion()
	f.wantShown("111%")
}

func TestLightboxDragPansTheZoomedPhoto(t *testing.T) {
	f := photoChat(t)
	f.openPhoto()
	f.T.Pinch(stageX, stageMiddle, 4)
	f.settleMotion()
	f.wantShown("400%")
	f.T.Move(stageX, stageMiddle)
	f.T.Frame()
	if got := f.T.Cursor(); got != ui.CursorGrab {
		t.Fatalf("cursor over the zoomed photo = %v, want grab", got)
	}
	want := f.pixel(stageX+dragX, stageMiddle+dragY)
	f.T.Press(stageX, stageMiddle)
	f.T.Move(stageX-dragX/2, stageMiddle-dragY/2)
	f.T.Move(stageX-dragX, stageMiddle-dragY)
	f.T.Frame()
	if got := f.T.Cursor(); got != ui.CursorGrabbing {
		t.Fatalf("cursor while dragging = %v, want grabbing", got)
	}
	f.T.Release(stageX-dragX, stageMiddle-dragY)
	f.settleMotion()
	if !f.lightboxShown() {
		t.Fatal("dragging closed the lightbox")
	}
	f.wantPixel(stageX, stageMiddle, want, "after the drag")

	f.T.ClickAt(stageX, stageMiddle)
	f.settleMotion()
	if !f.lightboxShown() {
		t.Fatal("a click on the zoomed photo closed the lightbox")
	}
}

func TestLightboxDoubleClickToggles(t *testing.T) {
	f := photoChat(t)
	f.openPhoto()
	f.T.ClickAt(stageX, stageMiddle)
	f.T.ClickAt(stageX, stageMiddle)
	f.settleMotion()
	f.wantShown("200%")
	f.T.ClickAt(stageX, stageMiddle)
	f.T.ClickAt(stageX, stageMiddle)
	f.settleMotion()
	f.wantShown("100%")
	if !f.lightboxShown() {
		t.Fatal("double clicks closed the lightbox")
	}
}

func TestLightboxBackdropStillClosesAfterZoom(t *testing.T) {
	f := photoChat(t)
	f.openPhoto()
	f.click(locales.ChatZoomIn())
	f.wantShown("125%")
	f.T.ClickAt(lightboxEdge, stageMiddle)
	f.settleMotion()
	f.wantHidden(locales.ChatCloseImage())

	f.openPhoto()
}

func TestLightboxLongImageFitsWidthFromTheTop(t *testing.T) {
	m := withAttachments(message("m1", "worker", "w1", "A long screenshot.", time.Now().Add(-time.Hour)), longName)
	f := newFixture(t, longWindowW, wideH, "w1", attached(map[string][]byte{longName: pngImage(t, longW, longH)}, m))
	deadline := time.Now().Add(imageTimeout)
	for !f.shows(locales.ChatViewImage(longName)) {
		f.Settle()
		f.T.Frame()
		if time.Now().After(deadline) {
			t.Fatalf("no thumbnail for the long image; texts %q", f.T.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.click(locales.ChatViewImage(longName))
	deadline = time.Now().Add(imageTimeout)
	for !f.shows("50%") {
		f.Settle()
		f.T.Frame()
		if time.Now().After(deadline) {
			t.Fatalf("the long image did not open at 50%%; texts %q", f.T.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
	row := func(y int) int { return int(float32(y-longStageTop-1) / longFit) }
	x := longWindowW / 2
	col := int(float32(x-longLeft) / longFit)
	f.wantPixel(x, longStageTop+4, photoColor(col, row(longStageTop+4)), "top of the stage")
	f.wantPixel(x, wideH-10, photoColor(col, row(wideH-10)), "bottom of the stage")

	f.T.Scroll(float32(x), wideH/2, 0, longScroll)
	f.settleMotion()
	f.wantShown("50%")
	f.wantPixel(x, longStageTop+4, photoColor(col, row(longStageTop+4+longScroll)), "after scrolling down")
}

func TestLightboxDownloadSavesWithoutOpening(t *testing.T) {
	f := photoChat(t)
	o := &opener{}
	f.view.SetOpener(o.open)
	f.openPhoto()
	f.click(locales.ChatDownload())
	saved := filepath.Join(f.Fix.DownloadDir, photoName)
	f.Fix.Await(t, func() bool {
		_, err := os.Stat(saved)
		return err == nil
	})
	f.settleMotion()
	f.wantShown(locales.FileSavedToDownloads())
	if got := o.paths(); len(got) != 0 {
		t.Fatalf("opened %q, want the download only saved", got)
	}
	if !f.lightboxShown() {
		t.Fatal("downloading closed the lightbox")
	}
}

func TestLightboxNarrowKeepsFitAndActions(t *testing.T) {
	m := withAttachments(message("m1", "worker", "w1", "Here is the photo.", time.Now().Add(-time.Hour)), photoName)
	f := newFixture(t, narrowPhone, wideH, "w1", attached(map[string][]byte{photoName: pngImage(t, photoW, photoH)}, m))
	deadline := time.Now().Add(imageTimeout)
	for !f.shows(locales.ChatViewImage(photoName)) {
		f.Settle()
		f.T.Frame()
		if time.Now().After(deadline) {
			t.Fatalf("no thumbnail; texts %q", f.T.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.click(locales.ChatViewImage(photoName))
	f.wantShown(locales.ChatCloseImage(), locales.ChatZoomFit(), locales.ChatDownload(), locales.ChatOpenOriginal())
	f.wantHidden(locales.ChatZoomIn(), locales.ChatZoomOut())
	title, _ := f.T.Find(photoName)
	if title.W < photoTitleMin {
		t.Fatalf("title is %v wide, want at least %v", title.W, photoTitleMin)
	}
}

const photoTitleMin = 60
