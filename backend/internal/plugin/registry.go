package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const (
	SourceNpm  = "npm"
	SourcePath = "path"

	registryFile = "plugins.json"
)

type Entry struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Package string `json:"package,omitempty"`
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
}

type registryData struct {
	Plugins map[string]Entry `json:"plugins"`
}

type Registry struct {
	dir string
}

func DefaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", "plugins")
}

func OpenRegistry(dir string) *Registry {
	return &Registry{dir: dir}
}

func (r *Registry) Dir() string { return r.dir }

func (r *Registry) DataDir(id string) string {
	return filepath.Join(r.dir, "data", id)
}

func (r *Registry) load() (registryData, error) {
	data, err := os.ReadFile(filepath.Join(r.dir, registryFile))
	if errors.Is(err, fs.ErrNotExist) {
		return registryData{Plugins: map[string]Entry{}}, nil
	}
	if err != nil {
		return registryData{}, fmt.Errorf("read %s: %w", registryFile, err)
	}
	var d registryData
	if err := json.Unmarshal(data, &d); err != nil {
		return registryData{}, fmt.Errorf("parse %s: %w", registryFile, err)
	}
	if d.Plugins == nil {
		d.Plugins = map[string]Entry{}
	}
	return d, nil
}

func (r *Registry) save(d registryData) error {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(r.dir, registryFile+".*")
	if err != nil {
		return fmt.Errorf("write %s: %w", registryFile, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", registryFile, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", registryFile, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", registryFile, err)
	}
	return os.Rename(tmp.Name(), filepath.Join(r.dir, registryFile))
}

func (r *Registry) List() ([]Entry, error) {
	d, err := r.load()
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(d.Plugins))
	for _, e := range d.Plugins {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *Registry) Get(id string) (Entry, bool, error) {
	d, err := r.load()
	if err != nil {
		return Entry{}, false, err
	}
	e, ok := d.Plugins[id]
	return e, ok, nil
}

func (r *Registry) Put(e Entry) error {
	d, err := r.load()
	if err != nil {
		return err
	}
	d.Plugins[e.ID] = e
	return r.save(d)
}

func (r *Registry) Remove(id string) error {
	d, err := r.load()
	if err != nil {
		return err
	}
	delete(d.Plugins, id)
	return r.save(d)
}
