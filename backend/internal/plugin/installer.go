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
	"regexp"
	"strings"
)

var ErrNotInstalled = errors.New("plugin not installed")

type Runner func(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error)

func ExecRunner(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
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
	Runtime  *RuntimeResolver
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
	entries, err := i.Registry.List()
	if err != nil {
		return Entry{}, Manifest{}, err
	}
	pm, err := i.packageManager(ctx)
	if err != nil {
		return Entry{}, Manifest{}, err
	}
	name, err := pm.add(ctx, dir, spec)
	if err != nil {
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
	var rerr error
	if previous, ok := registeredPackage(entries, name); ok && previous.Version != "" {
		rerr = pm.install(ctx, dir, name+"@"+previous.Version)
	} else {
		rerr = pm.remove(ctx, dir, name)
	}
	if rerr != nil {
		return Entry{}, Manifest{}, fmt.Errorf("%w (rollback failed: %v)", err, rerr)
	}
	return Entry{}, Manifest{}, err
}

func registeredPackage(entries []Entry, name string) (Entry, bool) {
	for _, e := range entries {
		if e.Source == SourceNpm && e.Package == name {
			return e, true
		}
	}
	return Entry{}, false
}

func (i *Installer) packageManager(ctx context.Context) (packageManager, error) {
	rt, err := i.Runtime.ResolvePackageManager(ctx)
	if err != nil {
		return packageManager{}, err
	}
	return packageManager{rt: rt, run: i.Run}, nil
}

type packageManager struct {
	rt  Runtime
	run Runner
}

func (p packageManager) invoke(ctx context.Context, dir string, args ...string) ([]byte, error) {
	exe := p.rt.Npm
	if p.rt.Kind == RuntimeBun {
		exe = p.rt.Exe
	}
	return p.run(ctx, dir, p.rt.Env, exe, args...)
}

func (p packageManager) install(ctx context.Context, dir, spec string) error {
	args := []string{"install", "--save", spec}
	if p.rt.Kind == RuntimeBun {
		args = []string{"add", spec}
	}
	_, err := p.invoke(ctx, dir, args...)
	return err
}

func (p packageManager) remove(ctx context.Context, dir, name string) error {
	args := []string{"uninstall", "--save", name}
	if p.rt.Kind == RuntimeBun {
		args = []string{"remove", name}
	}
	_, err := p.invoke(ctx, dir, args...)
	return err
}

func (p packageManager) add(ctx context.Context, dir, spec string) (string, error) {
	if p.rt.Kind != RuntimeBun {
		name, err := p.npmPackageName(ctx, dir, spec)
		if err != nil {
			return "", err
		}
		return name, p.install(ctx, dir, spec)
	}
	before, err := dependencies(dir)
	if err != nil {
		return "", err
	}
	if err := p.install(ctx, dir, spec); err != nil {
		return "", err
	}
	after, err := dependencies(dir)
	if err != nil {
		return "", err
	}
	return addedPackage(before, after, spec)
}

func (p packageManager) npmPackageName(ctx context.Context, dir, spec string) (string, error) {
	out, err := p.invoke(ctx, dir, "pack", "--dry-run", "--json", spec)
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

var registrySpec = regexp.MustCompile(`^((?:@[a-z0-9][\w.-]*/)?[a-z0-9][\w.-]*)(?:@.*)?$`)

func addedPackage(before, after map[string]string, spec string) (string, error) {
	var changed []string
	for name, v := range after {
		if before[name] != v {
			changed = append(changed, name)
		}
	}
	if len(changed) == 1 {
		return changed[0], nil
	}
	for name, v := range after {
		if v == spec {
			return name, nil
		}
	}
	if m := registrySpec.FindStringSubmatch(spec); m != nil {
		if _, ok := after[m[1]]; ok {
			return m[1], nil
		}
	}
	return "", fmt.Errorf("cannot determine package name for %q", spec)
}

func dependencies(dir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parse package.json: %w", err)
	}
	return pkg.Dependencies, nil
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
		pm, err := i.packageManager(ctx)
		if err != nil {
			return Entry{}, err
		}
		if err := pm.remove(ctx, i.Registry.Dir(), entry.Package); err != nil {
			return Entry{}, err
		}
	}
	if err := i.Registry.Remove(id); err != nil {
		return Entry{}, err
	}
	return entry, nil
}
