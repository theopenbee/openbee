package plugincmd

import (
	"context"
	"os"

	"github.com/theopenbee/openbee/internal/plugin"
)

func runLocalSetup(path, lang string) (map[string]any, error) {
	m, err := plugin.LoadManifest(path)
	if err != nil {
		return nil, err
	}
	return plugin.RunSetup(context.Background(), plugin.NewRuntimeResolver(), m, lang, os.Stdin, os.Stderr)
}
