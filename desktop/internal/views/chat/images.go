package chat

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
)

const (
	maxCachedImages      = 64
	maxCachedImageBytes  = 128 << 20
	markdownImageBudget  = 2 * maxCachedImageBytes
	maxExternalImage     = 10 << 20
	externalImageTimeout = 30 * time.Second
	bytesPerPixel        = 4
	svgSniffBytes        = 512
	mediaPathPrefix      = "/api/local/media/"
	mediaKeyPrefix       = "media:"
	photoKeyPrefix       = "photo:"
	svgExtension         = ".svg"
	photoEdge            = 2560
	photoDecodeSlots     = 2
	maxPhotoPixels       = 100_000_000
	jpegPrefix           = 0xff
	jpegStart            = 0xd8
	jpegScan             = 0xda
	jpegExif             = 0xe1
	jpegMarkerSize       = 2
	jpegSegmentHead      = 4
	exifSignature        = "Exif\x00\x00"
	exifOrientationTag   = 0x0112
	exifTypeShort        = 3
	exifEntrySize        = 12
	exifFirstIFD         = 8
)

var (
	errImageAddress  = errors.New("unsupported image address")
	errImageDecode   = errors.New("decode image")
	errImageTooLarge = errors.New("image is too large to decode")
	errStaleSession  = errors.New("the session that asked for the image has ended")
	imageClient      = &http.Client{Timeout: externalImageTimeout}
)

type imageRef struct {
	key, media, web string
	svg             bool
}

type cachedImage struct {
	key  string
	src  ui.ImageSource
	size int64
}

type imageCache struct {
	media  func(name string) ([]byte, error)
	web    func(url string) ([]byte, error)
	decode func(data []byte, svg bool) (ui.ImageSource, int64, error)
	slots  chan struct{}

	mu    sync.Mutex
	items map[string]*list.Element
	order list.List
	size  int64
}

type mediaResult struct {
	path string
	err  error
}

type imageLoads struct {
	mu     sync.Mutex
	known  map[string]bool
	seen   map[string]bool
	bytes  int64
	fresh  bool
	failed bool
}

func newImageLoads(prev *imageLoads) *imageLoads {
	l := &imageLoads{seen: map[string]bool{}}
	if prev != nil {
		prev.mu.Lock()
		l.known = maps.Clone(prev.seen)
		prev.mu.Unlock()
	}
	return l
}

func (l *imageLoads) track(ic *imageCache) func(address string) (ui.ImageSource, error) {
	return func(address string) (ui.ImageSource, error) {
		src, size, err := ic.load(address)
		l.note(address, size, err)
		return src, err
	}
}

func (l *imageLoads) note(address string, size int64, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case err != nil:
		l.failed = l.failed || !errors.Is(err, errImageAddress) && !errors.Is(err, errImageDecode)
	case !l.seen[address]:
		l.seen[address] = true
		l.bytes += size
		l.fresh = l.fresh || !l.known[address]
	}
}

func (l *imageLoads) due(retryFailed bool) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.fresh && l.bytes > markdownImageBudget || retryFailed && l.failed
}

func newImageCache(media, web func(string) ([]byte, error)) *imageCache {
	return &imageCache{media: media, web: web, decode: decodePhoto, slots: make(chan struct{}, photoDecodeSlots), items: map[string]*list.Element{}}
}

func (ic *imageCache) Load(address string) (ui.ImageSource, error) {
	src, _, err := ic.load(address)
	return src, err
}

func (ic *imageCache) load(address string) (ui.ImageSource, int64, error) {
	ref, ok := imageRefOf(address)
	if !ok {
		return nil, 0, fmt.Errorf("%w: %q", errImageAddress, address)
	}
	if src, size, ok := ic.get(ref.key); ok {
		return src, size, nil
	}
	fetch, from := ic.web, ref.web
	if ref.media != "" {
		fetch, from = ic.media, ref.media
	}
	if fetch == nil {
		return nil, 0, fmt.Errorf("%w: %q", errImageAddress, address)
	}
	data, err := fetch(from)
	if err != nil {
		return nil, 0, err
	}
	src, size, err := decodeImage(data, ref.svg)
	if err != nil {
		return nil, 0, fmt.Errorf("%w %q: %w", errImageDecode, address, err)
	}
	ic.put(ref.key, src, size)
	return src, size, nil
}

