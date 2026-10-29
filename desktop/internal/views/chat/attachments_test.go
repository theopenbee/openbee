package chat_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/chat"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	photoName       = "photo.png"
	photoW          = 400
	photoH          = 300
	thumbHeight     = 240
	thumbBorder     = 2
	photoEdge       = 2560
	photoShort      = 853
	thumbSlack      = 0.5
	pixelSlack      = 6
	barHeight       = 76
	stageBottom     = 32
	stageMiddle     = (barHeight + wideH - stageBottom) / 2
	tallPhotoW      = 9000
	tallPhotoH      = 7000
	landscapeW      = 3000
	landscapeH      = 1000
	exifRotateCW    = 6
	declaredSide    = 20000
	reportName      = "report.pdf"
	reportContent   = "%PDF-1.7 report"
	lightboxEdge    = 20
	columnMax       = 768
	columnPadding   = 24
	controlsHeight  = 32
	controlsLeft    = 80
	controlsRight   = 140
	narrowPhone     = 500
	longNameLength  = 160
	panoramaW       = 1800
	panoramaH       = 300
	bigPhotoTimeout = 30 * time.Second
)

type opener struct {
	mu     sync.Mutex
	opened []string
	err    error
}

func (o *opener) open(path string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.opened = append(o.opened, path)
	return o.err
}

func (o *opener) paths() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.opened...)
}

func withAttachments(m api.ChatMessage, paths ...string) api.ChatMessage {
	m.MediaPaths = paths
	return m
}

func attached(media map[string][]byte, msgs ...api.ChatMessage) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		seedChat(team(), workerCID, msgs...)(st)
		for name, data := range media {
			withMedia(name, data)(st)
		}
	}
}

func photoChat(t *testing.T) *fixture {
	t.Helper()
	m := withAttachments(message("m1", "worker", "w1", "Here is the photo.", time.Now().Add(-time.Hour)), photoName)
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{photoName: pngImage(t, photoW, photoH)}, m))
	f.awaitThumb(photoName, photoW*thumbHeight/photoH)
	return f
}

func (f *fixture) awaitThumb(name string, width float32) {
	f.t.Helper()
	f.awaitThumbWithin(name, width, imageTimeout)
}

