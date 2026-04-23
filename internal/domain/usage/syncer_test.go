package usage

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// stubStore implements usageSyncStore for testing.
type stubStore struct {
	unsynced []model.UnsyncedExecution
	inserted []*model.UsageRecord
}

func (s *stubStore) ListUnsynced(limit int) ([]model.UnsyncedExecution, error) {
	if len(s.unsynced) > limit {
		return s.unsynced[:limit], nil
	}
	return s.unsynced, nil
}

func (s *stubStore) Insert(record *model.UsageRecord) error {
	s.inserted = append(s.inserted, record)
	return nil
}

func TestUsageSyncer_SyncBatch_InsertsRecord(t *testing.T) {
	dir := t.TempDir()
	logPath := dir + "/exec.log"
	logContent := `{"type":"assistant","message":{"model":"claude-sonnet-4-6","content":[]}}
{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.10,"usage":{"input_tokens":5,"output_tokens":10,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
	require.NoError(t, os.WriteFile(logPath, []byte(logContent), 0o644))

	stub := &stubStore{
		unsynced: []model.UnsyncedExecution{
			{ID: "exec-1", LogPath: logPath},
		},
	}

	syncer := NewUsageSyncer(stub, 60*time.Second, 50)
	more := syncer.syncBatch()

	assert.False(t, more, "batch < limit so more should be false")
	require.Len(t, stub.inserted, 1)
	assert.Equal(t, "exec-1", stub.inserted[0].ExecutionID)
	assert.Equal(t, "claude-sonnet-4-6", stub.inserted[0].Model)
	assert.Equal(t, int64(15), stub.inserted[0].TotalTokens)
	assert.InDelta(t, 0.10, stub.inserted[0].CostUSD, 0.001)
}

func TestUsageSyncer_SyncBatch_MissingLog(t *testing.T) {
	stub := &stubStore{
		unsynced: []model.UnsyncedExecution{
			{ID: "exec-2", LogPath: "/no/such/file.log"},
		},
	}
	syncer := NewUsageSyncer(stub, 60*time.Second, 50)
	_ = syncer.syncBatch()

	// Missing log → zero-value record inserted to prevent retry
	require.Len(t, stub.inserted, 1)
	assert.Equal(t, "exec-2", stub.inserted[0].ExecutionID)
	assert.Equal(t, int64(0), stub.inserted[0].TotalTokens)
}

func TestUsageSyncer_SyncBatch_MoreWhenFull(t *testing.T) {
	stub := &stubStore{
		unsynced: []model.UnsyncedExecution{
			{ID: "e1", LogPath: "/no/such/1.log"},
			{ID: "e2", LogPath: "/no/such/2.log"},
		},
	}
	syncer := NewUsageSyncer(stub, 60*time.Second, 2) // batchSize == len(unsynced)
	more := syncer.syncBatch()
	assert.True(t, more, "full batch should signal more")
}
