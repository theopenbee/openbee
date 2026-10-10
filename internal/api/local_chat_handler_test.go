package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
)

func TestEncodeMediaPaths(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		text  string
		want  string
	}{
		{
			name:  "no files",
			paths: nil,
			text:  "hello",
			want:  "hello",
		},
		{
			name:  "single file",
			paths: []string{"photo.png"},
			text:  "see this",
			want:  "\x00[file] photo.png\nsee this",
		},
		{
			name:  "multiple files",
			paths: []string{"a.png", "b.pdf"},
			text:  "two files",
			want:  "\x00[file] a.png\n\x00[file] b.pdf\ntwo files",
		},
		{
			name:  "files with empty text",
			paths: []string{"x.jpg"},
			text:  " ",
			want:  "\x00[file] x.jpg\n ",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeMediaPaths(tc.paths, tc.text)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeMediaPaths(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantPaths []string
		wantText  string
	}{
		{
			name:      "no files",
			content:   "hello",
			wantPaths: nil,
			wantText:  "hello",
		},
		{
			name:      "single file (new format)",
			content:   "\x00[file] photo.png\nhello",
			wantPaths: []string{"photo.png"},
			wantText:  "hello",
		},
		{
			name:      "multiple files (new format)",
			content:   "\x00[file] a.png\n\x00[file] b.pdf\ntwo files",
			wantPaths: []string{"a.png", "b.pdf"},
			wantText:  "two files",
		},
		{
			name:      "single file (legacy format)",
			content:   "[file] photo.png\nhello",
			wantPaths: []string{"photo.png"},
			wantText:  "hello",
		},
		{
			name:      "multiple files (legacy format)",
			content:   "[file] a.png\n[file] b.pdf\ntwo files",
			wantPaths: []string{"a.png", "b.pdf"},
			wantText:  "two files",
		},
		{
			name:      "user text starting with [file] is not decoded",
			content:   "[file] this is just text without NUL prefix",
			wantPaths: nil,
			wantText:  "[file] this is just text without NUL prefix",
		},
		{
			name:      "files with empty text",
			content:   "\x00[file] x.jpg\n ",
			wantPaths: []string{"x.jpg"},
			wantText:  " ",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotPaths, gotText := decodeMediaPaths(tc.content)
			require.Len(t, gotPaths, len(tc.wantPaths))
			for i, p := range gotPaths {
				assert.Equal(t, tc.wantPaths[i], p)
			}
			assert.Equal(t, tc.wantText, gotText)
		})
	}
}

func newLocalChatTestServer(t *testing.T) (*gin.Engine, *local.LocalReceiver, *store.MessageStore, *store.OutboundMessageStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	msgStore := store.NewMessageStore(db)
	outboundStore := store.NewOutboundMessageStore(db)
	receiver := local.NewLocalReceiver(4)
	h := NewLocalChatHandler(receiver, local.NewSSEHub(), outboundStore, msgStore)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/local/messages", h.SendMessage)
	r.GET("/local/messages", h.GetMessages)
	return r, receiver, msgStore, outboundStore
}

type messagesResponse struct {
	Messages []chatMessage `json:"messages"`
	HasMore  bool          `json:"has_more"`
}

func getMessages(t *testing.T, r *gin.Engine, query string) messagesResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/local/messages?"+query, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res messagesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	return res
}

func seedOutbound(t *testing.T, s *store.OutboundMessageStore, id string, sentAt int64) {
	t.Helper()
	require.NoError(t, s.Create(context.Background(), store.OutboundMessage{
		ID: id, SessionKey: defaultSessionKey, Platform: local.PlatformID,
		Content: id, Status: store.OutboundStatusSent, SentAt: sentAt,
	}))
}