func (f *fixture) awaitThumbWithin(name string, width float32, timeout time.Duration) {
	f.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		f.Settle()
		f.T.Frame()
		r, ok := f.T.Find(locales.ChatViewImage(name))
		if ok && abs(r.W-(width+thumbBorder)) < thumbSlack && abs(r.H-(thumbHeight+thumbBorder)) < thumbSlack {
			return
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("thumbnail of %s did not load: box %v (shown %v), want %v×%v; texts %q", name, r, ok, width+thumbBorder, thumbHeight+thumbBorder, f.T.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func near(a, b color.RGBA) bool {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return d(a.R, b.R) <= pixelSlack && d(a.G, b.G) <= pixelSlack && d(a.B, b.B) <= pixelSlack
}

func abs(x float32) float32 {
	return max(x, -x)
}

func (f *fixture) pixel(x, y int) color.RGBA {
	return f.T.Image().RGBAAt(x, y)
}

func (f *fixture) press(key ui.Key) {
	f.t.Helper()
	f.T.Key(0, key)
	f.settleMotion()
}

func (f *fixture) lightboxShown() bool {
	return f.shows(locales.ChatCloseImage())
}

func exifJPEG(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{R: 220, A: 255}
			if x >= w/2 {
				c = color.RGBA{B: 220, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08\x00\x01")
	tiff = binary.BigEndian.AppendUint16(tiff, 0x0112)
	tiff = binary.BigEndian.AppendUint16(tiff, 3)
	tiff = binary.BigEndian.AppendUint32(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, uint16(orientation))
	tiff = append(tiff, 0, 0, 0, 0, 0, 0)
	segment := append([]byte("Exif\x00\x00"), tiff...)
	out := append([]byte{0xff, 0xd8, 0xff, 0xe1}, binary.BigEndian.AppendUint16(nil, uint16(len(segment)+2))...)
	out = append(out, segment...)
	return append(out, data[2:]...)
}

func TestFileCategory(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		{"a.PNG", "image"},
		{"photo.jpeg", "image"},
		{"vector.svg", "image"},
		{"song.m4a", "audio"},
		{"clip.webm", "video"},
		{"report.pdf", "document"},
		{"table.csv", "document"},
		{"x.tsx", "code"},
		{"query.sql", "code"},
		{"y.7z", "archive"},
		{"backup.tar.gz", "archive"},
		{"uploads/dir.d/deep.PY", "code"},
		{"C:\\files\\scan.bmp", "image"},
		{"z", "other"},
		{"png", "other"},
		{".gitignore", "other"},
		{"data.bin", "other"},
		{"", "other"},
	} {
		if got := chat.FileCategory(tc.name); got != tc.want {
			t.Errorf("fileCategory(%q) = %q, want %q", tc.name, got, tc.want)
		}
		if got, want := chat.IsImage(tc.name), tc.want == "image"; got != want {
			t.Errorf("isImage(%q) = %v, want %v", tc.name, got, want)
		}
	}
}

func TestImageThumbnailOpensLightbox(t *testing.T) {
	f := photoChat(t)
	if f.lightboxShown() {
		t.Fatal("the lightbox is open before the thumbnail is clicked")
	}
	f.click(locales.ChatViewImage(photoName))
	f.wantShown(locales.ChatCloseImage(), locales.ChatOpenOriginal(), photoName)
	f.T.Frame()
	if got, want := f.pixel(wideW/2, stageMiddle), (color.RGBA{R: photoW / 2, G: photoH / 2, B: 160, A: 255}); !near(got, want) {
		t.Fatalf("centre of the lightbox stage = %v, want the photo's centre %v", got, want)
	}
	if n := f.mediaGets(photoName); n != 1 {
		t.Fatalf("%d downloads of %s for the thumbnail and the lightbox, want 1", n, photoName)
	}
}

func TestLightboxEscapeCloses(t *testing.T) {
	f := photoChat(t)
	f.click(locales.ChatViewImage(photoName))
	f.wantShown(locales.ChatCloseImage())
	f.press(ui.KeyEscape)
	f.wantHidden(locales.ChatCloseImage())
	f.wantShown(locales.ChatViewImage(photoName))
}

func TestLightboxClosesFromButtonAndBackdrop(t *testing.T) {
	f := photoChat(t)
	f.click(locales.ChatViewImage(photoName))
	f.click(locales.ChatCloseImage())
	f.wantHidden(locales.ChatCloseImage())

	f.click(locales.ChatViewImage(photoName))
	f.T.ClickAt(wideW/2, wideH/2)
	f.settleMotion()
	f.wantShown(locales.ChatCloseImage())

	f.T.ClickAt(wideW/2, wideH-lightboxEdge)
	f.settleMotion()
	f.wantHidden(locales.ChatCloseImage())

	f.click(locales.ChatViewImage(photoName))
	f.T.ClickAt(lightboxEdge, wideH/2)
	f.settleMotion()
	f.wantHidden(locales.ChatCloseImage())
}

func TestLightboxBarClearsWindowControls(t *testing.T) {
	f := photoChat(t)
	f.T.SetTitleBar(ui.TitleBar{Height: controlsHeight, Left: controlsLeft, Right: controlsRight})
	f.click(locales.ChatViewImage(photoName))
	name, ok := f.T.Find(photoName)
	if !ok || name.X < controlsLeft {
		t.Fatalf("file name at %v (shown %v), want it right of the %v px of window controls", name, ok, controlsLeft)
	}
	closeButton, _ := f.T.Find(locales.ChatCloseImage())
	if right := closeButton.X + closeButton.W; right > wideW-controlsRight {
		t.Fatalf("close button ends at %v, want it left of the window controls at %v", right, wideW-controlsRight)
	}
}

func TestLightboxOpenOriginal(t *testing.T) {
	f := photoChat(t)
	o := &opener{}
	f.view.SetOpener(o.open)
	f.click(locales.ChatViewImage(photoName))
	f.click(locales.ChatOpenOriginal())
	f.Fix.Await(t, func() bool { return len(o.paths()) == 1 })
	got := o.paths()[0]
	if filepath.Base(got) != photoName || filepath.Base(filepath.Dir(got)) != "media" {
		t.Fatalf("opened %q, want the cached %s", got, photoName)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatal(err)
	}
	if !f.lightboxShown() {
		t.Fatal("opening the original closed the lightbox")
	}
}

func TestFileChipOpensWithShell(t *testing.T) {
	m := withAttachments(message("m1", "worker", "w1", "The report is attached.", time.Now().Add(-time.Hour)), "uploads/"+reportName)
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{reportName: []byte(reportContent)}, m))
	o := &opener{}
	f.view.SetOpener(o.open)
	f.wantShown(reportName)
	f.click(reportName)
	f.Fix.Await(t, func() bool { return len(o.paths()) == 1 })
	got := o.paths()[0]
	if filepath.Base(got) != reportName || filepath.Base(filepath.Dir(got)) != "media" {
		t.Fatalf("opened %q, want the cached %s", got, reportName)
	}
	if data, err := os.ReadFile(got); err != nil || string(data) != reportContent {
		t.Fatalf("cached file = %q, %v; want %q", data, err, reportContent)
	}
	f.settleMotion()
	f.wantHidden(locales.FileOpenFailed())
	f.click(reportName)
	f.Fix.Await(t, func() bool { return len(o.paths()) == 2 })
	if n := f.mediaGets(reportName); n != 1 {
		t.Fatalf("%d downloads of %s, want the second open to use the cache", n, reportName)
	}
}

func TestFileChipOpenFailureToasts(t *testing.T) {
	m := withAttachments(message("m1", "worker", "w1", "Files.", time.Now().Add(-time.Hour)), reportName)
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{reportName: []byte(reportContent)}, m))
	o := &opener{err: errors.New("no application")}
	f.view.SetOpener(o.open)
	f.wantHidden(locales.FileOpenFailed())
	f.click(reportName)
	f.Fix.Await(t, func() bool { return len(o.paths()) == 1 })
	f.settleMotion()
	f.wantShown(locales.FileOpenFailed())
}

