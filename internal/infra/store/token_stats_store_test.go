package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func newTokenStatsTestDB(t *testing.T) *TokenStatsStore {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewTokenStatsStore(db)
}

func TestTokenStatsStore_IsEmpty_WhenEmpty(t *testing.T) {
	s := newTokenStatsTestDB(t)

	empty, err := s.IsEmpty()
	require.NoError(t, err)
	assert.True(t, empty, "expected empty store to return true")
}

func TestTokenStatsStore_Upsert_InsertsRecord(t *testing.T) {
	s := newTokenStatsTestDB(t)

	require.NoError(t, s.Upsert(model.TokenStats{
		SessionID:           "session-1",
		AgentType:           "claude",
		Model:               "claude-3-5-sonnet",
		InputTokens:         100,
		OutputTokens:        200,
		CacheCreationTokens: 50,
		CacheReadTokens:     30,
		SyncedAt:            time.Now().UnixMilli(),
	}))

	empty, _ := s.IsEmpty()
	assert.False(t, empty, "expected non-empty store after insert")
}

func TestTokenStatsStore_Upsert_UpdatesOnConflict(t *testing.T) {
	s := newTokenStatsTestDB(t)

	base := model.TokenStats{
		SessionID: "session-1", AgentType: "claude", Model: "claude-3-5-sonnet",
		InputTokens: 100, OutputTokens: 200, SyncedAt: time.Now().UnixMilli(),
	}
	s.Upsert(base)

	updated := model.TokenStats{
		SessionID: "session-1", AgentType: "claude", Model: "claude-3-5-sonnet",
		InputTokens: 500, OutputTokens: 600, SyncedAt: time.Now().UnixMilli(),
	}
	require.NoError(t, s.Upsert(updated))

	got, err := s.GetBySessionID("session-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.EqualValues(t, 500, got[0].InputTokens)
}

func TestTokenStatsStore_Upsert_TotalTokensStored(t *testing.T) {
	s := newTokenStatsTestDB(t)

	require.NoError(t, s.Upsert(model.TokenStats{
		SessionID:           "session-total",
		AgentType:           "claude",
		Model:               "claude-3-5-sonnet",
		InputTokens:         100,
		OutputTokens:        200,
		CacheCreationTokens: 50,
		CacheReadTokens:     30,
		TotalTokens:         380,
		SyncedAt:            time.Now().UnixMilli(),
	}))

	got, err := s.GetBySessionID("session-total")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.EqualValues(t, 380, got[0].TotalTokens)

	// verify TotalTokens is updated on conflict
	require.NoError(t, s.Upsert(model.TokenStats{
		SessionID:           "session-total",
		AgentType:           "claude",
		Model:               "claude-3-5-sonnet",
		InputTokens:         200,
		OutputTokens:        300,
		CacheCreationTokens: 10,
		CacheReadTokens:     5,
		TotalTokens:         515,
		SyncedAt:            time.Now().UnixMilli(),
	}))
	got, err = s.GetBySessionID("session-total")
	require.NoError(t, err, "GetBySessionID after update")
	assert.EqualValues(t, 515, got[0].TotalTokens, "after update")
}

func TestTokenStatsStore_Upsert_MultipleModelsPerSession(t *testing.T) {
	s := newTokenStatsTestDB(t)

	for _, m := range []string{"claude-3-5-sonnet", "claude-3-opus"} {
		require.NoError(t, s.Upsert(model.TokenStats{
			SessionID: "session-1", AgentType: "claude", Model: m,
			InputTokens: 100, SyncedAt: time.Now().UnixMilli(),
		}), "Upsert %s", m)
	}

	got, err := s.GetBySessionID("session-1")
	require.NoError(t, err)
	assert.Len(t, got, 2, "expected one record per model")
}

func TestTokenStatsStore_GetBySessionIDs_ReturnsMatchingRows(t *testing.T) {
	s := newTokenStatsTestDB(t)

	now := time.Now().UnixMilli()
	for _, stat := range []model.TokenStats{
		{SessionID: "session-1", AgentType: "claude", Model: "claude-sonnet-4-6", InputTokens: 100, OutputTokens: 200, TotalTokens: 300, SyncedAt: now},
		{SessionID: "session-1", AgentType: "claude", Model: "claude-opus-4-7", InputTokens: 50, OutputTokens: 100, TotalTokens: 150, SyncedAt: now},
		{SessionID: "session-2", AgentType: "claude", Model: "claude-sonnet-4-6", InputTokens: 10, OutputTokens: 20, TotalTokens: 30, SyncedAt: now},
	} {
		require.NoError(t, s.Upsert(stat))
	}

	rows, err := s.GetBySessionIDs([]string{"session-1", "session-99"})
	require.NoError(t, err)
	assert.Len(t, rows, 2)
	for _, r := range rows {
		assert.Equal(t, "session-1", r.SessionID)
	}
}

func TestTokenStatsStore_GetBySessionIDs_NilSlice(t *testing.T) {
	s := newTokenStatsTestDB(t)

	rows, err := s.GetBySessionIDs(nil)
	require.NoError(t, err)
	assert.Nil(t, rows)
}
