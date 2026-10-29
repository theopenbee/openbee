package icons

import (
	"bytes"
	"image"
	"image/png"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	appIconCorner   = 0.224
	appIconMark     = 0.8
	badgeDiameter   = 0.62
	badgeFontScale  = 0.74
	wideFontScale   = 0.6
	badgeFontWeight = 700
)

var (
	white = ui.Color{R: 255, G: 255, B: 255, A: 255}
	black = ui.Color{A: 255}
)

func MenuBarIcon(size int) []byte {
	s := float32(size)
	mark := LogoMark(theme.Light())
	img := renderTransparent(size, func(c *ui.Context) {
		ui.Image(c, mark).Size(s, s)
	})
	ink := make([]float64, len(img.Pix)/4)
	darkest := 0.0
	for i := range ink {
		p := img.Pix[i*4 : i*4+4]
		lum := (0.299*float64(p[0]) + 0.587*float64(p[1]) + 0.114*float64(p[2])) / 255
		ink[i] = 1 - lum
		if p[3] == 255 {
			darkest = max(darkest, ink[i])
		}
	}
	for i := range ink {
		p := img.Pix[i*4 : i*4+4]
		alpha := float64(p[3])
		if darkest > 0 {
			alpha *= min(ink[i]/darkest, 1)
		}
		p[0], p[1], p[2], p[3] = 0, 0, 0, uint8(alpha+0.5)
	}
	return encode(img)
}

func AppIcon(size int, badge string) []byte {
	s := float32(size)
	mark := LogoMark(theme.Light())
	img := renderTransparent(size, func(c *ui.Context) {
		ui.Box(c).Size(s, s).Center().Background(white).Radius(s * appIconCorner).Children(func() {
			ui.Image(c, mark).Size(s*appIconMark, s*appIconMark)
		})
		if badge != "" {
			countBadge(c, badge, s*badgeDiameter)
		}
	})
	return encode(img)
}

func CountBadge(size int, badge string) []byte {
	img := renderTransparent(size, func(c *ui.Context) {
		countBadge(c, badge, float32(size))
	})
	return encode(img)
}

func countBadge(c *ui.Context, badge string, d float32) {
	scale := float32(badgeFontScale)
	if len(badge) > 1 {
		scale = wideFontScale
	}
	ui.Row(c).Absolute().Right(0).Bottom(0).Height(d).MinWidth(d).Radius(d / 2).
		Background(theme.Light().Danger).Center().Children(func() {
		ui.Text(c, badge).FontSize(d * scale).FontWeight(badgeFontWeight).TextColor(white).
			FontFeatures("tnum").LineHeight(1).SingleLine()
	})
}

func renderTransparent(size int, content func(c *ui.Context)) *image.NRGBA {
	s := float32(size)
	over := func(bg ui.Color) *image.RGBA {
		return ui.Render(func(c *ui.Context) {
			ui.Box(c).Size(s, s).Background(bg).Children(func() { content(c) })
		}, size, size, 1)
	}
	onWhite, onBlack := over(white), over(black)
	out := image.NewNRGBA(onWhite.Rect)
	for i := 0; i < len(out.Pix); i += 4 {
		var sum int
		for k := range 3 {
			sum += int(onWhite.Pix[i+k]) - int(onBlack.Pix[i+k])
		}
		a := 255 - min(max(sum/3, 0), 255)
		out.Pix[i+3] = uint8(a)
		if a == 0 {
			continue
		}
		for k := range 3 {
			out.Pix[i+k] = uint8(min(int(onBlack.Pix[i+k])*255/a, 255))
		}
	}
	return out
}

func encode(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
