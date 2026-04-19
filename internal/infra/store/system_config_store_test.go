package store

import (
	"context"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func setupSystemConfigDB(t *testing.T) *SystemConfigStore {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewSystemConfigStore(db)
}

func TestSystemConfigStore_GetMissing(t *testing.T) {
	s := setupSystemConfigDB(t)
	_, found, err := s.Get(context.Background(), "missing_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Error("expected found=false for missing key")
	}
}

func TestSystemConfigStore_SetAndGet(t *testing.T) {
	s := setupSystemConfigDB(t)
	ctx := context.Background()

	if err := s.Set(ctx, model.SystemConfigKeyDefaultEngine, "claude"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	cfg, found, err := s.Get(ctx, model.SystemConfigKeyDefaultEngine)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected found=true after Set")
	}
	if cfg.Value != "claude" {
		t.Errorf("expected claude, got %s", cfg.Value)
	}
}

func TestSystemConfigStore_SetOverwrites(t *testing.T) {
	s := setupSystemConfigDB(t)
	ctx := context.Background()

	_ = s.Set(ctx, model.SystemConfigKeyDefaultEngine, "claude")
	_ = s.Set(ctx, model.SystemConfigKeyDefaultEngine, "codex")

	cfg, _, _ := s.Get(ctx, model.SystemConfigKeyDefaultEngine)
	if cfg.Value != "codex" {
		t.Errorf("expected codex after overwrite, got %s", cfg.Value)
	}
}
