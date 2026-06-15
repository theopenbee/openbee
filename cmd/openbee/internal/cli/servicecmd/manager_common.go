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

var runCommand = defaultRunCommand

func defaultRunCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func runOrWrap(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := runCommand(ctx, name, args...)
	if err != nil {
		return out, wrapRunErr(name, args, err, out)
	}
	return out, nil
}

func wrapRunErr(name string, args []string, err error, out []byte) error {
	label := name
	if len(args) > 0 {
		label = name + " " + args[0]
	}
	return fmt.Errorf("%s: %w (%s)", label, err, strings.TrimSpace(string(out)))
}

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
