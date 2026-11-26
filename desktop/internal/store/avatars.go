package store

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	xdraw "golang.org/x/image/draw"
)

const (
	avatarCacheSize = 160
	avatarEdge      = 192
	avatarMaxPixels = 1 << 24
	avatarSlots     = 4
	avatarTimeout   = 30 * time.Second
	avatarRetry     = 30 * time.Second
)

var errAvatarTooLarge = errors.New("avatar image is too large to decode")

type avatarEntry struct {
	ref string
	src ui.ImageSource
}

type avatarImages struct {
	limit   int
	items   map[string]*list.Element
	order   list.List
	loading map[string]bool
	failed  map[string]time.Time
	slots   chan struct{}
	gen     uint64
}

func newAvatarImages(limit int) *avatarImages {
	a := &avatarImages{limit: max(1, limit), slots: make(chan struct{}, avatarSlots)}
	a.clear()
	return a
}

func (a *avatarImages) clear() {
	a.gen++
	a.items = map[string]*list.Element{}
	a.order.Init()
	a.loading = map[string]bool{}
	a.failed = map[string]time.Time{}
}

func (a *avatarImages) get(ref string) (ui.ImageSource, bool) {
	e, ok := a.items[ref]
	if !ok {
		return nil, false
	}
	a.order.MoveToFront(e)
	return e.Value.(*avatarEntry).src, true
}

func (a *avatarImages) put(ref string, src ui.ImageSource) {
	if e, ok := a.items[ref]; ok {
		e.Value.(*avatarEntry).src = src
		a.order.MoveToFront(e)
		return
	}
	a.items[ref] = a.order.PushFront(&avatarEntry{ref: ref, src: src})
	for a.order.Len() > a.limit {
		last := a.order.Back()
		a.order.Remove(last)
		delete(a.items, last.Value.(*avatarEntry).ref)
	}
}

func (a *avatarImages) dropPack(pluginID string) {
	prefix := pluginID + "/"
	for ref, e := range a.items {
		if strings.HasPrefix(ref, prefix) {
			a.order.Remove(e)
			delete(a.items, ref)
		}
	}
	for ref := range a.failed {
		if strings.HasPrefix(ref, prefix) {
			delete(a.failed, ref)
		}
	}
}

func (s *Store) initAvatars() {
	s.Avatars = NewResource(s, fetchAvatars)
	s.avatars = newAvatarImages(avatarCacheSize)
	s.resets = append(s.resets, s.avatars.clear)
}

func fetchAvatars(ctx context.Context, c *api.Client) ([]api.AvatarPack, error) {
	packs, err := c.Avatars(ctx)
	if errors.Is(err, api.ErrNotOpenBee) || api.IsStatus(err, http.StatusNotFound) {
		return []api.AvatarPack{}, nil
	}
	return packs, err
}

func (s *Store) AvatarImage(ref string) ui.ImageSource {
	if ref == "" || s.Phase != PhaseReady {
		return nil
	}
	a := s.avatars
	if src, ok := a.get(ref); ok {
		return src
	}
	if a.loading[ref] {
		return nil
	}
	if at, failed := a.failed[ref]; failed && s.o.Now().Sub(at) < avatarRetry {
		return nil
	}
	a.loading[ref] = true
	gen, slots := a.gen, a.slots
	s.async(func(ctx context.Context, c *api.Client) func() {
		src, err := loadAvatar(ctx, c, slots, ref)
		return func() {
			if a.gen != gen {
				return
			}
			delete(a.loading, ref)
			if err != nil {
				a.failed[ref] = s.o.Now()
				if !errors.Is(err, context.Canceled) {
					log.Printf("store: load avatar %s: %v", ref, err)
				}
				return
			}
			delete(a.failed, ref)
			a.put(ref, src)
		}
	}, nil)
	return nil
}

func (s *Store) avatarsChanged(dropped []string) {
	for _, id := range dropped {
		s.avatars.dropPack(id)
	}
	s.Avatars.Invalidate()
	s.invalidateWorkers()
}

func loadAvatar(ctx context.Context, c *api.Client, slots chan struct{}, ref string) (ui.ImageSource, error) {
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-slots }()
	ctx, cancel := context.WithTimeout(ctx, avatarTimeout)
	defer cancel()
	data, err := c.AvatarImage(ctx, ref)
	if err != nil {
		return nil, err
	}
	return decodeAvatar(data, avatarEdge)
}

func decodeAvatar(data []byte, edge int) (ui.ImageSource, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > avatarMaxPixels {
		return nil, fmt.Errorf("%w: %dx%d pixels", errAvatarTooLarge, cfg.Width, cfg.Height)
	}
	if max(cfg.Width, cfg.Height) <= edge {
		b, err := ui.DecodeBitmap(data)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	k := float64(edge) / float64(max(bounds.Dx(), bounds.Dy()))
	w, h := max(1, int(float64(bounds.Dx())*k+0.5)), max(1, int(float64(bounds.Dy())*k+0.5))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Src, nil)
	return ui.NewBitmap(dst), nil
}
