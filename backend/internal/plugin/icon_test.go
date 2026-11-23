package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testIcon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M0 0h24v24H0z"/></svg>`

func writeIconPlugin(t *testing.T, icon string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(engineManifest), &raw))
	raw["icon"] = icon
	b, err := json.Marshal(raw)
	require.NoError(t, err)
	writePlugin(t, dir, string(b))
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}

func TestLoadManifest_Icon(t *testing.T) {
	dir := writeIconPlugin(t, "assets/icon.svg", map[string]string{"assets/icon.svg": testIcon})

	m, err := LoadManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, "assets/icon.svg", m.Icon)
	assert.Equal(t, testIcon, m.IconSVG)
}

func TestLoadManifest_NoIcon(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, engineManifest)

	m, err := LoadManifest(dir)
	require.NoError(t, err)
	assert.Empty(t, m.Icon)
	assert.Empty(t, m.IconSVG)
}

func TestLoadManifest_IconRejected(t *testing.T) {
	cases := []struct {
		name  string
		icon  string
		files map[string]string
		want  string
	}{
		{"missing file", "icon.svg", nil, "read icon"},
		{"absolute path", "/etc/icon.svg", nil, "relative path"},
		{"outside the plugin", "../icon.svg", nil, "relative path"},
		{"not svg extension", "icon.png", map[string]string{"icon.png": testIcon}, ".svg"},
		{"directory", "icon.svg", map[string]string{"icon.svg/x": ""}, "regular file"},
		{"too large", "icon.svg", map[string]string{"icon.svg": `<svg xmlns="http://www.w3.org/2000/svg">` + strings.Repeat(" ", MaxIconSize) + `</svg>`}, "larger than"},
		{"not xml", "icon.svg", map[string]string{"icon.svg": `<svg xmlns="http://www.w3.org/2000/svg"><path></svg>`}, "not valid XML"},
		{"empty", "icon.svg", map[string]string{"icon.svg": ""}, "root element"},
		{"no svg namespace", "icon.svg", map[string]string{"icon.svg": `<svg viewBox="0 0 1 1"/>`}, "xmlns"},
		{"html root", "icon.svg", map[string]string{"icon.svg": `<html><svg xmlns="http://www.w3.org/2000/svg"/></html>`}, "root element"},
		{"two roots", "icon.svg", map[string]string{"icon.svg": `<svg xmlns="http://www.w3.org/2000/svg"/><svg xmlns="http://www.w3.org/2000/svg"/>`}, "root element"},
		{"script", "icon.svg", map[string]string{"icon.svg": `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`}, "<script>"},
		{"foreign object", "icon.svg", map[string]string{"icon.svg": `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><div/></foreignObject></svg>`}, "<foreignObject>"},
		{"event attribute", "icon.svg", map[string]string{"icon.svg": `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`}, "onload"},
		{"external href", "icon.svg", map[string]string{"icon.svg": `<svg xmlns="http://www.w3.org/2000/svg"><image href="https://example.com/a.png"/></svg>`}, "links"},
		{"external xlink href", "icon.svg", map[string]string{"icon.svg": `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="other.svg#a"/></svg>`}, "links"},
		{"doctype", "icon.svg", map[string]string{"icon.svg": `<!DOCTYPE svg [<!ENTITY a "x">]><svg xmlns="http://www.w3.org/2000/svg"/>`}, "DOCTYPE"},
		{"stylesheet instruction", "icon.svg", map[string]string{"icon.svg": `<?xml-stylesheet href="a.css"?><svg xmlns="http://www.w3.org/2000/svg"/>`}, "processing instruction"},
		{"text outside", "icon.svg", map[string]string{"icon.svg": `hello<svg xmlns="http://www.w3.org/2000/svg"/>`}, "text outside"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeIconPlugin(t, tc.icon, tc.files)
			_, err := LoadManifest(dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestLoadManifest_IconSymlinkRejected(t *testing.T) {
	target := filepath.Join(t.TempDir(), "real.svg")
	require.NoError(t, os.WriteFile(target, []byte(testIcon), 0o644))
	dir := writeIconPlugin(t, "icon.svg", nil)
	if err := os.Symlink(target, filepath.Join(dir, "icon.svg")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := LoadManifest(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "regular file")
}

func TestValidateIcon_Accepts(t *testing.T) {
	for name, svg := range map[string]string{
		"plain":           testIcon,
		"xml declaration": `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + testIcon + "\n",
		"comment":         `<!-- mark -->` + testIcon,
		"fragment href":   `<svg xmlns="http://www.w3.org/2000/svg"><defs><path id="a" d="M0 0h1v1z"/></defs><use href="#a"/></svg>`,
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, validateIcon([]byte(svg)))
		})
	}
}