func TestFileChipDownloadFailureToasts(t *testing.T) {
	m := withAttachments(message("m1", "worker", "w1", "Files.", time.Now().Add(-time.Hour)), "gone.zip")
	f := newFixture(t, wideW, wideH, "w1", attached(nil, m))
	o := &opener{}
	f.view.SetOpener(o.open)
	f.wantHidden(locales.FileOpenFailed())
	f.click("gone.zip")
	f.Fix.Await(t, func() bool { return f.mediaGets("gone.zip") == 1 })
	f.settleMotion()
	f.wantShown(locales.FileOpenFailed())
	if n := len(o.paths()); n != 0 {
		t.Fatalf("%d files opened after a failed download, want none", n)
	}
}

func TestUserAttachmentsSitAboveTheirText(t *testing.T) {
	at := time.Now().Add(-time.Hour)
	m := withAttachments(message("m1", "user", "", "Please review this.", at), photoName, reportName)
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{photoName: pngImage(t, photoW, photoH), reportName: []byte(reportContent)}, m))
	f.awaitThumb(photoName, photoW*thumbHeight/photoH)
	thumb, _ := f.T.Find(locales.ChatViewImage(photoName))
	chip, _ := f.T.Find(reportName)
	text, ok := f.T.Find("Please review this.")
	if !ok {
		t.Fatalf("no message text; texts %q", f.T.Texts())
	}
	if !(thumb.Y < chip.Y && chip.Y < text.Y) {
		t.Fatalf("thumbnail y=%v, chip y=%v, text y=%v; want attachments above the text, in order", thumb.Y, chip.Y, text.Y)
	}
	column := float32(wideW+columnMax)/2 - columnPadding
	if right := thumb.X + thumb.W; right != column {
		t.Fatalf("thumbnail right edge = %v, want the right edge of the column (%v)", right, column)
	}
}

