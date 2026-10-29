package icons

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return img
}

func TestMenuBarIconIsTemplate(t *testing.T) {
	img := decode(t, MenuBarIcon(36))
	if b := img.Bounds(); b.Dx() != 36 || b.Dy() != 36 {
		t.Fatalf("size = %v, want 36x36", b)
	}
	opaque, clear := 0, 0
	for y := 0; y < 36; y++ {
		for x := 0; x < 36; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0 && (r != 0 || g != 0 || b != 0) {
				t.Fatalf("pixel (%d,%d) = %d,%d,%d, want black", x, y, r, g, b)
			}
			switch {
			case a > 0xe000:
				opaque++
			case a == 0:
				clear++
			}
		}
	}
	if opaque < 100 || clear < 100 {
		t.Errorf("opaque = %d, clear = %d pixels, want a mark on a transparent ground", opaque, clear)
	}
}

func TestAppIconCornersAreTransparent(t *testing.T) {
	img := decode(t, AppIcon(32, ""))
	for _, p := range []image.Point{{0, 0}, {31, 0}, {0, 31}, {31, 31}} {
		if _, _, _, a := img.At(p.X, p.Y).RGBA(); a != 0 {
			t.Errorf("corner %v alpha = %d, want 0", p, a)
		}
	}
	if _, _, _, a := img.At(16, 2).RGBA(); a != 0xffff {
		t.Errorf("top edge alpha = %d, want opaque", a)
	}
}

func redPixels(img image.Image) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a > 0xe000 && r > 0x9000 && g < 0x5000 && bl < 0x6000 {
				n++
			}
		}
	}
	return n
}

func TestAppIconBadge(t *testing.T) {
	plain := redPixels(decode(t, AppIcon(32, "")))
	badged := decode(t, AppIcon(32, "3"))
	if plain != 0 {
		t.Errorf("plain icon has %d red pixels, want 0", plain)
	}
	if n := redPixels(badged); n < 60 {
		t.Errorf("badged icon has %d red pixels, want a badge", n)
	}
	if r, g, b, _ := badged.At(4, 4).RGBA(); r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Errorf("top left = %d,%d,%d, want the white tile outside the badge", r, g, b)
	}
	if !bytes.Equal(AppIcon(32, "3"), AppIcon(32, "3")) {
		t.Error("AppIcon is not deterministic")
	}
	if bytes.Equal(AppIcon(32, "3"), AppIcon(32, "9+")) {
		t.Error("badges 3 and 9+ draw the same icon")
	}
}

func TestCountBadge(t *testing.T) {
	for _, badge := range []string{"3", "9+"} {
		img := decode(t, CountBadge(16, badge))
		if b := img.Bounds(); b.Dx() != 16 || b.Dy() != 16 {
			t.Fatalf("%q: size = %v, want 16x16", badge, b)
		}
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Errorf("%q: corner alpha = %d, want 0", badge, a)
		}
		if n := redPixels(img); n < 60 {
			t.Errorf("%q: %d red pixels, want a red badge", badge, n)
		}
		white := 0
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				if r, g, b, a := img.At(x, y).RGBA(); a > 0xe000 && r > 0xe000 && g > 0xe000 && b > 0xe000 {
					white++
				}
			}
		}
		if white == 0 {
			t.Errorf("%q: no white pixels, want the count drawn", badge)
		}
		for _, x := range []int{0, 15} {
			for y := 0; y < 16; y++ {
				if r, g, b, a := img.At(x, y).RGBA(); a > 0x8000 && r > 0xc000 && g > 0xc000 && b > 0xc000 {
					t.Errorf("%q: white pixel at the edge (%d,%d), want the count inside the badge", badge, x, y)
				}
			}
		}
	}
}
