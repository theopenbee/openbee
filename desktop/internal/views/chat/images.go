package chat

import (
	"bytes"
	"container/list"
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
	svgExtension         = ".svg"
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
	media func(name string) ([]byte, error)
	web   func(url string) ([]byte, error)

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
	return &imageCache{media: media, web: web, items: map[string]*list.Element{}}
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