func TestAttachmentsStayInsideNarrowWindow(t *testing.T) {
	long := strings.Repeat("quarterly-", longNameLength/10) + "report.pdf"
	m := withAttachments(message("m1", "worker", "w1", "Wide things.", time.Now().Add(-time.Hour)), long, "panorama.png")
	f := newFixture(t, narrowPhone, wideH, "w1", attached(map[string][]byte{long: []byte(reportContent), "panorama.png": pngImage(t, panoramaW, panoramaH)}, m))
	deadline := time.Now().Add(imageTimeout)
	for {
		f.Settle()
		f.T.Frame()
		if _, ok := f.T.Find(locales.ChatViewImage("panorama.png")); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the panorama did not load; texts %q", f.T.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
	for _, name := range []string{long, locales.ChatViewImage("panorama.png")} {
		r, ok := f.T.Find(name)
		if !ok || r.X < 0 || r.X+r.W > narrowPhone {
			t.Fatalf("%.20q... spans %v (shown %v), want it inside the %d px window", name, r, ok, narrowPhone)
		}
	}
	thumb, _ := f.T.Find(locales.ChatViewImage("panorama.png"))
	if want := (thumb.W - thumbBorder) * panoramaH / panoramaW; abs(thumb.H-thumbBorder-want) > thumbSlack {
		t.Fatalf("panorama box %v×%v, want its proportions kept (height %v)", thumb.W, thumb.H, want+thumbBorder)
	}
}

func TestAttachmentOnlyMessage(t *testing.T) {
	m := withAttachments(message("m1", "user", "", " ", time.Now().Add(-time.Hour)), reportName)
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{reportName: []byte(reportContent)}, m))
	f.wantShown(reportName)
}

func TestWorkerAttachmentsKeepTheSenderHeader(t *testing.T) {
	m := withAttachments(message("m1", "worker", "w1", "", time.Now().Add(-time.Hour)), reportName)
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{reportName: []byte(reportContent)}, m))
	f.wantShown("Alice", reportName)
	name, _ := f.T.Find("Alice")
	chip, _ := f.T.Find(reportName)
	if chip.Y <= name.Y {
		t.Fatalf("chip y=%v is not below the sender name y=%v", chip.Y, name.Y)
	}
}

func TestLargePhotoDisplaysDownscaled(t *testing.T) {
	m := withAttachments(message("m1", "worker", "w1", "Full resolution.", time.Now().Add(-time.Hour)), "wide.png")
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{"wide.png": grayPNG(t, tallPhotoW, tallPhotoH)}, m))
	f.awaitThumbWithin("wide.png", float32(tallPhotoW)*thumbHeight/tallPhotoH, bigPhotoTimeout)
	if got, limit := f.view.CachedImageBytes(), int64(photoEdge*photoEdge*4); got <= 0 || got > limit {
		t.Fatalf("cached %d bytes for a %d×%d photo, want a bitmap of at most %d×%d (%d bytes)", got, tallPhotoW, tallPhotoH, photoEdge, photoEdge, limit)
	}
	f.click(locales.ChatViewImage("wide.png"))
	f.wantShown(locales.ChatCloseImage())
	if n := f.mediaGets("wide.png"); n != 1 {
		t.Fatalf("%d downloads of the photo, want 1", n)
	}
}

func TestPhotoScalesDownToTheEdge(t *testing.T) {
	ic := chat.NewImageCache(func(string) ([]byte, error) { return grayPNG(t, landscapeW, landscapeH), nil })
	src, err := ic.LoadPhoto("landscape.png")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := src.(*ui.Bitmap)
	if !ok {
		t.Fatalf("source is %T, want a bitmap", src)
	}
	w, h := b.Size()
	if w != photoEdge || h != photoShort {
		t.Fatalf("bitmap is %d×%d, want %d×%d", w, h, photoEdge, photoShort)
	}
	if got := ic.Bytes(); got != int64(w*h*4) {
		t.Fatalf("cache counts %d bytes, want %d", got, w*h*4)
	}
	if again, err := ic.LoadPhoto("landscape.png"); err != nil || again != src {
		t.Fatalf("second load = %v, %v; want the cached bitmap", again, err)
	}
	if ic.Len() != 1 {
		t.Fatalf("%d images cached, want 1", ic.Len())
	}
}

