package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const (
	NotificationsOn  = "on"
	NotificationsOff = "off"
)

type Prefs struct {
	ServerURL                  string `json:"server_url"`
	LastServer                 string `json:"last_server"`
	Username                   string `json:"username"`
	Theme                      string `json:"theme"`
	Language                   string `json:"language"`
	Notifications              string `json:"notifications"`
	NotificationsHintDismissed bool   `json:"notifications_hint_dismissed"`
	NotificationsDenied        bool   `json:"notifications_denied"`
}

type Store interface {
	Load() Prefs
	Save(Prefs) error
}

type fileStore struct {
	mu   sync.Mutex
	path string
}

func NewFileStore(path string) Store {
	return &fileStore{path: path}
}

func (s *fileStore) Load() Prefs {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		return Prefs{}
	}
	var p Prefs
	if err := json.Unmarshal(data, &p); err != nil {
		return Prefs{}
	}
	return p
}

func (s *fileStore) Save(p Prefs) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeFileAtomic(s.path, data)
}

type memoryStore struct {
	mu sync.Mutex
	p  Prefs
}

func NewMemoryStore(p Prefs) Store {
	return &memoryStore{p: p}
}

func (s *memoryStore) Load() Prefs {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p
}

func (s *memoryStore) Save(p Prefs) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.p = p
	return nil
}

func writeFileAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
