package pluginmgr

import (
	"context"
	"maps"
	"math/rand/v2"
	"sort"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/plugin"
)

type avatarPack struct {
	name    plugin.LocalizedText
	avatars []plugin.Avatar
	byName  map[string]plugin.Avatar
}

type AvatarPack struct {
	ID      string
	Name    plugin.LocalizedText
	Avatars []string
}

func (m *Manager) LoadAvatars(ctx context.Context) {
	entries, err := m.deps.Registry.List()
	if err != nil {
		log.Error("list installed plugins", zap.Error(err))
		return
	}
	for _, e := range entries {
		if m.kind(e) != plugin.TypeAvatar {
			continue
		}
		row, found, err := m.deps.Store.Get(ctx, e.ID)
		if err != nil {
			log.Error("read plugin settings", zap.String("plugin", e.ID), zap.Error(err))
			continue
		}
		if !found || !row.Enabled {
			continue
		}
		unlock := m.lockID(e.ID)
		if err := m.startLocked(ctx, e.ID); err != nil {
			log.Error("load avatars", zap.String("plugin", e.ID), zap.Error(err))
		}
		unlock()
	}
}

func (m *Manager) launchAvatars(id string, man plugin.Manifest) error {
	avatars, err := plugin.LoadAvatars(man)
	if err != nil {
		return err
	}
	name := man.Name
	if len(name) == 0 {
		name = plugin.LocalizedText{"en": id}
	}
	byName := make(map[string]plugin.Avatar, len(avatars))
	for _, a := range avatars {
		byName[a.Name] = a
	}
	m.mu.Lock()
	m.avatars[id] = &avatarPack{name: name, avatars: avatars, byName: byName}
	m.mu.Unlock()
	log.Info("avatars registered", zap.String("plugin", id), zap.Int("count", len(avatars)))
	return nil
}

func (m *Manager) AvatarPacks() []AvatarPack {
	m.mu.Lock()
	out := make([]AvatarPack, 0, len(m.avatars))
	for id, pack := range m.avatars {
		refs := make([]string, len(pack.avatars))
		for i, a := range pack.avatars {
			refs[i] = plugin.AvatarRef(id, a.Name)
		}
		out = append(out, AvatarPack{ID: id, Name: maps.Clone(pack.name), Avatars: refs})
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *Manager) AvatarFile(ref string) (plugin.Avatar, bool) {
	id, name, ok := plugin.ParseAvatarRef(ref)
	if !ok {
		return plugin.Avatar{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	pack := m.avatars[id]
	if pack == nil {
		return plugin.Avatar{}, false
	}
	a, ok := pack.byName[name]
	return a, ok
}

func (m *Manager) HasAvatar(ref string) bool {
	_, ok := m.AvatarFile(ref)
	return ok
}

func (m *Manager) RandomAvatar(used map[string]bool) string {
	var all, free []string
	for _, pack := range m.AvatarPacks() {
		for _, ref := range pack.Avatars {
			all = append(all, ref)
			if !used[ref] {
				free = append(free, ref)
			}
		}
	}
	if len(free) > 0 {
		return free[rand.IntN(len(free))]
	}
	if len(all) > 0 {
		return all[rand.IntN(len(all))]
	}
	return ""
}
