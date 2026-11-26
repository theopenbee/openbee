package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	MaxAvatarSize  = 256 << 10
	MaxAvatarCount = 2000
)

var avatarTypes = map[string]string{
	".webp": "image/webp",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
}

var avatarNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type Avatar struct {
	Name        string
	Path        string
	ContentType string
}

func LoadAvatars(m Manifest) ([]Avatar, error) {
	dir := filepath.Join(m.Dir, filepath.FromSlash(m.Avatars))
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("read avatars: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("avatars %q is not a folder", m.Avatars)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read avatars: %w", err)
	}
	out := make([]Avatar, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		file := e.Name()
		if strings.HasPrefix(file, ".") {
			continue
		}
		if !e.Type().IsRegular() {
			return nil, fmt.Errorf("avatar %q is not a regular file", file)
		}
		ext := strings.ToLower(filepath.Ext(file))
		contentType, ok := avatarTypes[ext]
		if !ok {
			return nil, fmt.Errorf("avatar %q must be a .webp, .png or .jpg image", file)
		}
		name := strings.TrimSuffix(file, filepath.Ext(file))
		if !avatarNamePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid avatar name %q: must match %s", name, avatarNamePattern.String())
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate avatar name %q", name)
		}
		seen[name] = true
		fi, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("read avatar %q: %w", file, err)
		}
		if fi.Size() > MaxAvatarSize {
			return nil, fmt.Errorf("avatar %q is %d bytes, larger than %d", file, fi.Size(), MaxAvatarSize)
		}
		out = append(out, Avatar{Name: name, Path: filepath.Join(dir, file), ContentType: contentType})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("avatars %q has no images", m.Avatars)
	}
	if len(out) > MaxAvatarCount {
		return nil, fmt.Errorf("avatars %q has %d images, more than %d", m.Avatars, len(out), MaxAvatarCount)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func AvatarRef(pluginID, name string) string {
	return pluginID + "/" + name
}

func ParseAvatarRef(ref string) (pluginID, name string, ok bool) {
	pluginID, name, ok = strings.Cut(ref, "/")
	if !ok || !idPattern.MatchString(pluginID) || !avatarNamePattern.MatchString(name) {
		return "", "", false
	}
	return pluginID, name, true
}
