package chat_test

import (
	"bytes"
	"encoding/binary"
	"errors"
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
	photoName      = "photo.png"
	photoW         = 400
	photoH         = 300
	thumbHeight    = 240
	thumbBorder    = 2
	photoEdge      = 2560
	photoShort     = 853
	thumbSlack     = 0.5
	pixelSlack     = 6
	barHeight      = 76
	stageBottom    = 32
	stageMiddle    = (barHeight + wideH - stageBottom) / 2
	tallPhotoW     = 9000
	tallPhotoH     = 7000
	landscapeW     = 3000
	landscapeH     = 1000
	exifRotateCW   = 6
	declaredSide   = 20000
	reportName     = "report.pdf"
	reportContent  = "%PDF-1.7 report"
	lightboxEdge   = 20
	columnMax      = 768
	columnPadding  = 24
	controlsHeight = 32
	controlsLeft   = 80
	controlsRight  = 140
	narrowPhone    = 500
	longNameLength = 160
	panoramaW      = 1800
	panoramaH      = 300
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
	deadline := time.Now().Add(imageTimeout)
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
	f.awaitThumb("wide.png", float32(tallPhotoW)*thumbHeight/tallPhotoH)
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
