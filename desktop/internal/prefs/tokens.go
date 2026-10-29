package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"
)

const (
	secretKeyPrefix    = "tokens:"
	fallbackTokensFile = "tokens.json"
)

var errCorruptFallback = errors.New("token fallback file is not valid JSON")

type Tokens struct{ Access, Refresh string }

type TokenStore interface {
	Load(server string) (Tokens, error)
	Save(server string, t Tokens) error
	Clear(server string) error
}

type tokenRecord struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
}

type secrets interface {
	Set(key string, value []byte) error
	Get(key string) ([]byte, error)
	Delete(key string) error
}

type keychain struct{}

func (keychain) Set(key string, value []byte) error { return mygo.Secrets.Set(key, value) }

func (keychain) Get(key string) ([]byte, error) { return mygo.Secrets.Get(key) }

func (keychain) Delete(key string) error { return mygo.Secrets.Delete(key) }

type secretTokenStore struct {
	mu          sync.Mutex
	backend     secrets
	fallbackDir string
}

func NewSecretTokenStore(fallbackDir string) TokenStore {
	return newSecretTokenStore(keychain{}, fallbackDir)
}

func newSecretTokenStore(backend secrets, fallbackDir string) *secretTokenStore {
	return &secretTokenStore{backend: backend, fallbackDir: fallbackDir}
}

func (s *secretTokenStore) Load(server string) (Tokens, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := s.backend.Get(secretKeyPrefix + server)
	switch {
	case err == nil:
		var rec tokenRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return Tokens{}, err
		}
		return Tokens(rec), nil
	case errors.Is(err, mygo.ErrSecretNotFound):
		return Tokens{}, nil
	case errors.Is(err, errors.ErrUnsupported):
		entries, err := s.readFallback()
		if err != nil {
			return Tokens{}, err
		}
		return Tokens(entries[server]), nil
	default:
		return Tokens{}, err
	}
}

func (s *secretTokenStore) Save(server string, t Tokens) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.Marshal(tokenRecord(t))
	if err != nil {
		return err
	}
	err = s.backend.Set(secretKeyPrefix+server, raw)
	switch {
	case err == nil:
		return s.removeFallback(server)
	case !errors.Is(err, errors.ErrUnsupported):
		return err
	}
	entries, err := s.fallbackForWrite()
	if err != nil {
		return err
	}
	entries[server] = tokenRecord(t)
	return s.writeFallback(entries)
}

func (s *secretTokenStore) Clear(server string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.backend.Delete(secretKeyPrefix + server)
	if errors.Is(err, mygo.ErrSecretNotFound) || errors.Is(err, errors.ErrUnsupported) {
		err = nil
	}
	return errors.Join(err, s.removeFallback(server))
}

func (s *secretTokenStore) removeFallback(server string) error {
	entries, err := s.readFallback()
	switch {
	case errors.Is(err, errCorruptFallback):
		return s.writeFallback(map[string]tokenRecord{})
	case err != nil:
		return err
	}
	if _, ok := entries[server]; !ok {
		return nil
	}
	delete(entries, server)
	return s.writeFallback(entries)
}

func (s *secretTokenStore) fallbackForWrite() (map[string]tokenRecord, error) {
	entries, err := s.readFallback()
	if errors.Is(err, errCorruptFallback) {
		return map[string]tokenRecord{}, nil
	}
	return entries, err
}

func (s *secretTokenStore) readFallback() (map[string]tokenRecord, error) {
	entries := map[string]tokenRecord{}
	data, err := os.ReadFile(filepath.Join(s.fallbackDir, fallbackTokensFile))
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%w: %w", errCorruptFallback, err)
	}
	if entries == nil {
		entries = map[string]tokenRecord{}
	}
	return entries, nil
}

func (s *secretTokenStore) writeFallback(entries map[string]tokenRecord) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(s.fallbackDir, fallbackTokensFile), data)
}

type memoryTokenStore struct {
	mu      sync.Mutex
	entries map[string]Tokens
}

func NewMemoryTokenStore() TokenStore {
	return &memoryTokenStore{entries: map[string]Tokens{}}
}

func (s *memoryTokenStore) Load(server string) (Tokens, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entries[server], nil
}

func (s *memoryTokenStore) Save(server string, t Tokens) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[server] = t
	return nil
}

func (s *memoryTokenStore) Clear(server string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, server)
	return nil
}
