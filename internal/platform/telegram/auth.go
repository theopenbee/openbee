package telegram

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
)

// AuthStore manages authorized Telegram user IDs with file-backed persistence.
type AuthStore struct {
	mu       sync.RWMutex
	users    map[string]bool // senderID -> authorized
	filePath string
}

func newAuthStore() *AuthStore {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".openbee", "telegram")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Error("create telegram auth dir", zap.Error(err))
	}
	s := &AuthStore{
		users:    make(map[string]bool),
		filePath: filepath.Join(dir, "authorized_users.json"),
	}
	s.load()
	return s
}

// IsAuthorized checks if a user ID is in the authorized list.
func (s *AuthStore) IsAuthorized(userID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.users[userID]
}

// Authorize adds a user ID to the authorized list and persists to disk.
func (s *AuthStore) Authorize(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[userID] = true
	s.save()
}

func (s *AuthStore) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Error("read auth store", zap.Error(err))
		}
		return
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		log.Error("parse auth store", zap.Error(err))
		return
	}
	for _, id := range ids {
		s.users[id] = true
	}
	log.Info("loaded authorized users", zap.Int("count", len(ids)))
}

func (s *AuthStore) save() {
	ids := make([]string, 0, len(s.users))
	for id := range s.users {
		ids = append(ids, id)
	}
	data, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		log.Error("marshal auth store", zap.Error(err))
		return
	}
	if err := os.WriteFile(s.filePath, data, 0o644); err != nil {
		log.Error("write auth store", zap.Error(err))
	}
}
