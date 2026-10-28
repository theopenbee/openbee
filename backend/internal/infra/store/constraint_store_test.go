package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstraintStore_SaveAndGet(t *testing.T) {
	db := newTestDB(t)
	cs := NewConstraintStore(db)

	require.NoError(t, cs.Save("global", "test_key", "test_value"))

	c, err := cs.Get("global", "test_key")
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "test_value", c.Value)

	c, err = cs.Get("global", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, c)
}

// TestConstraintStore_UpsertAndList merges the former Upsert and ListByScope
// tests: both share the same setup, differing only in how they read back data.
func TestConstraintStore_UpsertAndList(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, cs *ConstraintStore)
	}{
		{"Upsert", func(t *testing.T, cs *ConstraintStore) {
			require.NoError(t, cs.Save("global", "key1", "value1"))
			require.NoError(t, cs.Save("global", "key1", "value2"))

			c, err := cs.Get("global", "key1")
			require.NoError(t, err)
			assert.Equal(t, "value2", c.Value)
		}},
		{"ListByScope", func(t *testing.T, cs *ConstraintStore) {
			require.NoError(t, cs.Save("global", "key1", "val1"))
			require.NoError(t, cs.Save("global", "key2", "val2"))
			require.NoError(t, cs.Save("user123", "key3", "val3"))

			constraints, err := cs.ListByScope("global", 50)
			require.NoError(t, err)
			assert.Len(t, constraints, 2)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			cs := NewConstraintStore(db)
			tt.run(t, cs)
		})
	}
}

func TestConstraintStore_Delete(t *testing.T) {
	db := newTestDB(t)
	cs := NewConstraintStore(db)
	require.NoError(t, cs.Save("global", "key1", "val1"))

	require.NoError(t, cs.Delete("global", "key1"))
	c, _ := cs.Get("global", "key1")
	assert.Nil(t, c)

	assert.NoError(t, cs.Delete("global", "nonexistent"))
}
