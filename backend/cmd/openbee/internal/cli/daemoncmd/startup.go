package daemoncmd

import (
	"fmt"
	"io"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/skillinstall"
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

func installBuiltinSkills() {
	results, err := skillinstall.InstallSkillsToDefaults()
	if err != nil {
		logger.Warn("install builtin skills", zap.Error(err))
	}
	for _, r := range results {
		if r.Action != skillinstall.ActionUpToDate {
			logger.Info("builtin skill "+string(r.Action), zap.String("skill", r.Name))
		}
	}
}
