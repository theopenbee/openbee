package daemoncmd

import (
	"fmt"
	"io"

	"github.com/theopenbee/openbee/internal/infra/config"
)

func prepareConfig(out io.Writer, path string) error {
	created, err := config.Prepare(path)
	if err != nil {
		return fmt.Errorf("prepare config: %w", err)
	}
	if created {
		fmt.Fprintf(out, "Created config file at %s\n", path)
	}
	return nil
}
