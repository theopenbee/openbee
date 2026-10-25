package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	ErrNpmNotFound  = errors.New("npm not found on PATH")
	ErrNotInstalled = errors.New("plugin not installed")
)

type Runner func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)

func ExecRunner(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return stdout.Bytes(), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return stdout.Bytes(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
	}
	return stdout.Bytes(), nil
}

type Installer struct {
	Registry *Registry
	Run      Runner
}

func (i *Installer) Install(ctx context.Context, spec string) (Entry, Manifest, error) {
	if info, err := os.Stat(spec); err == nil && info.IsDir() {
		return i.installPath(spec)
	}
	return i.installNpm(ctx, spec)
}

func (i *Installer) installPath(dir string) (Entry, Manifest, error) {
	m, err := LoadManifest(dir)
	if err != nil {
		return Entry{}, Manifest{}, err
	}
	entry := Entry{ID: m.ID, Source: SourcePath, Path: m.Dir, Version: m.Version}
	if err := i.checkConflict(entry); err != nil {
		return Entry{}, Manifest{}, err
	}
	if err := i.Registry.Put(entry); err != nil {
		return Entry{}, Manifest{}, err
	}
	return entry, m, nil
}

func (i *Installer) installNpm(ctx context.Context, spec string) (Entry, Manifest, error) {
	dir := i.Registry.Dir()
	if err := ensureNpmProject(dir); err != nil {
		return Entry{}, Manifest{}, err
	}
	name, err := i.packageName(ctx, dir, spec)
	if err != nil {
		return Entry{}, Manifest{}, err
	}
	if _, err := i.npm(ctx, dir, "install", "--save", spec); err != nil {
		return Entry{}, Manifest{}, err
	}
	m, err := LoadManifest(filepath.Join(dir, "node_modules", filepath.FromSlash(name)))
	if err == nil {
		entry := Entry{ID: m.ID, Source: SourceNpm, Package: name, Path: m.Dir, Version: m.Version}
		if err = i.checkConflict(entry); err == nil {
			if err = i.Registry.Put(entry); err == nil {
				return entry, m, nil
			}
		}
	}
	if _, uerr := i.npm(ctx, dir, "uninstall", "--save", name); uerr != nil {
		return Entry{}, Manifest{}, fmt.Errorf("%w (rollback failed: %v)", err, uerr)
	}
	return Entry{}, Manifest{}, err
}

func (i *Installer) packageName(ctx context.Context, dir, spec string) (string, error) {
	out, err := i.npm(ctx, dir, "pack", "--dry-run", "--json", spec)
	if err != nil {
		return "", err
	}
	var packs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &packs); err != nil || len(packs) == 0 || packs[0].Name == "" {
		return "", fmt.Errorf("cannot determine package name for %q", spec)
	}
	return packs[0].Name, nil
}

func (i *Installer) npm(ctx context.Context, dir string, args ...string) ([]byte, error) {
	out, err := i.Run(ctx, dir, "npm", args...)
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrNpmNotFound
	}
	return out, err
}

func (i *Installer) checkConflict(e Entry) error {
	existing, ok, err := i.Registry.Get(e.ID)
	if err != nil {
		return err
	}
	if ok && existing.Path != e.Path {
		return fmt.Errorf("plugin id %q is already installed from %s; remove it first", e.ID, existing.Path)
	}
	return nil
}

func ensureNpmProject(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}
	pkg := filepath.Join(dir, "package.json")
	if _, err := os.Stat(pkg); err == nil {
		return nil
	}
	return os.WriteFile(pkg, []byte("{\n  \"name\": \"openbee-plugins\",\n  \"private\": true\n}\n"), 0o644)
}

func (i *Installer) Remove(ctx context.Context, id string) (Entry, error) {
	entry, ok, err := i.Registry.Get(id)
	if err != nil {
		return Entry{}, err
	}
	if !ok {
		return Entry{}, fmt.Errorf("%w: %s", ErrNotInstalled, id)
	}
	if entry.Source == SourceNpm && entry.Package != "" {
		if _, err := i.npm(ctx, i.Registry.Dir(), "uninstall", "--save", entry.Package); err != nil {
			return Entry{}, err
		}
	}
	if err := i.Registry.Remove(id); err != nil {
		return Entry{}, err
	}
	return entry, nil
}