func TestHugePhotoDeclarationRefused(t *testing.T) {
	huge := declaredPNG(t, declaredSide, declaredSide)
	ic := chat.NewImageCache(func(string) ([]byte, error) { return huge, nil })
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := ic.LoadPhoto("huge.png")
	runtime.ReadMemStats(&after)
	if !errors.Is(err, chat.ErrImageTooLarge) {
		t.Fatalf("load of a %d×%d PNG: err = %v, want ErrImageTooLarge", declaredSide, declaredSide, err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > smallGrowth {
		t.Fatalf("refusing the photo allocated %d bytes, want at most %d", grew, smallGrowth)
	}
	if n := ic.Len(); n != 0 {
		t.Fatalf("%d images cached, want none", n)
	}
}

func TestUnreadablePhotoFallsBackToChip(t *testing.T) {
	m := withAttachments(message("m1", "worker", "w1", "Scan attached.", time.Now().Add(-time.Hour)), "huge.png")
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{"huge.png": declaredPNG(t, declaredSide, declaredSide)}, m))
	o := &opener{}
	f.view.SetOpener(o.open)
	deadline := time.Now().Add(imageTimeout)
	for !f.shows("huge.png") {
		f.Settle()
		f.T.Frame()
		if time.Now().After(deadline) {
			t.Fatalf("no file chip for the unreadable photo; texts %q", f.T.Texts())
		}
		time.Sleep(time.Millisecond)
	}
	f.wantHidden(locales.ChatViewImage("huge.png"))
	f.click("huge.png")
	f.Fix.Await(t, func() bool { return len(o.paths()) == 1 })
}

func TestPhotoFollowsExifOrientation(t *testing.T) {
	img, err := chat.ScaledPhoto(exifJPEG(t, landscapeW, landscapeH, exifRotateCW))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dy() != photoEdge || b.Dx() != photoShort {
		t.Fatalf("rotated photo is %d×%d, want %d×%d", b.Dx(), b.Dy(), photoShort, photoEdge)
	}
	top, bottom := img.RGBAAt(b.Dx()/2, b.Dy()/8), img.RGBAAt(b.Dx()/2, b.Dy()*7/8)
	if top.R < 150 || top.B > 70 || bottom.B < 150 || bottom.R > 70 {
		t.Fatalf("top %v, bottom %v; a photo rotated clockwise should put its red left half on top", top, bottom)
	}
	plain, err := chat.ScaledPhoto(exifJPEG(t, landscapeW, landscapeH, 1))
	if err != nil {
		t.Fatal(err)
	}
	if b := plain.Bounds(); b.Dx() != photoEdge {
		t.Fatalf("unrotated photo is %d wide, want %d", b.Dx(), photoEdge)
	}
}

func TestOnlyKnownSafeAttachmentsOpenDirectly(t *testing.T) {
	opens := map[string]bool{reportName: true}
	names := []string{reportName, "run.command", "setup.exe", "x.exe.", "script.sh", "tool.py", "readme", "x.lnk", "Setup.EXE"}
	media := map[string][]byte{}
	for _, name := range names {
		media[name] = []byte("content of " + name)
	}
	m := withAttachments(message("m1", "worker", "w1", "Files.", time.Now().Add(-time.Hour)), names...)
	f := newFixture(t, wideW, wideH, "w1", attached(media, m))
	opened, revealed := &opener{}, &opener{}
	f.view.SetOpener(opened.open)
	f.view.SetRevealer(revealed.open)
	for i, name := range names {
		f.wantShown(name)
		f.click(name)
		f.Fix.Await(t, func() bool { return len(opened.paths())+len(revealed.paths()) == i+1 })
		got := revealed.paths()
		if opens[name] {
			got = opened.paths()
		}
		if len(got) == 0 {
			t.Fatalf("%s: handed to the wrong launcher; opened %q, revealed %q", name, opened.paths(), revealed.paths())
		}
		last := got[len(got)-1]
		if filepath.Base(last) != name || filepath.Base(filepath.Dir(last)) != "media" {
			t.Fatalf("%s: launcher got %q, want the cached file", name, last)
		}
	}
	if got := opened.paths(); len(got) != len(opens) {
		t.Fatalf("opened %q, want only %s", got, reportName)
	}
	if got := revealed.paths(); len(got) != len(names)-len(opens) {
		t.Fatalf("revealed %q, want the %d unsafe files", got, len(names)-len(opens))
	}
}

