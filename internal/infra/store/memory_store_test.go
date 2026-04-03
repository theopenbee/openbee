package store

import (
	"testing"
)

func TestMemoryStore_SaveAndGet(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ms := NewMemoryStore(db)

	// Save a memory
	err = ms.Save("global", "test_key", "test_value")
	if err != nil {
		t.Fatal(err)
	}

	// Get single memory by key
	mem, err := ms.Get("global", "test_key")
	if err != nil {
		t.Fatal(err)
	}
	if mem == nil {
		t.Fatal("expected memory, got nil")
	}
	if mem.Value != "test_value" {
		t.Errorf("expected value 'test_value', got %q", mem.Value)
	}

	// Get non-existent key
	mem, err = ms.Get("global", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if mem != nil {
		t.Error("expected nil for non-existent key")
	}
}

func TestMemoryStore_Upsert(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ms := NewMemoryStore(db)

	// Save initial
	if err := ms.Save("global", "key1", "value1"); err != nil {
		t.Fatal(err)
	}
	// Upsert same key
	if err := ms.Save("global", "key1", "value2"); err != nil {
		t.Fatal(err)
	}

	mem, err := ms.Get("global", "key1")
	if err != nil {
		t.Fatal(err)
	}
	if mem.Value != "value2" {
		t.Errorf("expected updated value 'value2', got %q", mem.Value)
	}
}

func TestMemoryStore_ListByScope(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ms := NewMemoryStore(db)

	ms.Save("global", "key1", "val1")
	ms.Save("global", "key2", "val2")
	ms.Save("user123", "key3", "val3")

	memories, err := ms.ListByScope("global", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 2 {
		t.Errorf("expected 2 global memories, got %d", len(memories))
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ms := NewMemoryStore(db)
	ms.Save("global", "key1", "val1")

	if err := ms.Delete("global", "key1"); err != nil {
		t.Fatal(err)
	}
	mem, _ := ms.Get("global", "key1")
	if mem != nil {
		t.Error("expected nil after delete")
	}

	// Delete non-existent is no-op
	if err := ms.Delete("global", "nonexistent"); err != nil {
		t.Errorf("expected no error on delete of non-existent key, got %v", err)
	}
}
