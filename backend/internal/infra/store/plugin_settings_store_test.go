package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func TestPluginSettingsStore_GetMissing(t *testing.T) {
	s := NewPluginSettingsStore(newTestDB(t))
	_, found, err := s.Get(context.Background(), "telegram")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestPluginSettingsStore_UpsertGetListDelete(t *testing.T) {
	s := NewPluginSettingsStore(newTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, model.PluginSettings{ID: "telegram", Enabled: true, BotName: "bee", Config: "c1"}))
	require.NoError(t, s.Upsert(ctx, model.PluginSettings{ID: "discord", Config: "c2"}))

	got, found, err := s.Get(ctx, "telegram")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "telegram", got.ID)
	assert.True(t, got.Enabled)
	assert.Equal(t, "bee", got.BotName)
	assert.Equal(t, "c1", got.Config)
	assert.NotZero(t, got.UpdatedAt)

	require.NoError(t, s.Upsert(ctx, model.PluginSettings{ID: "telegram", Enabled: false, Config: "c3"}))
	got, _, err = s.Get(ctx, "telegram")
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Equal(t, "", got.BotName)
	assert.Equal(t, "c3", got.Config)

	list, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "discord", list[0].ID)
	assert.Equal(t, "telegram", list[1].ID)

	require.NoError(t, s.Delete(ctx, "telegram"))
	_, found, err = s.Get(ctx, "telegram")
	require.NoError(t, err)
	assert.False(t, found)
	require.NoError(t, s.Delete(ctx, "telegram"))
}