func TestSafeToOpen(t *testing.T) {
	for name, want := range map[string]bool{
		"report.pdf":     true,
		"photo.PNG":      true,
		"clip.MP4":       true,
		"song.mp3":       true,
		"backup.tar.gz":  true,
		"uploads/x.pdf":  true,
		"x.pdf.":         true,
		"x.pdf ":         true,
		"x.pdf. .":       true,
		"run.command":    false,
		"setup.exe":      false,
		"x.exe.":         false,
		"x.exe ":         false,
		"x.EXE. .":       false,
		"x.pdf.exe":      false,
		"script.sh":      false,
		"tool.py":        false,
		"index.html":     false,
		"vector.svg":     false,
		"Logo.SVG. ":     false,
		"readme":         false,
		"x.lnk":          false,
		"":               false,
		"...":            false,
		"C:\\dir\\a.bat": false,
	} {
		if got := chat.SafeToOpen(name); got != want {
			t.Errorf("safeToOpen(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestUppercaseImageOpensFromLightbox(t *testing.T) {
	const name = "photo.PNG"
	m := withAttachments(message("m1", "worker", "w1", "Here.", time.Now().Add(-time.Hour)), name)
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{name: pngImage(t, photoW, photoH)}, m))
	f.awaitThumb(name, photoW*thumbHeight/photoH)
	opened, revealed := &opener{}, &opener{}
	f.view.SetOpener(opened.open)
	f.view.SetRevealer(revealed.open)
	f.click(locales.ChatViewImage(name))
	f.click(locales.ChatOpenOriginal())
	f.Fix.Await(t, func() bool { return len(opened.paths()) == 1 })
	if got := opened.paths()[0]; filepath.Base(got) != name {
		t.Fatalf("opened %q, want the cached %s", got, name)
	}
	if got := revealed.paths(); len(got) != 0 {
		t.Fatalf("revealed %q, want none", got)
	}
}

func TestRevealFailureToasts(t *testing.T) {
	m := withAttachments(message("m1", "worker", "w1", "Files.", time.Now().Add(-time.Hour)), "run.command")
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{"run.command": []byte("#!/bin/sh")}, m))
	opened, revealed := &opener{}, &opener{err: errors.New("no file manager")}
	f.view.SetOpener(opened.open)
	f.view.SetRevealer(revealed.open)
	f.wantHidden(locales.FileOpenFailed())
	f.click("run.command")
	f.Fix.Await(t, func() bool { return len(revealed.paths()) == 1 })
	f.settleMotion()
	f.wantShown(locales.FileOpenFailed())
	if got := opened.paths(); len(got) != 0 {
		t.Fatalf("opened %q after a failed reveal, want none", got)
	}
}

