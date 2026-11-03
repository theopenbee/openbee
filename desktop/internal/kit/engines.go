package kit

import (
	"slices"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/locales"
)

func EngineOptions(engines []api.EngineInfo, stored string) []SelectOption {
	options := make([]SelectOption, len(engines), len(engines)+1)
	for i, engine := range engines {
		options[i] = SelectOption{Value: engine.ID, Text: l10n.EngineName(engine)}
	}
	if stored != "" && !slices.ContainsFunc(engines, func(e api.EngineInfo) bool { return e.ID == stored }) {
		options = append(options, SelectOption{Value: stored, Text: locales.WorkersFormEngineUnavailable(stored), Disabled: true})
	}
	return options
}
