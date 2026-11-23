package rpc_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/aitest"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/rpc"
)

func engineSource(models *aitest.ModelEngine) *aitest.Source {
	src := aitest.NewSource(nil)
	src.Add("claude", map[string]string{"en": "Claude Code", "zh": "Claude Code"}, &aitest.Engine{})
	src.Add("opencode", map[string]string{"zh": "OpenCode 中文"}, models)
	src.Add("plain", nil, &aitest.Engine{})
	return src
}

func decodeList(t *testing.T, result any) []map[string]any {
	t.Helper()
	b, err := json.Marshal(result)
	require.NoError(t, err)
	var out []map[string]any
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

func TestListEngines(t *testing.T) {
	s, _ := newTestServerWithEngines(t, engineSource(&aitest.ModelEngine{}), "opencode")

	res, err := s.CallTool(context.Background(), "list_engines", json.RawMessage(`{}`))
	require.NoError(t, err)

	assert.Equal(t, []map[string]any{
		{"id": "claude", "name": "Claude Code", "default": false, "default_model": "", "bee_model": ""},
		{"id": "opencode", "name": "OpenCode 中文", "default": true, "default_model": "", "bee_model": ""},
		{"id": "plain", "name": "plain", "default": false, "default_model": "", "bee_model": ""},
	}, decodeList(t, res))
}

func TestListEnginesShowsDefaultAndBeeModels(t *testing.T) {
	s, db := newTestServerWithEngines(t, engineSource(&aitest.ModelEngine{}), "opencode")
	configs := store.NewSystemConfigStore(db)
	ctx := context.Background()
	require.NoError(t, configs.Set(ctx, "engine_models_global", `{"opencode":"deepseek/deepseek-flash","claude":"sonnet"}`))
	require.NoError(t, configs.Set(ctx, "engine_models_bee", `{"opencode":" deepseek/deepseek-v4-pro "}`))

	res, err := s.CallTool(ctx, "list_engines", json.RawMessage(`{}`))
	require.NoError(t, err)

	assert.Equal(t, []map[string]any{
		{"id": "claude", "name": "Claude Code", "default": false, "default_model": "sonnet", "bee_model": ""},
		{"id": "opencode", "name": "OpenCode 中文", "default": true, "default_model": "deepseek/deepseek-flash", "bee_model": "deepseek/deepseek-v4-pro"},
		{"id": "plain", "name": "plain", "default": false, "default_model": "", "bee_model": ""},
	}, decodeList(t, res))
}

func TestListModels(t *testing.T) {
	models := &aitest.ModelEngine{ModelsFunc: func(context.Context, []string) ([]ai.ModelInfo, error) {
		return []ai.ModelInfo{{ID: "deepseek/deepseek-flash"}, {ID: "opencode/big-pickle", Name: "Big Pickle"}}, nil
	}}
	s, _ := newTestServerWithEngines(t, engineSource(models), "claude")
	ctx := context.Background()

	res, err := s.CallTool(ctx, "list_models", mustMarshal(t, map[string]any{"engine": "opencode"}))
	require.NoError(t, err)
	assert.Equal(t, []map[string]any{
		{"id": "deepseek/deepseek-flash"},
		{"id": "opencode/big-pickle", "name": "Big Pickle"},
	}, decodeList(t, res))

	w := mustCreateWorker(t, s, map[string]any{"name": "Coder", "engine": "opencode"})
	_, err = s.CallTool(ctx, "list_models", mustMarshal(t, map[string]any{"engine": "opencode", "worker_id": w.ID}))
	require.NoError(t, err)
	assert.Len(t, models.ModelCalls(), 2)

	res, err = s.CallTool(ctx, "list_models", mustMarshal(t, map[string]any{"engine": "claude"}))
	require.NoError(t, err)
	assert.Empty(t, decodeList(t, res))
}

func TestListModelsErrors(t *testing.T) {
	models := &aitest.ModelEngine{ModelsFunc: func(context.Context, []string) ([]ai.ModelInfo, error) {
		return nil, errors.New("opencode models timed out after 20s")
	}}
	s, _ := newTestServerWithEngines(t, engineSource(models), "claude")
	ctx := context.Background()

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"no engine", map[string]any{}, "engine is required"},
		{"unknown engine", map[string]any{"engine": "gemini"}, "gemini"},
		{"unknown worker", map[string]any{"engine": "opencode", "worker_id": "nope"}, "worker nope not found"},
		{"engine failure", map[string]any{"engine": "opencode"}, "list opencode models: opencode models timed out after 20s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.CallTool(ctx, "list_models", mustMarshal(t, tc.args))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestEngineToolsAreBeeOnly(t *testing.T) {
	s, _ := newTestServerWithEngines(t, engineSource(&aitest.ModelEngine{}), "claude")
	ctx := context.WithValue(context.Background(), rpc.CtxWorkerIDKey, "worker-1")
	ctx = context.WithValue(ctx, rpc.CtxScopesKey, []string{"read:workers", "read:tasks", "read:messages"})

	for _, tool := range []string{"list_engines", "list_models"} {
		_, err := s.CallTool(ctx, tool, mustMarshal(t, map[string]any{"engine": "opencode"}))
		require.Error(t, err, tool)
		assert.Contains(t, err.Error(), "not available to workers", tool)
	}
}