func TestPhotoDecodesAreBounded(t *testing.T) {
	const loads, slots = 6, 2
	var mu sync.Mutex
	var active, peak, fetched int
	release := make(chan struct{})
	var once sync.Once
	releaseAll := func() { once.Do(func() { close(release) }) }
	defer releaseAll()
	data := pngImage(t, 4, 4)
	ic := chat.NewImageCache(func(string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		fetched++
		return data, nil
	})
	ic.SetDecoder(func([]byte, bool, int) (ui.ImageSource, int64, error) {
		mu.Lock()
		active++
		peak = max(peak, active)
		mu.Unlock()
		<-release
		mu.Lock()
		active--
		mu.Unlock()
		return ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1))), 4, nil
	})
	var wg sync.WaitGroup
	for i := range loads {
		wg.Go(func() {
			if _, err := ic.LoadPhoto(fmt.Sprintf("p%d.png", i)); err != nil {
				t.Error(err)
			}
		})
	}
	state := func() (int, int, int) {
		mu.Lock()
		defer mu.Unlock()
		return fetched, active, peak
	}
	deadline := time.Now().Add(imageTimeout)
	for {
		if fetched, active, _ := state(); fetched == loads && active == slots {
			break
		}
		if time.Now().After(deadline) {
			fetched, active, peak := state()
			t.Fatalf("after %v: %d of %d photos downloaded, %d decoding (peak %d), want all downloaded and %d decoding", imageTimeout, fetched, loads, active, peak, slots)
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if _, active, peak := state(); active != slots || peak != slots {
		t.Fatalf("%d decoding (peak %d) while %d wait, want %d", active, peak, loads-slots, slots)
	}
	releaseAll()
	wg.Wait()
	if _, _, peak := state(); peak > slots {
		t.Fatalf("peak of %d concurrent decodes, want at most %d", peak, slots)
	}
	if ic.Len() != loads {
		t.Fatalf("%d photos cached, want %d", ic.Len(), loads)
	}
}

func TestPhotosLargerThanTheCacheStayLoaded(t *testing.T) {
	const photos, albumH, budget, settleFrames = 10, 3200, 4 << 20, 40
	data := grayPNG(t, 3000, 2000)
	media := map[string][]byte{}
	names := make([]string, photos)
	for i := range names {
		names[i] = fmt.Sprintf("album-%02d.png", i)
		media[names[i]] = data
	}
	f := newFixture(t, wideW, albumH, "w1", attached(media))
	f.view.SetImageBudget(budget)
	var mu sync.Mutex
	loads := map[string]int{}
	f.view.OnMediaLoad(func(name string) {
		mu.Lock()
		defer mu.Unlock()
		loads[name]++
	})
	f.Fake.Reply(workerCID, withAttachments(api.ChatMessage{ID: "album", Content: "The album."}, names...))
	shown := func() int {
		n := 0
		for _, name := range names {
			if _, ok := f.T.Find(locales.ChatViewImage(name)); ok {
				n++
			}
		}
		return n
	}
	deadline := time.Now().Add(bigPhotoTimeout)
	for shown() < photos {
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d photos shown after %v", shown(), photos, bigPhotoTimeout)
		}
		f.Settle()
		f.T.Frame()
		time.Sleep(5 * time.Millisecond)
	}
	for range settleFrames {
		f.Settle()
		f.T.Frame()
		time.Sleep(5 * time.Millisecond)
	}
	if n := shown(); n != photos {
		t.Fatalf("%d of %d photos shown after %d more frames", n, photos, settleFrames)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, name := range names {
		if loads[name] != 1 {
			t.Fatalf("loads = %v, want each photo loaded once", loads)
		}
	}
}

func TestThumbnailsDecodeSmallerThanTheLightbox(t *testing.T) {
	ic := chat.NewImageCache(func(string) ([]byte, error) { return grayPNG(t, landscapeW, landscapeH), nil })
	thumb, err := ic.LoadThumbnail("landscape.png")
	if err != nil {
		t.Fatal(err)
	}
	if w, h := thumb.(*ui.Bitmap).Size(); w != chat.ThumbEdge || h != chat.ThumbEdge*landscapeH/landscapeW {
		t.Fatalf("thumbnail is %d×%d, want %d wide", w, h, chat.ThumbEdge)
	}
	full, err := ic.LoadPhoto("landscape.png")
	if err != nil {
		t.Fatal(err)
	}
	if w, _ := full.(*ui.Bitmap).Size(); w != photoEdge {
		t.Fatalf("lightbox photo is %d wide, want %d", w, photoEdge)
	}
	if ic.Len() != 2 {
		t.Fatalf("%d images cached, want the thumbnail and the photo apart", ic.Len())
	}
}

func TestMediaFilesOverTheCapAreNotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.png")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, chat.MaxMediaFile+1); err != nil {
		t.Fatal(err)
	}
	if _, err := chat.ReadMediaFile(path); !errors.Is(err, chat.ErrImageTooLarge) {
		t.Fatalf("read of a %d byte file: err = %v, want ErrImageTooLarge", chat.MaxMediaFile+1, err)
	}
	if err := os.Truncate(path, 16); err != nil {
		t.Fatal(err)
	}
	if data, err := chat.ReadMediaFile(path); err != nil || len(data) != 16 {
		t.Fatalf("read of a small file = %d bytes, %v", len(data), err)
	}
}

func TestSVGAttachmentIsRevealedNotOpened(t *testing.T) {
	const name = "diagram.svg"
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="400" height="300"><rect width="400" height="300" fill="#36c"/></svg>`)
	m := withAttachments(message("m1", "worker", "w1", "A drawing.", time.Now().Add(-time.Hour)), name)
	f := newFixture(t, wideW, wideH, "w1", attached(map[string][]byte{name: svg}, m))
	f.awaitThumb(name, photoW*thumbHeight/photoH)
	opened, revealed := &opener{}, &opener{}
	f.view.SetOpener(opened.open)
	f.view.SetRevealer(revealed.open)
	f.click(locales.ChatViewImage(name))
	f.click(locales.ChatOpenOriginal())
	f.Fix.Await(t, func() bool { return len(revealed.paths()) == 1 })
	if got := opened.paths(); len(got) != 0 {
		t.Fatalf("opened %q, want the SVG revealed in its folder", got)
	}
}
