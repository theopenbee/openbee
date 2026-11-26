package store

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.RGBA{A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pngHeader(w, h uint32) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6
	chunk := append([]byte("IHDR"), ihdr...)
	var buf bytes.Buffer
	buf.WriteString("\x89PNG\r\n\x1a\n")
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(ihdr)))
	buf.Write(chunk)
	_ = binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return buf.Bytes()
}

func bitmapSize(t *testing.T, src ui.ImageSource) (int, int) {
	t.Helper()
	b, ok := src.(*ui.Bitmap)
	if !ok {
		t.Fatalf("decoded %T, want a bitmap", src)
	}
	return b.Size()
}

func TestDecodeAvatarScalesLargePictures(t *testing.T) {
	src, err := decodeAvatar(pngOf(t, 512, 256), 192)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := bitmapSize(t, src); w != 192 || h != 96 {
		t.Fatalf("scaled avatar = %dx%d, want 192x96", w, h)
	}
	src, err = decodeAvatar(pngOf(t, 128, 128), 192)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := bitmapSize(t, src); w != 128 || h != 128 {
		t.Fatalf("small avatar = %dx%d, want it kept at 128x128", w, h)
	}
}

func TestDecodeAvatarRejectsBadPictures(t *testing.T) {
	if _, err := decodeAvatar([]byte("not an image"), 192); err == nil {
		t.Fatal("decoded garbage")
	}
	if _, err := decodeAvatar(pngHeader(5000, 5000), 192); !errors.Is(err, errAvatarTooLarge) {
		t.Fatalf("a 5000x5000 picture: err = %v, want errAvatarTooLarge", err)
	}
}

func TestAvatarImagesEvictTheLeastRecentlyUsed(t *testing.T) {
	a := newAvatarImages(2)
	pics := map[string]ui.ImageSource{}
	for _, ref := range []string{"p/a", "p/b", "p/c", "q/d"} {
		pics[ref] = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	}
	a.put("p/a", pics["p/a"])
	a.put("p/b", pics["p/b"])
	if _, ok := a.get("p/a"); !ok {
		t.Fatal("p/a missing before the cache was full")
	}
	a.put("p/c", pics["p/c"])
	if _, ok := a.get("p/b"); ok {
		t.Fatal("p/b survived although p/a was used after it")
	}
	if src, ok := a.get("p/a"); !ok || src != pics["p/a"] {
		t.Fatal("the recently used p/a was evicted")
	}
	a.put("q/d", pics["q/d"])
	if a.order.Len() != 2 || len(a.items) != 2 {
		t.Fatalf("cache holds %d/%d entries, want 2", a.order.Len(), len(a.items))
	}
	a.dropPack("p")
	if _, ok := a.get("p/a"); ok {
		t.Fatal("dropPack kept p/a")
	}
	if _, ok := a.get("q/d"); !ok {
		t.Fatal("dropPack removed another pack's avatar")
	}
}