func (ic *imageCache) loadPhoto(name string) (ui.ImageSource, error) {
	key := photoKey(name)
	if src, _, ok := ic.get(key); ok {
		return src, nil
	}
	if ic.media == nil {
		return nil, fmt.Errorf("%w: %q", errImageAddress, name)
	}
	data, err := ic.media(name)
	if err != nil {
		return nil, err
	}
	src, size, err := ic.decodeBounded(data, hasSVGExtension(name))
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", errImageDecode, name, err)
	}
	ic.put(key, src, size)
	return src, nil
}

func (ic *imageCache) decodeBounded(data []byte, svg bool) (ui.ImageSource, int64, error) {
	ic.slots <- struct{}{}
	defer func() { <-ic.slots }()
	return ic.decode(data, svg)
}

func photoKey(name string) string {
	return photoKeyPrefix + name
}

func (ic *imageCache) get(key string) (ui.ImageSource, int64, bool) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	e, ok := ic.items[key]
	if !ok {
		return nil, 0, false
	}
	ic.order.MoveToFront(e)
	img := e.Value.(*cachedImage)
	return img.src, img.size, true
}

func (ic *imageCache) put(key string, src ui.ImageSource, size int64) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if e, ok := ic.items[key]; ok {
		img := e.Value.(*cachedImage)
		ic.size += size - img.size
		img.src, img.size = src, size
		ic.order.MoveToFront(e)
	} else {
		ic.items[key] = ic.order.PushFront(&cachedImage{key: key, src: src, size: size})
		ic.size += size
	}
	for ic.order.Len() > 1 && (ic.order.Len() > maxCachedImages || ic.size > maxCachedImageBytes) {
		img := ic.order.Remove(ic.order.Back()).(*cachedImage)
		delete(ic.items, img.key)
		ic.size -= img.size
	}
}

func imageRefOf(address string) (imageRef, bool) {
	u, err := url.Parse(address)
	if err != nil {
		return imageRef{}, false
	}
	switch {
	case u.Scheme == "http" || u.Scheme == "https":
		return imageRef{key: address, web: address, svg: hasSVGExtension(u.Path)}, u.Host != ""
	case u.Scheme != "" || u.Host != "" || u.Opaque != "":
		return imageRef{}, false
	}
	name, inMedia := strings.CutPrefix(u.Path, mediaPathPrefix)
	if !inMedia && strings.HasPrefix(u.Path, "/") || name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return imageRef{}, false
	}
	return imageRef{key: mediaKeyPrefix + name, media: name, svg: hasSVGExtension(name)}, true
}

func hasSVGExtension(p string) bool {
	return strings.EqualFold(path.Ext(p), svgExtension)
}

func decodeImage(data []byte, svg bool) (ui.ImageSource, int64, error) {
	if svg || looksLikeSVG(data) {
		s, err := ui.ParseSVG(data)
		if err != nil {
			return nil, 0, err
		}
		return s, max(1, int64(len(data))), nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	if int64(cfg.Width)*int64(cfg.Height)*bytesPerPixel > maxCachedImageBytes {
		return nil, 0, fmt.Errorf("%w: %dx%d pixels", errImageTooLarge, cfg.Width, cfg.Height)
	}
	b, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, 0, err
	}
	w, h := b.Size()
	return b, int64(w) * int64(h) * bytesPerPixel, nil
}

