package worker

import (
	"fmt"

	"go.uber.org/zap"
)

type AvatarSource interface {
	HasAvatar(ref string) bool
	RandomAvatar(used map[string]bool) string
}

func (m *Manager) SetAvatarSource(src AvatarSource) {
	m.mu.Lock()
	m.avatars = src
	m.mu.Unlock()
}

func (m *Manager) avatarSource() AvatarSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.avatars
}

func (m *Manager) HasAvatar(ref string) bool {
	src := m.avatarSource()
	return ref != "" && src != nil && src.HasAvatar(ref)
}

func (m *Manager) validateAvatar(ref string) error {
	if ref == "" {
		return nil
	}
	if !m.HasAvatar(ref) {
		return fmt.Errorf("avatar %q is not available: %w", ref, ErrValidation)
	}
	return nil
}

func (m *Manager) pickAvatar() string {
	src := m.avatarSource()
	if src == nil {
		return ""
	}
	refs, err := m.workerStore.ListAvatars()
	if err != nil {
		log.Warn("list worker avatars", zap.Error(err))
	}
	used := make(map[string]bool, len(refs))
	for _, ref := range refs {
		used[ref] = true
	}
	return src.RandomAvatar(used)
}
