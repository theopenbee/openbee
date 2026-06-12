package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
)

// runCommand is the package-wide command runner; tests may override it.
var runCommand = defaultRunCommand

func defaultRunCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// runOrWrap invokes runCommand and wraps any error with the given label and
// the trimmed combined output, matching the convention used by every platform
// manager.
func runOrWrap(ctx context.Context, label, name string, args ...string) ([]byte, error) {
	out, err := runCommand(ctx, name, args...)
	if err != nil {
		return out, wrapRunErr(label, err, out)
	}
	return out, nil
}

// wrapRunErr is the standard "label: %w (trimmed output)" wrap used after
// callers have already captured runCommand's output for separate inspection.
func wrapRunErr(label string, err error, out []byte) error {
	return fmt.Errorf("%s: %w (%s)", label, err, strings.TrimSpace(string(out)))
}

// parseTemplate parses a template embedded at path. It panics on parse errors,
// which can only happen if the embedded template source is malformed — a
// compile-time concern, not a runtime one.
func parseTemplate(fs embed.FS, path, name string) *template.Template {
	b, err := fs.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return template.Must(template.New(name).Parse(string(b)))
}

func executeTemplate(t *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
