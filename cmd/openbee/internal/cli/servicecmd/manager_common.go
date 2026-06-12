package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"os/exec"
	"text/template"
)

// runCommand is the package-wide command runner; tests may override it.
var runCommand = defaultRunCommand

func defaultRunCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
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
