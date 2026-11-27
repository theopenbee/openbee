package chat

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	imageCopyFeedback = 1900 * time.Millisecond
	copyCommand       = "copy"
)

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

type imageCopy struct {
	busy     string
	copied   string
	copiedAt time.Time
}

func (v *View) copyInput(svc ui.Services, name string) func(ui.InputEvent) bool {
	return func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputCommand || ev.Text != copyCommand {
			return false
		}
		v.copyImage(svc, name)
		return true
	}
}

func (v *View) copyAction(c *ui.Context, name string, input func(ui.InputEvent) bool) viewerAction {
	text, icon := locales.ArcCopyButtonCopy(), "copy"
	if v.copied(c, name) {
		text, icon = locales.ArcCopyButtonCopied(), "check"
	}
	return viewerAction{
		text:    text,
		reserve: locales.ArcCopyButtonCopied(),
		label:   locales.ChatCopyImage(),
		icon:    icon,
		busy:    v.copying.busy == name,
		click:   func() { v.copyImage(c.Services(), name) },
		input:   input,
	}
}

func (v *View) copied(c *ui.Context, name string) bool {
	if v.copying.copied != name {
		return false
	}
	if v.copying.copiedAt.IsZero() {
		v.copying.copiedAt = c.Now()
	}
	left := imageCopyFeedback - c.Now().Sub(v.copying.copiedAt)
	if left <= 0 {
		v.copying.copied = ""
		return false
	}
	c.After(left)
	return true
}

func (v *View) copyImage(svc ui.Services, name string) {
	if v.copying.busy != "" {
		return
	}
	v.copying.busy = name
	gen := v.mediaGen
	v.s.MediaFile(name, func(localPath string, err error) {
		if v.mediaGen != gen {
			return
		}
		if err != nil {
			v.copyDone(svc, name, nil, err)
			return
		}
		go func() {
			data, err := clipboardPNG(localPath)
			v.s.Post(func() {
				if v.mediaGen == gen {
					v.copyDone(svc, name, data, err)
				}
			})
		}()
	})
}

func (v *View) copyDone(svc ui.Services, name string, data []byte, err error) {
	v.copying.busy = ""
	if err != nil {
		log.Printf("chat: copy image %s: %v", name, err)
		v.s.Toast(kit.Toast{Tone: kit.ToneDanger, Title: locales.ChatCopyImageFailed()})
		return
	}
	svc.WriteClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, data))))
	v.copying.copied, v.copying.copiedAt = name, time.Time{}
}

func clipboardPNG(path string) ([]byte, error) {
	data, err := readMediaFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, pngSignature) {
		return data, nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxPhotoPixels {
		return nil, fmt.Errorf("%w: %dx%d pixels", errImageTooLarge, cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if o := exifOrientation(data); o > 1 {
		img = orientPhoto(toRGBA(img), o)
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok && rgba.Rect.Min == (image.Point{}) {
		return rgba
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}
