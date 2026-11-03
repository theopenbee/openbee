package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

const (
	canvasSize   = 1024
	markFraction = 0.8
	cornerRatio  = 0.224
	brandColor   = "#ec7b35"
)

func main() {
	svgPath := flag.String("svg", "../web/public/logo-mark.svg", "path to the logo mark SVG")
	outPath := flag.String("out", "resources/icon.png", "path of the PNG to write")
	flag.Parse()
	if err := run(*svgPath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "icon:", err)
		os.Exit(1)
	}
}

func run(svgPath, outPath string) error {
	data, err := os.ReadFile(svgPath)
	if err != nil {
		return err
	}
	data = bytes.ReplaceAll(data, []byte("var(--brand)"), []byte(brandColor))
	mark, err := ui.ParseSVG(data)
	if err != nil {
		return err
	}
	white := ui.Color{R: 255, G: 255, B: 255, A: 255}
	corner := float32(canvasSize) * cornerRatio
	markSize := float32(canvasSize) * markFraction
	view := func(c *ui.Context) {
		ui.Box(c).Fill().Center().Children(func() {
			ui.Box(c).Size(canvasSize, canvasSize).Center().Background(white).Radius(corner).Children(func() {
				ui.Image(c, mark).Size(markSize, markSize)
			})
		})
	}
	img := ui.Render(view, canvasSize, canvasSize, 1)
	clearOutsideRoundedSquare(img, float64(corner))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, buf.Bytes(), 0o644)
}

func clearOutsideRoundedSquare(img *image.RGBA, radius float64) {
	b := img.Bounds()
	half := float64(b.Dx()) / 2
	inner := half - radius
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			qx := math.Abs(float64(x-b.Min.X)+0.5-half) - inner
			qy := math.Abs(float64(y-b.Min.Y)+0.5-half) - inner
			dist := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
			coverage := math.Min(math.Max(0.5-dist, 0), 1)
			if coverage >= 1 {
				continue
			}
			v := uint8(math.Round(coverage * 255))
			img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: v})
		}
	}
}