func decodePhoto(data []byte, svg bool) (ui.ImageSource, int64, error) {
	if svg || looksLikeSVG(data) {
		return decodeImage(data, true)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	switch {
	case int64(cfg.Width)*int64(cfg.Height) > maxPhotoPixels:
		return nil, 0, fmt.Errorf("%w: %dx%d pixels", errImageTooLarge, cfg.Width, cfg.Height)
	case max(cfg.Width, cfg.Height) <= photoEdge:
		return decodeImage(data, false)
	}
	img, err := scalePhoto(data)
	if err != nil {
		return nil, 0, err
	}
	return ui.NewBitmap(img), int64(img.Rect.Dx()) * int64(img.Rect.Dy()) * bytesPerPixel, nil
}

func scalePhoto(data []byte) (*image.RGBA, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	for {
		b := src.Bounds()
		if max(b.Dx(), b.Dy()) <= 2*photoEdge {
			break
		}
		src = resample(src, max(1, b.Dx()/2), max(1, b.Dy()/2))
	}
	b := src.Bounds()
	k := float64(photoEdge) / float64(max(b.Dx(), b.Dy()))
	scaled := resample(src, max(1, int(float64(b.Dx())*k+0.5)), max(1, int(float64(b.Dy())*k+0.5)))
	return orientPhoto(scaled, exifOrientation(data)), nil
}

func resample(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return dst
}

func exifOrientation(jpg []byte) int {
	if len(jpg) < jpegMarkerSize || jpg[0] != jpegPrefix || jpg[1] != jpegStart {
		return 0
	}
	for i := jpegMarkerSize; i+jpegSegmentHead <= len(jpg); {
		if jpg[i] != jpegPrefix {
			return 0
		}
		marker, size := jpg[i+1], int(binary.BigEndian.Uint16(jpg[i+jpegMarkerSize:]))
		if marker == jpegScan || size < jpegMarkerSize || i+jpegMarkerSize+size > len(jpg) {
			return 0
		}
		segment := jpg[i+jpegSegmentHead : i+jpegMarkerSize+size]
		if marker == jpegExif && bytes.HasPrefix(segment, []byte(exifSignature)) {
			return tiffOrientation(segment[len(exifSignature):])
		}
		i += jpegMarkerSize + size
	}
	return 0
}

func tiffOrientation(t []byte) int {
	if len(t) < exifFirstIFD {
		return 0
	}
	var order binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0
	}
	ifd := int(order.Uint32(t[4:]))
	if ifd < exifFirstIFD || ifd+2 > len(t) {
		return 0
	}
	entries := int(order.Uint16(t[ifd:]))
	for at := ifd + 2; entries > 0 && at+exifEntrySize <= len(t); at, entries = at+exifEntrySize, entries-1 {
		if order.Uint16(t[at:]) == exifOrientationTag && order.Uint16(t[at+2:]) == exifTypeShort {
			if o := int(order.Uint16(t[at+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 0
		}
	}
	return 0
}

func orientPhoto(img *image.RGBA, o int) *image.RGBA {
	if o <= 1 {
		return img
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		for x := range dw {
			sx, sy := x, y
			switch o {
			case 2:
				sx = w - 1 - x
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sy = h - 1 - y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			}
			copy(out.Pix[out.PixOffset(x, y):][:bytesPerPixel], img.Pix[img.PixOffset(sx, sy):])
		}
	}
	return out
}

func looksLikeSVG(data []byte) bool {
	return bytes.Contains(data[:min(len(data), svgSniffBytes)], []byte("<svg"))
}

func fetchExternalImage(address string) ([]byte, error) {
	resp, err := imageClient.Get(address)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch image %q: %s", address, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxExternalImage+1))
	if err != nil {
		return nil, fmt.Errorf("fetch image %q: %w", address, err)
	}
	if len(data) > maxExternalImage {
		return nil, fmt.Errorf("image %q is larger than %d bytes", address, maxExternalImage)
	}
	return data, nil
}

func (v *View) mediaImages(gen uint64) func(name string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		done := make(chan mediaResult, 1)
		v.s.Post(func() {
			if v.mediaGen != gen {
				done <- mediaResult{err: errStaleSession}
				return
			}
			v.s.MediaFile(name, func(localPath string, err error) { done <- mediaResult{path: localPath, err: err} })
		})
		r := <-done
		if r.err != nil {
			return nil, r.err
		}
		return os.ReadFile(r.path)
	}
}