func TestGetMessages_ReturnsNewestPageWithIDs(t *testing.T) {
	r, _, msgStore, outboundStore := newLocalChatTestServer(t)
	_, err := msgStore.CreateBatch(context.Background(), []store.BatchMsg{
		{ID: "row-1", SessionKey: defaultSessionKey, Platform: local.PlatformID, Content: "u1", PlatformMsgID: "local:c1", Status: "received", MessageTime: 100},
		{ID: "row-3", SessionKey: defaultSessionKey, Platform: local.PlatformID, Content: "u3", PlatformMsgID: "local:c3", Status: "received", MessageTime: 300},
		{ID: "row-5", SessionKey: defaultSessionKey, Platform: local.PlatformID, Content: "u5", Status: "received", MessageTime: 500},
	})
	require.NoError(t, err)
	seedOutbound(t, outboundStore, "r2", 200)
	seedOutbound(t, outboundStore, "r4", 400)
	seedOutbound(t, outboundStore, "r6", 600)

	res := getMessages(t, r, "limit=3")

	require.True(t, res.HasMore)
	require.Len(t, res.Messages, 3)
	assert.Equal(t, []string{"r4", "row-5", "r6"}, []string{res.Messages[0].ID, res.Messages[1].ID, res.Messages[2].ID})

	newest := getMessages(t, r, "limit=2")

	assert.True(t, newest.HasMore)
	require.Len(t, newest.Messages, 2)
	assert.Equal(t, []string{"row-5", "r6"}, []string{newest.Messages[0].ID, newest.Messages[1].ID})

	older := getMessages(t, r, "limit=3&before=400")

	assert.False(t, older.HasMore)
	require.Len(t, older.Messages, 3)
	assert.Equal(t, []string{"c1", "r2", "c3"}, []string{older.Messages[0].ID, older.Messages[1].ID, older.Messages[2].ID})
}

func TestGetMessages_HasMoreWhenStoresTogetherExceedLimit(t *testing.T) {
	r, _, msgStore, outboundStore := newLocalChatTestServer(t)
	_, err := msgStore.CreateBatch(context.Background(), []store.BatchMsg{
		{ID: "u1", SessionKey: defaultSessionKey, Platform: local.PlatformID, Content: "u1", Status: "received", MessageTime: 100},
		{ID: "u3", SessionKey: defaultSessionKey, Platform: local.PlatformID, Content: "u3", Status: "received", MessageTime: 300},
	})
	require.NoError(t, err)
	seedOutbound(t, outboundStore, "r2", 200)
	seedOutbound(t, outboundStore, "r4", 400)

	res := getMessages(t, r, "limit=3")

	assert.True(t, res.HasMore)
	require.Len(t, res.Messages, 3)
	assert.Equal(t, int64(200), res.Messages[0].Timestamp)
}

func postMessage(t *testing.T, r *gin.Engine, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/local/messages", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func nextInbound(t *testing.T, receiver *local.LocalReceiver) platform.InboundMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got := make(chan platform.InboundMessage, 1)
	go receiver.Start(ctx, func(msg platform.InboundMessage) { //nolint:errcheck
		got <- msg
		cancel()
	})
	select {
	case msg := <-got:
		return msg
	case <-ctx.Done():
		t.Fatal("no inbound message enqueued")
		return platform.InboundMessage{}
	}
}

func TestSendMessage_UsesClientIDAndReturnsServerTimestamp(t *testing.T) {
	r, receiver, _, _ := newLocalChatTestServer(t)

	rec := postMessage(t, r, map[string]any{"id": "client-1", "content": "hi"})

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var res struct {
		ID string `json:"id"`
		TS int64  `json:"ts"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.Equal(t, "client-1", res.ID)
	msg := nextInbound(t, receiver)
	assert.Equal(t, "local:client-1", msg.PlatformMessageID)
	assert.Equal(t, res.TS, msg.MessageTime)
}

func TestSendMessage_GeneratesIDWhenMissing(t *testing.T) {
	r, receiver, _, _ := newLocalChatTestServer(t)

	rec := postMessage(t, r, map[string]any{"content": "hi"})

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var res struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	require.NotEmpty(t, res.ID)
	assert.Equal(t, "local:"+res.ID, nextInbound(t, receiver).PlatformMessageID)
}

func TestSendMessage_RejectsInvalidID(t *testing.T) {
	r, _, _, _ := newLocalChatTestServer(t)

	rec := postMessage(t, r, map[string]any{"id": "bad id/..", "content": "hi"})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
