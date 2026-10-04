package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/store"
)

func TestMessageStore_ListBySessionKey_ExcludesMerged(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	s.CreateBatch(ctx, []store.BatchMsg{ //nolint:errcheck
		{ID: "m1", SessionKey: "local:s1", Platform: "local", Content: "hello",
			Status: "received", MessageTime: 1000},
		{ID: "m2", SessionKey: "local:s1", Platform: "local", Content: "world",
			Status: "merged", MergedInto: "m1", MessageTime: 900},
	})

	msgs, err := s.ListBySessionKey(ctx, "local:s1", 0, 50)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "m1", msgs[0].ID)
}

func TestMessageStore_ListBySessionKey_Pagination(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	// Insert 5 messages with timestamps 100..500
	s.CreateBatch(ctx, []store.BatchMsg{ //nolint:errcheck
		{ID: "a", SessionKey: "local:s1", Platform: "local", Content: "1", Status: "received", MessageTime: 100},
		{ID: "b", SessionKey: "local:s1", Platform: "local", Content: "2", Status: "received", MessageTime: 200},
		{ID: "c", SessionKey: "local:s1", Platform: "local", Content: "3", Status: "received", MessageTime: 300},
		{ID: "d", SessionKey: "local:s1", Platform: "local", Content: "4", Status: "received", MessageTime: 400},
		{ID: "e", SessionKey: "local:s1", Platform: "local", Content: "5", Status: "received", MessageTime: 500},
	})

	// limit=3, no before -> latest 3: c,d,e (returned ASC)
	msgs, err := s.ListBySessionKey(ctx, "local:s1", 0, 3)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, "c", msgs[0].ID)
	assert.Equal(t, "e", msgs[2].ID)

	// before=300 (exclusive) -> store fetches limit+1=4 before ts 300, but only 2 exist (a,b)
	msgs2, err := s.ListBySessionKey(ctx, "local:s1", 300, 3)
	require.NoError(t, err)
	require.Len(t, msgs2, 2)
	assert.Equal(t, "a", msgs2[0].ID)
	assert.Equal(t, "b", msgs2[1].ID)
}
