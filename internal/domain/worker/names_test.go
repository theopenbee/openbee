package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPickRandomName_AllUnused(t *testing.T) {
	pool := []string{"Alice", "Bob", "Carol"}
	used := map[string]struct{}{}
	name, ok := PickRandomName(pool, used)
	require.True(t, ok, "expected ok=true, got false")
	assert.Contains(t, pool, name, "returned name %q not in pool", name)
}

func TestPickRandomName_SomeUsed(t *testing.T) {
	pool := []string{"Alice", "Bob", "Carol"}
	used := map[string]struct{}{"alice": {}, "bob": {}}
	name, ok := PickRandomName(pool, used)
	require.True(t, ok, "expected ok=true")
	assert.Equal(t, "Carol", name)
}

func TestPickRandomName_AllUsed(t *testing.T) {
	pool := []string{"Alice", "Bob"}
	used := map[string]struct{}{"alice": {}, "bob": {}}
	_, ok := PickRandomName(pool, used)
	require.False(t, ok, "expected ok=false when all names used")
}

func TestPickRandomName_CaseInsensitive(t *testing.T) {
	pool := []string{"Alice"}
	used := map[string]struct{}{"ALICE": {}}
	_, ok := PickRandomName(pool, used)
	require.False(t, ok, "expected ok=false: pool name should be filtered case-insensitively")
}

func TestNamePool_ZH(t *testing.T) {
	pool := NamePool("zh")
	assert.Len(t, pool, 200)
}

func TestNamePool_EN(t *testing.T) {
	pool := NamePool("en")
	assert.Len(t, pool, 200)
}

func TestNamePool_DefaultsToEN(t *testing.T) {
	pool := NamePool("fr")
	assert.Len(t, pool, 200)
}
