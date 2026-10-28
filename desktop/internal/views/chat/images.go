package chat

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"io"
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
	maxExternalImage     = 10 << 20
	externalImageTimeout = 30 * time.Second
	bytesPerPixel        = 4
	svgSniffBytes        = 512
	mediaPathPrefix      = "/api/local/media/"
	mediaKeyPrefix       = "media:"
	svgExtension         = ".svg"
)

var (
	errImageAddress = errors.New("unsupported image address")
	errStaleSession = errors.New("the session that asked for the image has ended")
	imageClient     = &http.Client{Timeout: externalImageTimeout}
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

func newImageCache(media, web func(string) ([]byte, error)) *imageCache {
	return &imageCache{media: media, web: web, items: map[string]*list.Element{}}
}

func (ic *imageCache) Load(address string) (ui.ImageSource, error) {
	ref, ok := imageRefOf(address)
	if !ok {
		return nil, fmt.Errorf("%w: %q", errImageAddress, address)
	}
	if src, ok := ic.get(ref.key); ok {
		return src, nil
	}
	fetch, from := ic.web, ref.web
	if ref.media != "" {
		fetch, from = ic.media, ref.media
	}
	if fetch == nil {
		return nil, fmt.Errorf("%w: %q", errImageAddress, address)
	}
	data, err := fetch(from)
	if err != nil {
		return nil, err
	}
	src, size, err := decodeImage(data, ref.svg)
	if err != nil {
		return nil, fmt.Errorf("decode image %q: %w", address, err)
	}
	ic.put(ref.key, src, size)
	return src, nil
}

func (ic *imageCache) get(key string) (ui.ImageSource, bool) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	e, ok := ic.items[key]
	if !ok {
		return nil, false
	}
	ic.order.MoveToFront(e)
	return e.Value.(*cachedImage).src, true
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
