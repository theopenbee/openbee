package store

import (
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func newTokenStatsTestDB(t *testing.T) (*TokenStatsStore, func()) {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	return NewTokenStatsStore(db), func() { db.Close() }
}

func TestTokenStatsStore_IsEmpty_WhenEmpty(t *testing.T) {
	s, cleanup := newTokenStatsTestDB(t)
	defer cleanup()

	empty, err := s.IsEmpty()
	if err != nil {
		t.Fatalf("IsEmpty: %v", err)
	}
	if !empty {
		t.Error("expected empty store to return true")
	}
}

func TestTokenStatsStore_Upsert_InsertsRecord(t *testing.T) {
	s, cleanup := newTokenStatsTestDB(t)
	defer cleanup()

	if err := s.Upsert(model.TokenStats{
		SessionID:           "session-1",
		AgentType:           "claude",
		Model:               "claude-3-5-sonnet",
		InputTokens:         100,
		OutputTokens:        200,
		CacheCreationTokens: 50,
		CacheReadTokens:     30,
		SyncedAt:            time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	empty, _ := s.IsEmpty()
	if empty {
		t.Error("expected non-empty store after insert")
	}
}

func TestTokenStatsStore_Upsert_UpdatesOnConflict(t *testing.T) {
	s, cleanup := newTokenStatsTestDB(t)
	defer cleanup()

	base := model.TokenStats{
		SessionID: "session-1", AgentType: "claude", Model: "claude-3-5-sonnet",
		InputTokens: 100, OutputTokens: 200, SyncedAt: time.Now().UnixMilli(),
	}
	s.Upsert(base)

	updated := model.TokenStats{
		SessionID: "session-1", AgentType: "claude", Model: "claude-3-5-sonnet",
		InputTokens: 500, OutputTokens: 600, SyncedAt: time.Now().UnixMilli(),
	}
	if err := s.Upsert(updated); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	got, err := s.GetBySessionID("session-1")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	if got[0].InputTokens != 500 {
		t.Errorf("InputTokens: want 500, got %d", got[0].InputTokens)
	}
}

func TestTokenStatsStore_Upsert_MultipleModelsPerSession(t *testing.T) {
	s, cleanup := newTokenStatsTestDB(t)
	defer cleanup()

	for _, m := range []string{"claude-3-5-sonnet", "claude-3-opus"} {
		if err := s.Upsert(model.TokenStats{
			SessionID: "session-1", AgentType: "claude", Model: m,
			InputTokens: 100, SyncedAt: time.Now().UnixMilli(),
		}); err != nil {
			t.Fatalf("Upsert %s: %v", m, err)
		}
	}

	got, err := s.GetBySessionID("session-1")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 records (one per model), got %d", len(got))
	}
}
