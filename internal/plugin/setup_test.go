package plugin

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupManifest(t *testing.T, script string) Manifest {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "setup.sh"), []byte(script), 0o755))
	return Manifest{
		ProtocolVersion: 1,
		ID:              "discord",
		Command:         []string{"node", "index.js"},
		Setup:           &Setup{Command: []string{"sh", "./setup.sh"}},
		Config: []ConfigField{
			{Key: "token", Type: FieldSecret, Required: true},
			{Key: "max_media_size", Type: FieldInt},
			{Key: "user_id", Type: FieldString},
		},
		Dir: dir,
	}
}

func TestRunSetup_ReturnsConfigValues(t *testing.T) {
	m := setupManifest(t, `read code
echo "lang=$OPENBEE_LANGUAGE code=$code dir=$(pwd)" 1>&2
printf '{"token": "tok-%s", "max_media_size": 1024, "user_id": null}\n' "$code"
`)
	var stderr bytes.Buffer
	values, err := RunSetup(context.Background(), m, "zh", strings.NewReader("42\n"), &stderr)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"token": "tok-42", "max_media_size": 1024}, values)
	dir, _ := filepath.EvalSymlinks(m.Dir)
	assert.Contains(t, stderr.String(), "lang=zh code=42 dir="+dir)
}

func TestRunSetup_Errors(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		wantErr string
	}{
		{"non-zero exit", "echo nope 1>&2; exit 3", "exit status 3"},
		{"not json", "echo done", "JSON object"},
		{"json array", "echo '[1]'", "JSON object"},
		{"unknown key", `echo '{"token": "t", "extra": 1}'`, "extra"},
		{"wrong type", `echo '{"max_media_size": "big"}'`, "max_media_size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := setupManifest(t, tc.script)
			_, err := RunSetup(context.Background(), m, "en", strings.NewReader(""), &bytes.Buffer{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestRunSetup_NoSetup(t *testing.T) {
	_, err := RunSetup(context.Background(), Manifest{ID: "discord"}, "en", strings.NewReader(""), &bytes.Buffer{})
	require.Error(t, err)
}

func TestResolveCommand(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "bin", "run")
	got, err := ResolveCommand(dir, abs)
	require.NoError(t, err)
	assert.Equal(t, abs, got)

	got, err = ResolveCommand(dir, "./bin/run")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "bin", "run"), got)

	_, err = ResolveCommand(dir, "openbee-no-such-binary")
	require.Error(t, err)
}
