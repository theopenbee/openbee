package app

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/skillinstall"
)

func Boot(cfg config.Config) (*App, error) {
	if err := i18n.Load(cfg.Language); err != nil {
		return nil, fmt.Errorf("load locale: %w", err)
	}
	installBuiltinSkills()
	a, err := BuildApp(cfg)
	if err != nil {
		return nil, fmt.Errorf("build app: %w", err)
	}
	return a, nil
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
