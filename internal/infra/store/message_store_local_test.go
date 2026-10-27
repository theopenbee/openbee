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

func TestInsertProcessedSkipsClaim(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	require.NoError(t, s.InsertProcessed(ctx, store.ProcessedMsg{
		ID: "p1", SessionKey: "local:u1:worker:w1", Platform: "local",
		Content: "from bee", SenderType: store.SenderTypeBee, ReceivedAt: 1000,
	}))

	claimed, err := s.ClaimBatch(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, claimed)

	msgs, err := s.ListBySessionKey(ctx, "local:u1:worker:w1", 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "p1", msgs[0].ID)
	assert.Equal(t, "from bee", msgs[0].Content)
	assert.Equal(t, int64(1000), msgs[0].ReceivedAt)
	assert.Equal(t, store.SenderTypeBee, msgs[0].SenderType)

	var status string
	require.NoError(t, db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id = 'p1'`).Scan(&status))
	assert.Equal(t, store.MsgStatusBeeProcessed, status)
}

func TestInsertProcessedDefaultsSenderAndTime(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	require.NoError(t, s.InsertProcessed(ctx, store.ProcessedMsg{
		ID: "p1", SessionKey: "local:u1", Platform: "local", Content: "x",
	}))

	msgs, err := s.ListBySessionKey(ctx, "local:u1", 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, store.SenderTypeUser, msgs[0].SenderType)
	assert.Positive(t, msgs[0].ReceivedAt)
}

func TestDeleteByID(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	require.NoError(t, s.InsertProcessed(ctx, store.ProcessedMsg{ID: "p1", SessionKey: "local:u1", Platform: "local", Content: "a"}))
	require.NoError(t, s.InsertProcessed(ctx, store.ProcessedMsg{ID: "p2", SessionKey: "local:u1", Platform: "local", Content: "b"}))

	require.NoError(t, s.DeleteByID(ctx, "p1"))
	require.NoError(t, s.DeleteByID(ctx, "missing"))

	msgs, err := s.ListBySessionKey(ctx, "local:u1", 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "p2", msgs[0].ID)
}

func TestCreateBatchDefaultsSenderUser(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	_, err = s.CreateBatch(ctx, []store.BatchMsg{
		{ID: "u", SessionKey: "local:u1", Platform: "local", Content: "1", Status: "received", MessageTime: 100},
		{ID: "b", SessionKey: "local:u1", Platform: "local", Content: "2", Status: "received", MessageTime: 200, SenderType: store.SenderTypeBee},
	})
	require.NoError(t, err)

	msgs, err := s.ListBySessionKey(ctx, "local:u1", 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "u", msgs[0].ID)
	assert.Equal(t, store.SenderTypeUser, msgs[0].SenderType)
	assert.Equal(t, "b", msgs[1].ID)
	assert.Equal(t, store.SenderTypeBee, msgs[1].SenderType)
}
