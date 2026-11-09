package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

const testMark = "../../../web/public/logo-mark.svg"

func runPNG(t *testing.T, size int, tile bool) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "icon.png")
	if err := run(testMark, out, size, tile); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRunDrawsTheTiledIconAtTheRequestedSize(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(runPNG(t, 64, true)))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 64 {
		t.Fatalf("size = %v, want 64x64", b)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Errorf("corner alpha = %d, want transparent", a)
	}
	if r, g, b, a := img.At(32, 2).RGBA(); a != 0xffff || r != 0xffff || g != 0xffff || b != 0xffff {
		t.Errorf("tile edge = %v, want opaque white", img.At(32, 2))
	}
}

func TestRunWithoutTileDrawsOnlyTheMark(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(runPNG(t, 64, false)))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 64 {
		t.Fatalf("size = %v, want 64x64", b)
	}
	opaque := 0
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0xffff && r > 0xf000 && g > 0xf000 && b > 0xf000 {
				t.Fatalf("white pixel at %d,%d: no tile expected", x, y)
			}
			if a == 0xffff {
				opaque++
			}
		}
	}
	if _, _, _, a := img.At(32, 2).RGBA(); a != 0 {
		t.Errorf("top margin alpha = %d, want transparent", a)
	}
	if opaque < 64*64/4 {
		t.Errorf("only %d opaque pixels, the mark is missing", opaque)
	}
}
