package bee

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/theopenbee/openbee/internal/ai/aitest"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type mapSystemConfigs map[string]string

func (m mapSystemConfigs) Get(_ context.Context, key string) (model.SystemConfig, bool, error) {
	value, ok := m[key]
	return model.SystemConfig{Key: key, Value: value}, ok, nil
}

func TestBeeProcessResolveModel(t *testing.T) {
	configs := mapSystemConfigs{
		model.SystemConfigKeyEngineModelsGlobal: `{"pi":"deepseek/deepseek-flash","claude":"sonnet"}`,
		model.SystemConfigKeyEngineModelsBee:    `{"claude":"opus"}`,
	}
	cases := []struct{ engine, want string }{
		{"claude", "opus"},
		{"pi", "deepseek/deepseek-flash"},
		{"codex", ""},
	}
	for _, tc := range cases {
		p := &BeeProcess{sysConfigStore: configs, engineCfg: enginecfg.NewStore(tc.engine, "", aitest.IDs("claude", "pi", "codex"))}
		assert.Equal(t, tc.want, p.resolveModel(context.Background()), tc.engine)
	}

	bare := &BeeProcess{engineCfg: enginecfg.NewStore("claude", "", aitest.IDs("claude"))}
	assert.Equal(t, "", bare.resolveModel(context.Background()))
}
