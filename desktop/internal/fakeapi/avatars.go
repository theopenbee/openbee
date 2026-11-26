package fakeapi

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const (
	typeAvatar     = "avatar"
	avatarPixels   = 128
	avatarCache    = "private, max-age=86400"
	avatarMix      = 2654435761
	avatarHueShift = 7
)

func AvatarPlugin(id string, name api.LocalizedText) api.PluginDetail {
	if name == nil {
		name = api.LocalizedText{"en": id}
	}
	return api.PluginDetail{
		PluginStatus: api.PluginStatus{
			ID: id, Type: typeAvatar, Package: "@openbee/avatars-" + id, Source: "npm", Version: "1.0.0",
			Path: "/tmp/openbee/plugins/" + id, Name: name, Enabled: true, State: stateReady,
		},
		Config: map[string]any{},
		Fields: []api.PluginConfigField{},
	}
}

func AvatarNames(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("%05d", i+1)
	}
	return names
}

func (st *State) AddAvatarPack(id string, name api.LocalizedText, count int) []string {
	st.Plugins = append(st.Plugins, AvatarPlugin(id, name))
	if st.Avatars == nil {
		st.Avatars = map[string][]string{}
	}
	st.Avatars[id] = AvatarNames(count)
	refs := make([]string, count)
	for i, n := range st.Avatars[id] {
		refs[i] = id + "/" + n
	}
	return refs
}

func (s *Server) avatarRoutes() {
	s.route("GET /api/avatars", authUser, s.listAvatars)
	s.raw("GET /api/avatars/{plugin}/{name}", authUser, s.serveAvatar)
}

func (s *Server) avatarPacks() []api.AvatarPack {
	var packs []api.AvatarPack
	for _, p := range s.st.Plugins {
		names, found := s.st.Avatars[p.ID]
		if p.Type != typeAvatar || !p.Enabled || !found {
			continue
		}
		refs := make([]string, len(names))
		for i, n := range names {
			refs[i] = p.ID + "/" + n
		}
		packs = append(packs, api.AvatarPack{ID: p.ID, Name: outDetail(p).Name, Avatars: refs})
	}
	slices.SortFunc(packs, func(a, b api.AvatarPack) int { return strings.Compare(a.ID, b.ID) })
	return packs
}

func (s *Server) hasAvatar(ref string) bool {
	plugin, name, ok := api.SplitAvatar(ref)
	if !ok {
		return false
	}
	return slices.ContainsFunc(s.avatarPacks(), func(p api.AvatarPack) bool {
		return p.ID == plugin && slices.Contains(p.Avatars, plugin+"/"+name)
	})
}

func (s *Server) randomAvatar() string {
	used := map[string]bool{}
	for _, w := range s.st.Workers {
		used[w.Avatar] = true
	}
	var all, free []string
	for _, p := range s.avatarPacks() {
		for _, ref := range p.Avatars {
			all = append(all, ref)
			if !used[ref] {
				free = append(free, ref)
			}
		}
	}
	pool := free
	if len(pool) == 0 {
		pool = all
	}
	if len(pool) == 0 {
		return ""
	}
	return pool[rand.IntN(len(pool))]
}

func (s *Server) listAvatars(*request) reply {
	packs := s.avatarPacks()
	if packs == nil {
		packs = []api.AvatarPack{}
	}
	return ok(map[string]any{"packs": packs})
}

func (s *Server) serveAvatar(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("plugin") + "/" + r.PathValue("name")
	s.mu.Lock()
	found := s.hasAvatar(ref)
	s.mu.Unlock()
	if !found {
		codedReply(http.StatusNotFound, "avatar_not_found", "avatar not found", nil).write(w)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "image/png")
	header.Set("Cache-Control", avatarCache)
	header.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(AvatarPNG(ref))
}

func AvatarPNG(ref string) []byte {
	h := fnv.New32a()
	_, _ = h.Write([]byte(ref))
	seed := h.Sum32() * avatarMix
	hue := float64((seed>>avatarHueShift)%360) / 360
	bg := hsl(hue, 0.55, 0.78)
	face := hsl(math.Mod(hue+0.5, 1), 0.45, 0.62)
	ink := hsl(hue, 0.35, 0.22)
	img := image.NewRGBA(image.Rect(0, 0, avatarPixels, avatarPixels))
	const c = avatarPixels / 2
	headR := float64(avatarPixels) * (0.28 + float64((seed>>9)%5)/100)
	eyeY := c - headR*0.15
	eyeDX := headR * 0.38
	eyeR := headR * 0.11
	for y := range avatarPixels {
		for x := range avatarPixels {
			px, py := float64(x)+0.5, float64(y)+0.5
			col := bg
			if dist(px, py, c, c+6) <= headR {
				col = face
			}
			if dist(px, py, c-eyeDX, eyeY+6) <= eyeR || dist(px, py, c+eyeDX, eyeY+6) <= eyeR {
				col = ink
			}
			if mouth := dist(px, py, c, c+6); mouth >= headR*0.42 && mouth <= headR*0.52 && py > c+6+headR*0.2 {
				col = ink
			}
			img.SetRGBA(x, y, col)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func dist(x, y, cx, cy float64) float64 {
	return math.Hypot(x-cx, y-cy)
}

func hsl(h, s, l float64) color.RGBA {
	q := l + s - l*s
	if l < 0.5 {
		q = l * (1 + s)
	}
	p := 2*l - q
	channel := func(t float64) uint8 {
		t = math.Mod(t+1, 1)
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return uint8(math.Round(v * 255))
	}
	return color.RGBA{R: channel(h + 1.0/3), G: channel(h), B: channel(h - 1.0/3), A: 255}
}
