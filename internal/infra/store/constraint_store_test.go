package store

import (
	"testing"
)

func TestConstraintStore_SaveAndGet(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cs := NewConstraintStore(db)

	err = cs.Save("global", "test_key", "test_value")
	if err != nil {
		t.Fatal(err)
	}

	c, err := cs.Get("global", "test_key")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("expected constraint, got nil")
	}
	if c.Value != "test_value" {
		t.Errorf("expected value 'test_value', got %q", c.Value)
	}

	c, err = cs.Get("global", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Error("expected nil for non-existent key")
	}
}

func TestConstraintStore_Upsert(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cs := NewConstraintStore(db)

	if err := cs.Save("global", "key1", "value1"); err != nil {
		t.Fatal(err)
	}
	if err := cs.Save("global", "key1", "value2"); err != nil {
		t.Fatal(err)
	}

	c, err := cs.Get("global", "key1")
	if err != nil {
		t.Fatal(err)
	}
	if c.Value != "value2" {
		t.Errorf("expected updated value 'value2', got %q", c.Value)
	}
}

func TestConstraintStore_ListByScope(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cs := NewConstraintStore(db)

	if err := cs.Save("global", "key1", "val1"); err != nil {
		t.Fatal(err)
	}
	if err := cs.Save("global", "key2", "val2"); err != nil {
		t.Fatal(err)
	}
	if err := cs.Save("user123", "key3", "val3"); err != nil {
		t.Fatal(err)
	}

	constraints, err := cs.ListByScope("global", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(constraints) != 2 {
		t.Errorf("expected 2 global constraints, got %d", len(constraints))
	}
}

func TestConstraintStore_Delete(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cs := NewConstraintStore(db)
	if err := cs.Save("global", "key1", "val1"); err != nil {
		t.Fatal(err)
	}

	if err := cs.Delete("global", "key1"); err != nil {
		t.Fatal(err)
	}
	c, _ := cs.Get("global", "key1")
	if c != nil {
		t.Error("expected nil after delete")
	}

	if err := cs.Delete("global", "nonexistent"); err != nil {
		t.Errorf("expected no error on delete of non-existent key, got %v", err)
	}
}
