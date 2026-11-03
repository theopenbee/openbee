package plugincmd

import (
	"context"
	"os"

	"github.com/theopenbee/openbee/internal/plugin"
)

func runLocalSetup(path string) (map[string]any, error) {
	m, err := plugin.LoadManifest(path)
	if err != nil {
		return nil, err
	}
	return plugin.RunSetup(context.Background(), plugin.NewRuntimeResolver(), m, cliLang, os.Stdin, os.Stderr)
}
