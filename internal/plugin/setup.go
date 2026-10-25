package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func ResolveCommand(dir, name string) (string, error) {
	switch {
	case filepath.IsAbs(name):
		return name, nil
	case strings.HasPrefix(name, ".") || strings.ContainsRune(name, '/') || strings.ContainsRune(name, filepath.Separator):
		return filepath.Join(dir, name), nil
	}
	return exec.LookPath(name)
}

func RunSetup(ctx context.Context, m Manifest, lang string, stdin io.Reader, stderr io.Writer) (map[string]any, error) {
	if m.Setup == nil || len(m.Setup.Command) == 0 {
		return nil, fmt.Errorf("plugin %s has no setup command", m.ID)
	}
	path, err := ResolveCommand(m.Dir, m.Setup.Command[0])
	if err != nil {
		return nil, fmt.Errorf("resolve setup command: %w", err)
	}
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, path, m.Setup.Command[1:]...)
	cmd.Dir = m.Dir
	cmd.Env = append(os.Environ(), "OPENBEE_LANGUAGE="+lang)
	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("setup command: %w", err)
	}
	return parseSetupOutput(m, stdout.Bytes())
}

func parseSetupOutput(m Manifest, data []byte) (map[string]any, error) {
	var raw map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &raw); err != nil || raw == nil {
		return nil, errors.New("setup output must be a JSON object")
	}
	fields := make(map[string]ConfigField, len(m.Config))
	for _, f := range m.Config {
		fields[f.Key] = f
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		f, ok := fields[k]
		if !ok {
			return nil, fmt.Errorf("setup returned unknown config key %q", k)
		}
		if v == nil {
			continue
		}
		coerced, err := coerce(f, v)
		if err != nil {
			return nil, err
		}
		out[k] = coerced
	}
	return out, nil
}
