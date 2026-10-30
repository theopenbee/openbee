package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const tokensFile = "tokens.json"

var errCorruptTokens = errors.New("tokens file is not valid JSON")

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

type fileTokenStore struct {
	mu   sync.Mutex
	path string
}

func NewFileTokenStore(dir string) TokenStore {
	return &fileTokenStore{path: filepath.Join(dir, tokensFile)}
}

func (s *fileTokenStore) Load(server string) (Tokens, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.read()
	if err != nil {
		return Tokens{}, err
	}
	return Tokens(entries[server]), nil
}

func (s *fileTokenStore) Save(server string, t Tokens) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.read()
	switch {
	case errors.Is(err, errCorruptTokens):
		entries = map[string]tokenRecord{}
	case err != nil:
		return err
	}
	entries[server] = tokenRecord(t)
	return s.write(entries)
}

func (s *fileTokenStore) Clear(server string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.read()
	switch {
	case errors.Is(err, errCorruptTokens):
		return s.write(map[string]tokenRecord{})
	case err != nil:
		return err
	}
	if _, ok := entries[server]; !ok {
		return nil
	}
	delete(entries, server)
	return s.write(entries)
}

func (s *fileTokenStore) read() (map[string]tokenRecord, error) {
	entries := map[string]tokenRecord{}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%w: %w", errCorruptTokens, err)
	}
	if entries == nil {
		entries = map[string]tokenRecord{}
	}
	return entries, nil
}

func (s *fileTokenStore) write(entries map[string]tokenRecord) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, data)
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
