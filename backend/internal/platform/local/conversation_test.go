package local_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform/local"
)

func TestConversationSessionKeyRoundTrip(t *testing.T) {
	tests := []struct {
		conversationID string
		wantKey        string
	}{
		{local.BeeConversationID, "local:u1"},
		{"worker:w1", "local:u1:worker:w1"},
	}
	for _, tc := range tests {
		t.Run(tc.conversationID, func(t *testing.T) {
			key, err := local.ConversationSessionKey("u1", tc.conversationID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantKey, key)

			uid, cid, ok := local.ParseSessionKey(key)
			assert.True(t, ok)
			assert.Equal(t, "u1", uid)
			assert.Equal(t, tc.conversationID, cid)
		})
	}
}

func TestParseSessionKeyRejects(t *testing.T) {
	for _, key := range []string{
		"telegram:1:2",
		"local:",
		"local:u1:group:g",
		"local:u1:worker:",
		"local:u1:worker:a:b",
	} {
		t.Run(key, func(t *testing.T) {
			_, _, ok := local.ParseSessionKey(key)
			assert.False(t, ok)
		})
	}
}

func TestConversationSessionKeyUnknown(t *testing.T) {
	for _, conversationID := range []string{"worker:", "group:x", ""} {
		t.Run(conversationID, func(t *testing.T) {
			_, err := local.ConversationSessionKey("u1", conversationID)
			assert.Error(t, err)
		})
	}
}

func TestWorkerConversationID(t *testing.T) {
	assert.Equal(t, "worker:w1", local.WorkerConversationID("w1"))

	id, ok := local.WorkerIDFromConversation("worker:w1")
	assert.True(t, ok)
	assert.Equal(t, "w1", id)

	for _, cid := range []string{"bee", "worker:", "worker:a:b", ""} {
		_, ok := local.WorkerIDFromConversation(cid)
		assert.False(t, ok, cid)
	}
}

func TestWorkerSessionKey(t *testing.T) {
	assert.Equal(t, "local:u1:worker:w1", local.WorkerSessionKey("u1", "w1"))
}

func TestSessionKeyKinds(t *testing.T) {
	assert.True(t, local.IsBeeSessionKey("local:u1"))
	assert.False(t, local.IsBeeSessionKey("local:u1:worker:w1"))
	assert.False(t, local.IsBeeSessionKey("telegram:1:2"))
	assert.False(t, local.IsBeeSessionKey("local:"))

	id, ok := local.WorkerIDFromSessionKey("local:u1:worker:w1")
	assert.True(t, ok)
	assert.Equal(t, "w1", id)

	for _, key := range []string{"local:u1", "telegram:1:2", "local:u1:worker:"} {
		_, ok := local.WorkerIDFromSessionKey(key)
		assert.False(t, ok, key)
	}
}

func TestWorkerScopedSessionKey(t *testing.T) {
	assert.Equal(t, "local:u1:worker:w1", local.WorkerScopedSessionKey("local:u1", "w1"))
	assert.Equal(t, "local:u1:worker:w2", local.WorkerScopedSessionKey("local:u1:worker:w2", "w1"))
	assert.Equal(t, "telegram:1:2", local.WorkerScopedSessionKey("telegram:1:2", "w1"))
}

func TestMediaDirUsesUserForWorkerKeys(t *testing.T) {
	root := t.TempDir()

	dir, err := local.MediaDir(root, "local:u1:worker:w1")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "u1"), dir)

	_, err = local.MediaDir(root, "local:../x:worker:w1")
	assert.Error(t, err)
	_, err = local.MediaDir(root, "local:u1:worker:")
	assert.Error(t, err)
}
