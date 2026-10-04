package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func setupSystemConfigDB(t *testing.T) *SystemConfigStore {
	t.Helper()
	return NewSystemConfigStore(newTestDB(t))
}

func TestSystemConfigStore_GetMissing(t *testing.T) {
	s := setupSystemConfigDB(t)
	_, found, err := s.Get(context.Background(), "missing_key")
	require.NoError(t, err)
	assert.False(t, found, "expected found=false for missing key")
}

func TestSystemConfigStore_SetAndGet(t *testing.T) {
	s := setupSystemConfigDB(t)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, model.SystemConfigKeyDefaultEngine, "claude"))
	cfg, found, err := s.Get(ctx, model.SystemConfigKeyDefaultEngine)
	require.NoError(t, err)
	require.True(t, found, "expected found=true after Set")
	assert.Equal(t, "claude", cfg.Value)
}

func TestSystemConfigStore_SetOverwrites(t *testing.T) {
	s := setupSystemConfigDB(t)
	ctx := context.Background()

	_ = s.Set(ctx, model.SystemConfigKeyDefaultEngine, "claude")
	_ = s.Set(ctx, model.SystemConfigKeyDefaultEngine, "codex")

	cfg, _, _ := s.Get(ctx, model.SystemConfigKeyDefaultEngine)
	assert.Equal(t, "codex", cfg.Value)
}
