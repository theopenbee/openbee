package api

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
)

const testUserID = "user-1"
const testUserHeader = "X-Test-User"
const anonymousUser = "-"

var testSessionKey = local.SessionKey(testUserID)

type fakeClearer struct {
	mu            sync.Mutex
	sessionActive []model.Task
	workerActive  []model.Task
	calls         []string
	keys          []string
	err           error
}

func (f *fakeClearer) record(call, key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	f.keys = append(f.keys, key)
}

func (f *fakeClearer) callList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeClearer) keyList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.keys...)
}

func (f *fakeClearer) EvaluateClearSession(_ context.Context, sessionKey string) (session.ClearSessionPreview, error) {
	f.record("evaluate-session", sessionKey)
	return session.ClearSessionPreview{ActiveTasks: f.sessionActive}, f.err
}

func (f *fakeClearer) ClearSession(_ context.Context, sessionKey string, _ session.ClearSessionPreview) (session.ClearSessionResult, error) {
	f.record("clear-session", sessionKey)
	return session.ClearSessionResult{}, f.err
}

func (f *fakeClearer) EvaluateClearWorker(_ context.Context, sessionKey string, w model.Worker) (session.ClearWorkerPreview, error) {
	f.record("evaluate-worker:"+w.ID, sessionKey)
	return session.ClearWorkerPreview{ActiveTasks: f.workerActive}, f.err
}

func (f *fakeClearer) ClearWorker(_ context.Context, sessionKey string, w model.Worker, _ session.ClearWorkerPreview) (session.ClearWorkerResult, error) {
	f.record("clear-worker:"+w.ID, sessionKey)
	return session.ClearWorkerResult{}, f.err
}

func (f *fakeClearer) StopConversation(_ context.Context, sessionKey string, w model.Worker) (session.StopWorkerResult, error) {
	f.record("stop:"+w.ID, sessionKey)
	return session.StopWorkerResult{}, f.err
}

type localChatTestServer struct {
	router        *gin.Engine
	receiver      *local.LocalReceiver
	hub           *local.SSEHub
	publisher     *local.EventPublisher
	msgStore      *store.MessageStore
	outboundStore *store.OutboundMessageStore
	chats         *store.LocalChatStore
	workers       *store.WorkerStore
	clear         *fakeClearer
	mediaRoot     string
}

func newLocalChatServer(t *testing.T) localChatTestServer {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := localChatTestServer{
		receiver:      local.NewLocalReceiver(4),
		hub:           local.NewSSEHub(),
		msgStore:      store.NewMessageStore(db),
		outboundStore: store.NewOutboundMessageStore(db),
		chats:         store.NewLocalChatStore(db),
		workers:       store.NewWorkerStore(db),
		clear:         &fakeClearer{},
		mediaRoot:     t.TempDir(),
	}
	s.publisher = local.NewEventPublisher(s.hub)
	h := NewLocalChatHandler(LocalChatDeps{
		Receiver:  s.receiver,
		Hub:       s.hub,
		Publisher: s.publisher,
		Messages:  s.msgStore,
		Outbound:  s.outboundStore,
		Chats:     s.chats,
		Workers:   s.workers,
		Clear:     s.clear,
		MediaRoot: s.mediaRoot,
	})

	s.router = localChatRouter(h)
	return s
}

func localChatRouter(h *LocalChatHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if uid := c.GetHeader(testUserHeader); uid != anonymousUser {
			auth.SetUserID(c, cmp.Or(uid, testUserID))
		}
		c.Next()
	})
	r.GET("/local/conversations", h.ListConversations)
	r.GET("/local/conversations/:cid/messages", h.GetMessages)
	r.POST("/local/conversations/:cid/messages", h.SendMessage)
	r.POST("/local/conversations/:cid/read", h.MarkRead)
	r.POST("/local/conversations/:cid/clear", h.Clear)
	r.POST("/local/conversations/:cid/stop", h.Stop)
	r.POST("/local/media", h.UploadMedia)
	r.GET("/local/media/:filename", h.ServeMedia)
	r.GET("/local/stream", h.StreamReplies)
	return r
}

func (s localChatTestServer) createWorker(t *testing.T, name string) model.Worker {
	t.Helper()
	w, err := s.workers.Create(model.Worker{Name: name})
	require.NoError(t, err)
	return w
}

func newLocalChatTestServer(t *testing.T) (*gin.Engine, *local.LocalReceiver, *store.MessageStore, *store.OutboundMessageStore) {
	t.Helper()
	s := newLocalChatServer(t)
	return s.router, s.receiver, s.msgStore, s.outboundStore
}

type messagesResponse struct {
	Messages []local.ChatMessage `json:"messages"`
	HasMore  bool                `json:"has_more"`
}

func serveAs(r *gin.Engine, req *http.Request, userID string) *httptest.ResponseRecorder {
	req.Header.Set(testUserHeader, userID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func getMessages(t *testing.T, r *gin.Engine, query string) messagesResponse {
	t.Helper()
	return getConversationMessages(t, r, testUserID, local.BeeConversationID, query)
}

func getMessagesAs(t *testing.T, r *gin.Engine, userID, query string) messagesResponse {
	t.Helper()
	return getConversationMessages(t, r, userID, local.BeeConversationID, query)
}

func getConversationMessages(t *testing.T, r *gin.Engine, userID, cid, query string) messagesResponse {
	t.Helper()
	rec := serveAs(r, httptest.NewRequest(http.MethodGet, "/local/conversations/"+cid+"/messages?"+query, nil), userID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res messagesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	return res
}

func seedOutbound(t *testing.T, s *store.OutboundMessageStore, id string, sentAt int64) {
	t.Helper()
	seedOutboundIn(t, s, testSessionKey, store.OutboundMessage{ID: id, Content: id, SentAt: sentAt})
}

func seedOutboundIn(t *testing.T, s *store.OutboundMessageStore, sessionKey string, m store.OutboundMessage) {
	t.Helper()
	m.SessionKey = sessionKey
	m.Platform = local.PlatformID
	if m.Status == "" {
		m.Status = store.OutboundStatusSent
	}
	require.NoError(t, s.Create(context.Background(), m))
}

func seedInbound(t *testing.T, s *store.MessageStore, sessionKey string, m store.BatchMsg) {
	t.Helper()
	m.SessionKey = sessionKey
	m.Platform = local.PlatformID
	if m.Status == "" {
		m.Status = "received"
	}
	_, err := s.CreateBatch(context.Background(), []store.BatchMsg{m})
	require.NoError(t, err)
}

func TestGetMessages_ReturnsNewestPageWithIDs(t *testing.T) {
	r, _, msgStore, outboundStore := newLocalChatTestServer(t)
	_, err := msgStore.CreateBatch(context.Background(), []store.BatchMsg{
		{ID: "row-1", SessionKey: testSessionKey, Platform: local.PlatformID, Content: "u1", PlatformMsgID: "local:c1", Status: "received", MessageTime: 100},
		{ID: "row-3", SessionKey: testSessionKey, Platform: local.PlatformID, Content: "u3", PlatformMsgID: "local:c3", Status: "received", MessageTime: 300},
		{ID: "row-5", SessionKey: testSessionKey, Platform: local.PlatformID, Content: "u5", Status: "received", MessageTime: 500},
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
		{ID: "u1", SessionKey: testSessionKey, Platform: local.PlatformID, Content: "u1", Status: "received", MessageTime: 100},
		{ID: "u3", SessionKey: testSessionKey, Platform: local.PlatformID, Content: "u3", Status: "received", MessageTime: 300},
	})
	require.NoError(t, err)
	seedOutbound(t, outboundStore, "r2", 200)
	seedOutbound(t, outboundStore, "r4", 400)

	res := getMessages(t, r, "limit=3")

	assert.True(t, res.HasMore)
	require.Len(t, res.Messages, 3)
	assert.Equal(t, int64(200), res.Messages[0].TS)
}

func postJSON(t *testing.T, r *gin.Engine, userID, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(http.MethodPost, path, reader)
	req.Header.Set("Content-Type", "application/json")
	return serveAs(r, req, userID)
}

func postMessage(t *testing.T, r *gin.Engine, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return postJSON(t, r, testUserID, "/local/conversations/bee/messages", body)
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
	assert.Equal(t, "local:user-1:client-1", msg.PlatformMessageID)
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
	assert.Equal(t, "local:user-1:"+res.ID, nextInbound(t, receiver).PlatformMessageID)
}

func TestSendMessage_RejectsInvalidID(t *testing.T) {
	r, _, _, _ := newLocalChatTestServer(t)

	rec := postMessage(t, r, map[string]any{"id": "bad id/..", "content": "hi"})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSendMessage_ScopesDedupIDToUser(t *testing.T) {
	s := newLocalChatServer(t)

	for _, uid := range []string{testUserID, "user-2"} {
		rec := postJSON(t, s.router, uid, "/local/conversations/bee/messages", map[string]any{"id": "same", "content": "hi"})
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
		assert.Equal(t, "local:"+uid+":same", nextInbound(t, s.receiver).PlatformMessageID)
	}
}

func TestGetMessages_DecodesClientIDs(t *testing.T) {
	s := newLocalChatServer(t)
	_, err := s.msgStore.CreateBatch(context.Background(), []store.BatchMsg{
		{ID: "row-1", SessionKey: testSessionKey, Platform: local.PlatformID, Content: "legacy", PlatformMsgID: "local:c1", Status: "received", MessageTime: 100},
		{ID: "row-2", SessionKey: testSessionKey, Platform: local.PlatformID, Content: "scoped", PlatformMsgID: "local:user-1:c2", Status: "received", MessageTime: 200},
	})
	require.NoError(t, err)

	res := getMessages(t, s.router, "")

	require.Len(t, res.Messages, 2)
	assert.Equal(t, []string{"c1", "c2"}, []string{res.Messages[0].ID, res.Messages[1].ID})
}

func TestGetMessages_KeepsMergedBurstMessages(t *testing.T) {
	s := newLocalChatServer(t)
	w := s.createWorker(t, "alice")
	key := local.WorkerSessionKey(testUserID, w.ID)
	_, err := s.msgStore.CreateBatch(context.Background(), []store.BatchMsg{
		{ID: "row-1", SessionKey: key, Platform: local.PlatformID, Content: "one", PlatformMsgID: "local:user-1:c1", Status: store.MsgStatusMerged, MergedInto: "row-3", MessageTime: 100},
		{ID: "row-2", SessionKey: key, Platform: local.PlatformID, Content: "two", PlatformMsgID: "local:user-1:c2", Status: store.MsgStatusMerged, MergedInto: "row-3", MessageTime: 110},
		{ID: "row-3", SessionKey: key, Platform: local.PlatformID, Content: "three", PlatformMsgID: "local:user-1:c3", Status: store.MsgStatusBeeProcessed, MessageTime: 120},
	})
	require.NoError(t, err)

	burst := listConversations(t, s.router, testUserID)[1]
	require.NotNil(t, burst.LastMessage)
	assert.Equal(t, "three", burst.LastMessage.Preview)
	assert.Equal(t, 0, burst.Unread)

	seedOutboundIn(t, s.outboundStore, key, store.OutboundMessage{ID: "reply", Content: "done", SourceType: store.SourceTypeWorker, SourceID: w.ID, SentAt: 200})

	res := getConversationMessages(t, s.router, testUserID, local.WorkerConversationID(w.ID), "")

	require.Len(t, res.Messages, 4)
	assert.Equal(t, []string{"c1", "c2", "c3", "reply"}, []string{res.Messages[0].ID, res.Messages[1].ID, res.Messages[2].ID, res.Messages[3].ID})
	assert.Equal(t, []string{"one", "two", "three"}, []string{res.Messages[0].Content, res.Messages[1].Content, res.Messages[2].Content})
	for _, m := range res.Messages[:3] {
		assert.Equal(t, local.Sender{Type: "user"}, m.Sender)
	}

	replied := listConversations(t, s.router, testUserID)[1]
	require.NotNil(t, replied.LastMessage)
	assert.Equal(t, "done", replied.LastMessage.Preview)
	assert.Equal(t, 1, replied.Unread)
}

func TestLocalChat_RejectsRequestsWithoutUser(t *testing.T) {
	s := newLocalChatServer(t)

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/local/conversations", nil),
		httptest.NewRequest(http.MethodGet, "/local/conversations/bee/messages", nil),
		httptest.NewRequest(http.MethodPost, "/local/conversations/bee/messages", bytes.NewReader([]byte(`{"content":"hi"}`))),
		httptest.NewRequest(http.MethodPost, "/local/conversations/bee/read", nil),
		httptest.NewRequest(http.MethodPost, "/local/conversations/bee/clear", nil),
		httptest.NewRequest(http.MethodPost, "/local/conversations/worker:w1/stop", nil),
		httptest.NewRequest(http.MethodPost, "/local/media", bytes.NewReader([]byte("payload"))),
		httptest.NewRequest(http.MethodGet, "/local/media/a.png", nil),
		httptest.NewRequest(http.MethodGet, "/local/stream", nil),
	} {
		rec := serveAs(s.router, req, anonymousUser)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, req.Method+" "+req.URL.Path)
	}
}

func TestStreamDeliversEventsForUser(t *testing.T) {
	s := newLocalChatServer(t)
	srv := httptest.NewServer(s.router)
	defer srv.Close()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				s.hub.Publish("user-1", local.Event{Type: local.EventTask, TaskID: "other"})
				s.hub.Publish("user-2", local.Event{Type: local.EventTask, TaskID: "mine"})
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/local/stream", nil)
	require.NoError(t, err)
	req.Header.Set(testUserHeader, "user-2")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	line := nextStreamLine(t, bufio.NewReader(resp.Body), "data: ")
	var ev local.Event
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev))
	assert.Equal(t, "mine", ev.TaskID)
	assert.Positive(t, ev.Seq)
}

func nextStreamLine(t *testing.T, r *bufio.Reader, prefix string) string {
	t.Helper()
	for {
		line, err := r.ReadString('\n')
		require.NoError(t, err)
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
}

func openStream(t *testing.T, h *LocalChatHandler) (*http.Response, *bufio.Reader) {
	t.Helper()
	srv := httptest.NewServer(localChatRouter(h))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/local/stream", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	return resp, bufio.NewReader(resp.Body)
}

func TestStreamOpensBeforeAnyEvent(t *testing.T) {
	h := NewLocalChatHandler(LocalChatDeps{Hub: local.NewSSEHub()})

	resp, body := openStream(t, h)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	line, err := body.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, ": ok\n", line)
}

func TestStreamSendsHeartbeat(t *testing.T) {
	h := NewLocalChatHandler(LocalChatDeps{Hub: local.NewSSEHub(), Heartbeat: 20 * time.Millisecond})

	_, body := openStream(t, h)

	assert.Equal(t, ": ping\n", nextStreamLine(t, body, ": ping"))
}

func TestSendMessage_UsesCurrentUserSession(t *testing.T) {
	s := newLocalChatServer(t)

	rec := postJSON(t, s.router, "user-2", "/local/conversations/bee/messages", map[string]any{"content": "hi"})

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	msg := nextInbound(t, s.receiver)
	assert.Equal(t, "local:user-2", msg.SessionKey)
	assert.Equal(t, "user-2", msg.SenderID)
}

func TestGetMessages_ScopesHistoryToCurrentUser(t *testing.T) {
	s := newLocalChatServer(t)
	_, err := s.msgStore.CreateBatch(context.Background(), []store.BatchMsg{
		{ID: "mine", SessionKey: testSessionKey, Platform: local.PlatformID, Content: "mine", Status: "received", MessageTime: 100},
		{ID: "theirs", SessionKey: local.SessionKey("user-2"), Platform: local.PlatformID, Content: "theirs", Status: "received", MessageTime: 200},
	})
	require.NoError(t, err)
	require.NoError(t, s.outboundStore.Create(context.Background(), store.OutboundMessage{
		ID: "reply-theirs", SessionKey: local.SessionKey("user-2"), Platform: local.PlatformID,
		Content: "reply", Status: store.OutboundStatusSent, SentAt: 300,
	}))

	mine := getMessagesAs(t, s.router, testUserID, "")
	require.Len(t, mine.Messages, 1)
	assert.Equal(t, "mine", mine.Messages[0].ID)

	theirs := getMessagesAs(t, s.router, "user-2", "")
	require.Len(t, theirs.Messages, 2)
	assert.Equal(t, []string{"theirs", "reply-theirs"}, []string{theirs.Messages[0].ID, theirs.Messages[1].ID})
}

func TestGetMessages_ReturnsStagedReplyMedia(t *testing.T) {
	s := newLocalChatServer(t)
	userDir := filepath.Join(s.mediaRoot, testUserID)
	otherDir := filepath.Join(s.mediaRoot, "user-2")
	for dir, names := range map[string][]string{
		userDir:  {"r1_report.pdf", "r2_failed.png"},
		otherDir: {"r4_theirs.png"},
	} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		for _, name := range names {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644))
		}
	}
	for _, m := range []store.OutboundMessage{
		{ID: "r1", MediaPath: "/work/out/report.pdf", Status: store.OutboundStatusSent, SentAt: 100},
		{ID: "r2", MediaPath: "/work/out/failed.png", Status: store.OutboundStatusFailed, SentAt: 200},
		{ID: "r3", Content: "text only", Status: store.OutboundStatusSent, SentAt: 300},
		{ID: "r4", MediaPath: "/work/out/theirs.png", Status: store.OutboundStatusSent, SentAt: 400},
		{ID: "r5", MediaPath: "/work/out/never-staged.png", Status: store.OutboundStatusSent, SentAt: 500},
	} {
		m.SessionKey = testSessionKey
		m.Platform = local.PlatformID
		require.NoError(t, s.outboundStore.Create(context.Background(), m))
	}

	res := getMessages(t, s.router, "")

	require.Len(t, res.Messages, 5)
	assert.Equal(t, []string{"r1_report.pdf"}, res.Messages[0].MediaPaths)
	for _, m := range res.Messages[1:] {
		assert.Empty(t, m.MediaPaths, m.ID)
	}
}

func TestUploadMedia_StoresInCurrentUserDir(t *testing.T) {
	s := newLocalChatServer(t)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "photo.png")
	require.NoError(t, err)
	_, err = fw.Write([]byte("png-bytes"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/local/media", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := serveAs(s.router, req, "user-2")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res struct {
		Path string `json:"path"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	data, err := os.ReadFile(filepath.Join(s.mediaRoot, "user-2", res.Path))
	require.NoError(t, err)
	assert.Equal(t, "png-bytes", string(data))
}

func TestServeMedia_IsolatesUsers(t *testing.T) {
	s := newLocalChatServer(t)
	require.NoError(t, os.MkdirAll(filepath.Join(s.mediaRoot, testUserID), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(s.mediaRoot, testUserID, "mine.png"), []byte(testUserID), 0o644))

	fetch := func(userID, name string) *httptest.ResponseRecorder {
		return serveAs(s.router, httptest.NewRequest(http.MethodGet, "/local/media/"+name, nil), userID)
	}

	rec := fetch(testUserID, "mine.png")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, testUserID, rec.Body.String())

	assert.Equal(t, http.StatusNotFound, fetch("user-2", "mine.png").Code)
}

func TestServeMedia_SandboxesAndDownloadsActiveContent(t *testing.T) {
	s := newLocalChatServer(t)
	userDir := filepath.Join(s.mediaRoot, testUserID)
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	for _, name := range []string{"photo.png", "report.html", "icon.svg", "noext"} {
		require.NoError(t, os.WriteFile(filepath.Join(userDir, name), []byte("<script>alert(1)</script>"), 0o644))
	}

	for _, tc := range []struct {
		name       string
		inlineType string
	}{
		{"photo.png", "image/png"},
		{"icon.svg", "image/svg+xml"},
		{"report.html", ""},
		{"noext", ""},
	} {
		rec := serveAs(s.router, httptest.NewRequest(http.MethodGet, "/local/media/"+tc.name, nil), testUserID)

		require.Equal(t, http.StatusOK, rec.Code, tc.name)
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"), tc.name)
		assert.Equal(t, "sandbox", rec.Header().Get("Content-Security-Policy"), tc.name)
		if tc.inlineType != "" {
			assert.Empty(t, rec.Header().Get("Content-Disposition"), tc.name)
			assert.Equal(t, tc.inlineType, rec.Header().Get("Content-Type"), tc.name)
			continue
		}
		assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment", tc.name)
		assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"), tc.name)
	}
}

func listConversations(t *testing.T, r *gin.Engine, userID string) []ConversationSummary {
	t.Helper()
	rec := serveAs(r, httptest.NewRequest(http.MethodGet, "/local/conversations", nil), userID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res []ConversationSummary
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	return res
}

func conversationIDs(list []ConversationSummary) []string {
	ids := make([]string, 0, len(list))
	for _, c := range list {
		ids = append(ids, c.ID)
	}
	return ids
}

func conversationsByID(list []ConversationSummary) map[string]ConversationSummary {
	byID := make(map[string]ConversationSummary, len(list))
	for _, c := range list {
		byID[c.ID] = c
	}
	return byID
}

func handoffPayload(workerID, taskID string) string {
	return `{"worker_id":"` + workerID + `","task_id":"` + taskID + `","task_type":"immediate","conversation_id":"worker:` + workerID + `"}`
}

func TestConversationsListsBeeAndWorkers(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	bob := s.createWorker(t, "bob")
	aliceKey := local.WorkerSessionKey(testUserID, alice.ID)
	seedInbound(t, s.msgStore, testSessionKey, store.BatchMsg{ID: "b1", Content: "hello bee", MessageTime: 100})
	seedOutboundIn(t, s.outboundStore, aliceKey, store.OutboundMessage{ID: "a1", Content: "first", SourceType: store.SourceTypeWorker, SourceID: alice.ID, SentAt: 200})
	seedOutboundIn(t, s.outboundStore, aliceKey, store.OutboundMessage{ID: "a2", Content: "second", SourceType: store.SourceTypeWorker, SourceID: alice.ID, SentAt: 300})
	require.NoError(t, s.chats.MarkRead(context.Background(), testUserID, local.WorkerConversationID(alice.ID), 200))

	list := listConversations(t, s.router, testUserID)

	require.Len(t, list, 3)
	assert.Equal(t, []string{local.BeeConversationID, local.WorkerConversationID(alice.ID), local.WorkerConversationID(bob.ID)}, conversationIDs(list))
	beeConv, aliceConv, bobConv := list[0], list[1], list[2]

	assert.Equal(t, "bee", beeConv.Kind)
	assert.Empty(t, beeConv.WorkerID)
	require.NotNil(t, beeConv.LastMessage)
	assert.Equal(t, local.Sender{Type: "user"}, beeConv.LastMessage.Sender)
	assert.Equal(t, "hello bee", beeConv.LastMessage.Preview)
	assert.Equal(t, int64(100), beeConv.LastMessage.TS)
	assert.Equal(t, 0, beeConv.Unread)

	assert.Equal(t, "worker", aliceConv.Kind)
	assert.Equal(t, alice.ID, aliceConv.WorkerID)
	require.NotNil(t, aliceConv.LastMessage)
	assert.Equal(t, local.Sender{Type: "worker", ID: alice.ID}, aliceConv.LastMessage.Sender)
	assert.Equal(t, "message", aliceConv.LastMessage.Kind)
	assert.Equal(t, "second", aliceConv.LastMessage.Preview)
	assert.Equal(t, int64(300), aliceConv.LastMessage.TS)
	assert.Equal(t, 1, aliceConv.Unread)

	assert.Equal(t, bob.ID, bobConv.WorkerID)
	assert.Nil(t, bobConv.LastMessage)
	assert.Equal(t, 0, bobConv.Unread)
	assert.Equal(t, local.Activity{State: local.ActivityIdle}, bobConv.Activity)

	rec := postJSON(t, s.router, testUserID, "/local/conversations/worker:"+alice.ID+"/read", map[string]any{"ts": 300})
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, 0, listConversations(t, s.router, testUserID)[1].Unread)
}

func TestConversationsListsBeeWithoutMessages(t *testing.T) {
	s := newLocalChatServer(t)

	list := listConversations(t, s.router, testUserID)

	require.Len(t, list, 1)
	assert.Equal(t, local.BeeConversationID, list[0].ID)
	assert.Nil(t, list[0].LastMessage)
}

func TestConversationsOrdersWorkersByLastMessageThenName(t *testing.T) {
	s := newLocalChatServer(t)
	zed := s.createWorker(t, "zed")
	amy := s.createWorker(t, "amy")
	cat := s.createWorker(t, "cat")
	dan := s.createWorker(t, "dan")
	eve := s.createWorker(t, "eve")
	seedOutboundIn(t, s.outboundStore, local.WorkerSessionKey(testUserID, zed.ID), store.OutboundMessage{ID: "z1", Content: "z", SourceType: store.SourceTypeWorker, SourceID: zed.ID, SentAt: 500})
	seedInbound(t, s.msgStore, local.WorkerSessionKey(testUserID, cat.ID), store.BatchMsg{ID: "c1", Content: "c", MessageTime: 700})
	seedOutboundIn(t, s.outboundStore, local.WorkerSessionKey(testUserID, eve.ID), store.OutboundMessage{ID: "e1", Content: "e", SentAt: 500})

	list := listConversations(t, s.router, testUserID)

	assert.Equal(t, []string{
		local.BeeConversationID,
		local.WorkerConversationID(cat.ID),
		local.WorkerConversationID(eve.ID),
		local.WorkerConversationID(zed.ID),
		local.WorkerConversationID(amy.ID),
		local.WorkerConversationID(dan.ID),
	}, conversationIDs(list))
}

func TestConversationsPreviewStripsMediaAndTruncates(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	bob := s.createWorker(t, "bob")
	seedInbound(t, s.msgStore, local.WorkerSessionKey(testUserID, alice.ID), store.BatchMsg{
		ID: "a1", Content: local.EncodeMediaPaths([]string{"photo.png"}, "look\n\n  at   this\tpic"), MessageTime: 100,
	})
	seedInbound(t, s.msgStore, local.WorkerSessionKey(testUserID, bob.ID), store.BatchMsg{
		ID: "b1", Content: strings.Repeat("字", 130), MessageTime: 200,
	})

	byID := conversationsByID(listConversations(t, s.router, testUserID))

	assert.Equal(t, "look at this pic", byID[local.WorkerConversationID(alice.ID)].LastMessage.Preview)
	assert.Equal(t, strings.Repeat("字", 120), byID[local.WorkerConversationID(bob.ID)].LastMessage.Preview)
}

func TestConversationsReportActivityAndHandoffPreview(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	bob := s.createWorker(t, "bob")
	s.publisher.TaskState(local.WorkerSessionKey(testUserID, alice.ID), "t1", alice.ID, "running", 0)
	s.publisher.TaskState(local.WorkerSessionKey(testUserID, bob.ID), "t2", bob.ID, "queued", 2)
	seedOutboundIn(t, s.outboundStore, testSessionKey, store.OutboundMessage{
		ID: "h1", Content: "write the report", SourceType: store.SourceTypeBee, Kind: store.OutboundKindHandoff,
		Payload: handoffPayload(alice.ID, "t1"), SentAt: 100,
	})

	byID := conversationsByID(listConversations(t, s.router, testUserID))

	assert.Equal(t, local.Activity{State: local.ActivityRunning}, byID[local.WorkerConversationID(alice.ID)].Activity)
	assert.Equal(t, local.Activity{State: local.ActivityQueued, QueuedAhead: 2}, byID[local.WorkerConversationID(bob.ID)].Activity)
	assert.Equal(t, local.Activity{State: local.ActivityIdle}, byID[local.BeeConversationID].Activity)
	beeLast := byID[local.BeeConversationID].LastMessage
	require.NotNil(t, beeLast)
	assert.Equal(t, "handoff", beeLast.Kind)
	assert.Equal(t, local.Sender{Type: "bee"}, beeLast.Sender)
	assert.Equal(t, "write the report", beeLast.Preview)
	assert.Equal(t, alice.ID, beeLast.HandoffWorkerID)

	rec := serveAs(s.router, httptest.NewRequest(http.MethodGet, "/local/conversations", nil), testUserID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"handoff_worker_id":"`+alice.ID+`"`)
}

func TestConversationsHandoffPreviewToleratesMalformedPayload(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	aliceKey := local.WorkerSessionKey(testUserID, alice.ID)
	seedOutboundIn(t, s.outboundStore, testSessionKey, store.OutboundMessage{
		ID: "h1", Content: "write the report", SourceType: store.SourceTypeBee, Kind: store.OutboundKindHandoff,
		Payload: "not json", SentAt: 100,
	})
	seedOutboundIn(t, s.outboundStore, aliceKey, store.OutboundMessage{
		ID: "a1", Content: "done", SourceType: store.SourceTypeWorker, SourceID: alice.ID,
		Payload: handoffPayload(alice.ID, "t1"), SentAt: 200,
	})

	rec := serveAs(s.router, httptest.NewRequest(http.MethodGet, "/local/conversations", nil), testUserID)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "handoff_worker_id")
	byID := conversationsByID(listConversations(t, s.router, testUserID))
	beeLast := byID[local.BeeConversationID].LastMessage
	require.NotNil(t, beeLast)
	assert.Equal(t, "handoff", beeLast.Kind)
	assert.Equal(t, "write the report", beeLast.Preview)
	assert.Empty(t, beeLast.HandoffWorkerID)
	aliceLast := byID[local.WorkerConversationID(alice.ID)].LastMessage
	require.NotNil(t, aliceLast)
	assert.Equal(t, "message", aliceLast.Kind)
	assert.Empty(t, aliceLast.HandoffWorkerID)
}

func TestConversationsScopeToCurrentUser(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	seedOutboundIn(t, s.outboundStore, local.WorkerSessionKey("user-2", alice.ID), store.OutboundMessage{
		ID: "o1", Content: "theirs", SourceType: store.SourceTypeWorker, SourceID: alice.ID, SentAt: 100,
	})

	mine := listConversations(t, s.router, testUserID)
	theirs := listConversations(t, s.router, "user-2")

	assert.Nil(t, mine[1].LastMessage)
	assert.Equal(t, 0, mine[1].Unread)
	require.NotNil(t, theirs[1].LastMessage)
	assert.Equal(t, 1, theirs[1].Unread)
}

func TestSendToWorkerConversationUsesWorkerKey(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")

	rec := postJSON(t, s.router, testUserID, "/local/conversations/worker:"+alice.ID+"/messages", map[string]any{
		"id": "client-9", "content": "do it", "media_paths": []string{"a.png"},
	})

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var res struct {
		ID string `json:"id"`
		TS int64  `json:"ts"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.Equal(t, "client-9", res.ID)
	msg := nextInbound(t, s.receiver)
	assert.Equal(t, "local:user-1:worker:"+alice.ID, msg.SessionKey)
	assert.Equal(t, testUserID, msg.SenderID)
	assert.Equal(t, "local:user-1:client-9", msg.PlatformMessageID)
	assert.Equal(t, res.TS, msg.MessageTime)
	paths, text := local.DecodeMediaPaths(msg.Content)
	assert.Equal(t, []string{"a.png"}, paths)
	assert.Equal(t, "do it", text)
}

func TestSendToBeeConversationUsesBeeKey(t *testing.T) {
	s := newLocalChatServer(t)

	rec := postJSON(t, s.router, testUserID, "/local/conversations/bee/messages", map[string]any{"content": "hi"})

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	assert.Equal(t, "local:user-1", nextInbound(t, s.receiver).SessionKey)
}

func TestSendToUnknownWorker404(t *testing.T) {
	s := newLocalChatServer(t)

	rec := postJSON(t, s.router, testUserID, "/local/conversations/worker:missing/messages", map[string]any{"content": "hi"})

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"error":"conversation not found"}`, rec.Body.String())
}

func TestUnknownConversation404OnEveryRoute(t *testing.T) {
	s := newLocalChatServer(t)

	for _, cid := range []string{"worker:missing", "worker:", "worker:a:b", "nope", "bee:x", "Bee"} {
		for _, route := range []struct{ method, suffix string }{
			{http.MethodGet, "/messages"},
			{http.MethodPost, "/messages"},
			{http.MethodPost, "/read"},
			{http.MethodPost, "/clear"},
			{http.MethodPost, "/stop"},
		} {
			req := httptest.NewRequest(route.method, "/local/conversations/"+cid+route.suffix, bytes.NewReader([]byte(`{"content":"hi"}`)))
			req.Header.Set("Content-Type", "application/json")
			rec := serveAs(s.router, req, testUserID)

			label := route.method + " " + cid + route.suffix
			assert.Equal(t, http.StatusNotFound, rec.Code, label)
			assert.JSONEq(t, `{"error":"conversation not found"}`, rec.Body.String(), label)
		}
	}
	assert.Empty(t, s.clear.callList())
}

type failingWorkers struct{}

func (failingWorkers) List() ([]model.Worker, error) { return nil, errors.New("db down") }

func (failingWorkers) GetByID(string) (model.Worker, error) {
	return model.Worker{}, errors.New("db down")
}

func TestWorkerStoreFailureIs500(t *testing.T) {
	s := newLocalChatServer(t)
	r := localChatRouter(NewLocalChatHandler(LocalChatDeps{
		Receiver: s.receiver, Hub: s.hub, Publisher: s.publisher, Messages: s.msgStore, Outbound: s.outboundStore,
		Chats: s.chats, Workers: failingWorkers{}, Clear: s.clear, MediaRoot: s.mediaRoot,
	}))

	for _, path := range []string{"/local/conversations", "/local/conversations/worker:w1/messages"} {
		rec := serveAs(r, httptest.NewRequest(http.MethodGet, path, nil), testUserID)

		assert.Equal(t, http.StatusInternalServerError, rec.Code, path)
	}
}

func TestConversationIDAcceptsEncodedColon(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")

	rec := postJSON(t, s.router, testUserID, "/local/conversations/worker%3A"+alice.ID+"/messages", map[string]any{"content": "hi"})

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	assert.Equal(t, "local:user-1:worker:"+alice.ID, nextInbound(t, s.receiver).SessionKey)
}

func TestMessagesAttributeWorkerAndHandoff(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	seedInbound(t, s.msgStore, testSessionKey, store.BatchMsg{ID: "u1", Content: "please ask alice", PlatformMsgID: "local:user-1:c1", MessageTime: 100})
	seedOutboundIn(t, s.outboundStore, testSessionKey, store.OutboundMessage{ID: "bee-old", Content: "on it", SentAt: 200})
	seedOutboundIn(t, s.outboundStore, testSessionKey, store.OutboundMessage{ID: "bee-new", Content: "delegating", SourceType: store.SourceTypeBee, SourceID: "ignored", SentAt: 250})
	seedOutboundIn(t, s.outboundStore, testSessionKey, store.OutboundMessage{ID: "card", Content: "write the report", SourceType: store.SourceTypeBee, Kind: store.OutboundKindHandoff, Payload: handoffPayload(alice.ID, "t1"), SentAt: 300})
	seedOutboundIn(t, s.outboundStore, testSessionKey, store.OutboundMessage{ID: "legacy-worker", Content: "legacy reply", SourceType: store.SourceTypeWorker, SourceID: alice.ID, SentAt: 400})
	seedOutboundIn(t, s.outboundStore, testSessionKey, store.OutboundMessage{ID: "sys", Content: "failed", SourceType: store.SourceTypeSystem, SentAt: 500})

	res := getMessages(t, s.router, "")

	require.Len(t, res.Messages, 6)
	byID := map[string]local.ChatMessage{}
	for _, m := range res.Messages {
		byID[m.ID] = m
	}
	assert.Equal(t, local.Sender{Type: "user"}, byID["c1"].Sender)
	assert.Equal(t, "message", byID["c1"].Kind)
	assert.Equal(t, local.Sender{Type: "bee"}, byID["bee-old"].Sender)
	assert.Equal(t, local.Sender{Type: "bee"}, byID["bee-new"].Sender)
	assert.Equal(t, "message", byID["bee-new"].Kind)
	assert.Nil(t, byID["bee-new"].Handoff)
	assert.Equal(t, local.Sender{Type: "worker", ID: alice.ID}, byID["legacy-worker"].Sender)
	assert.Equal(t, local.Sender{Type: "system"}, byID["sys"].Sender)
	card := byID["card"]
	assert.Equal(t, "handoff", card.Kind)
	assert.Equal(t, "write the report", card.Content)
	require.NotNil(t, card.Handoff)
	assert.Equal(t, local.HandoffInfo{WorkerID: alice.ID, TaskID: "t1", TaskType: "immediate", ConversationID: "worker:" + alice.ID}, *card.Handoff)
}

func TestMessagesInWorkerConversationMapSenders(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	bob := s.createWorker(t, "bob")
	aliceKey := local.WorkerSessionKey(testUserID, alice.ID)
	seedInbound(t, s.msgStore, aliceKey, store.BatchMsg{ID: "row-u", Content: "from me", PlatformMsgID: "local:user-1:cu", MessageTime: 100})
	seedInbound(t, s.msgStore, aliceKey, store.BatchMsg{ID: "row-bee", Content: "handed off", SenderType: store.SenderTypeBee, Status: store.MsgStatusBeeProcessed, MessageTime: 200})
	seedOutboundIn(t, s.outboundStore, aliceKey, store.OutboundMessage{ID: "w1", Content: "done", SourceType: store.SourceTypeWorker, SourceID: alice.ID, SentAt: 300})
	seedOutboundIn(t, s.outboundStore, local.WorkerSessionKey(testUserID, bob.ID), store.OutboundMessage{ID: "bob-1", Content: "other chat", SourceType: store.SourceTypeWorker, SourceID: bob.ID, SentAt: 350})
	seedInbound(t, s.msgStore, testSessionKey, store.BatchMsg{ID: "bee-chat", Content: "bee chat", MessageTime: 400})

	res := getConversationMessages(t, s.router, testUserID, local.WorkerConversationID(alice.ID), "")

	require.Len(t, res.Messages, 3)
	assert.Equal(t, []string{"cu", "row-bee", "w1"}, []string{res.Messages[0].ID, res.Messages[1].ID, res.Messages[2].ID})
	assert.Equal(t, local.Sender{Type: "user"}, res.Messages[0].Sender)
	assert.Equal(t, local.Sender{Type: "bee"}, res.Messages[1].Sender)
	assert.Equal(t, local.Sender{Type: "worker", ID: alice.ID}, res.Messages[2].Sender)
}

func TestMessagesInWorkerConversationReturnMedia(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	userDir := filepath.Join(s.mediaRoot, testUserID)
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "r1_report.pdf"), []byte("pdf"), 0o644))
	aliceKey := local.WorkerSessionKey(testUserID, alice.ID)
	seedOutboundIn(t, s.outboundStore, aliceKey, store.OutboundMessage{ID: "r1", MediaPath: "/work/out/report.pdf", SourceType: store.SourceTypeWorker, SourceID: alice.ID, SentAt: 100})
	seedInbound(t, s.msgStore, aliceKey, store.BatchMsg{ID: "u1", Content: local.EncodeMediaPaths([]string{"photo.png"}, "see"), MessageTime: 200})

	res := getConversationMessages(t, s.router, testUserID, local.WorkerConversationID(alice.ID), "")

	require.Len(t, res.Messages, 2)
	assert.Equal(t, []string{"r1_report.pdf"}, res.Messages[0].MediaPaths)
	assert.Equal(t, []string{"photo.png"}, res.Messages[1].MediaPaths)
	assert.Equal(t, "see", res.Messages[1].Content)
}

func TestMarkReadDefaultsToNow(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	cid := local.WorkerConversationID(alice.ID)
	before := time.Now().UnixMilli()

	rec := postJSON(t, s.router, testUserID, "/local/conversations/"+cid+"/read", nil)

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	marks, err := s.chats.ReadMarks(context.Background(), testUserID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, marks[cid], before)
	assert.LessOrEqual(t, marks[cid], time.Now().UnixMilli())
}

func TestMarkReadStoresGivenTimestampPerUser(t *testing.T) {
	s := newLocalChatServer(t)

	rec := postJSON(t, s.router, "user-2", "/local/conversations/bee/read", map[string]any{"ts": 1234})

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	mine, err := s.chats.ReadMarks(context.Background(), testUserID)
	require.NoError(t, err)
	assert.Empty(t, mine)
	theirs, err := s.chats.ReadMarks(context.Background(), "user-2")
	require.NoError(t, err)
	assert.Equal(t, int64(1234), theirs[local.BeeConversationID])
}

func TestMarkReadRejectsMalformedBody(t *testing.T) {
	s := newLocalChatServer(t)
	req := httptest.NewRequest(http.MethodPost, "/local/conversations/bee/read", strings.NewReader("{nope"))
	req.Header.Set("Content-Type", "application/json")

	rec := serveAs(s.router, req, testUserID)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestClearBeeConversationClearsSession(t *testing.T) {
	s := newLocalChatServer(t)

	rec := postJSON(t, s.router, testUserID, "/local/conversations/bee/clear", nil)

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"evaluate-session", "clear-session"}, s.clear.callList())
	assert.Equal(t, []string{"local:user-1", "local:user-1"}, s.clear.keyList())
}

func TestClearBeeConversationConflictWithoutForce(t *testing.T) {
	s := newLocalChatServer(t)
	s.clear.sessionActive = []model.Task{{ID: "t1", Instruction: "build", Type: model.TaskTypeImmediate}}

	rec := postJSON(t, s.router, testUserID, "/local/conversations/bee/clear", map[string]any{})

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, []string{"evaluate-session"}, s.clear.callList())
}

func TestClearWorkerConflictWithoutForce(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	s.clear.workerActive = []model.Task{
		{ID: "t1", Instruction: "write report", Type: model.TaskTypeImmediate, SessionKey: "ignored"},
		{ID: "t2", Instruction: "nightly digest", Type: model.TaskTypeScheduled},
	}
	path := "/local/conversations/worker:" + alice.ID + "/clear"

	rec := postJSON(t, s.router, testUserID, path, map[string]any{"force": false})

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"error":"active tasks","active_tasks":[
		{"id":"t1","instruction":"write report","type":"immediate"},
		{"id":"t2","instruction":"nightly digest","type":"scheduled"}]}`, rec.Body.String())
	assert.Equal(t, []string{"evaluate-worker:" + alice.ID}, s.clear.callList())

	rec = postJSON(t, s.router, testUserID, path, map[string]any{"force": true})

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"evaluate-worker:" + alice.ID, "evaluate-worker:" + alice.ID, "clear-worker:" + alice.ID}, s.clear.callList())
	for _, key := range s.clear.keyList() {
		assert.Equal(t, "local:user-1:worker:"+alice.ID, key)
	}
}

func TestClearWorkerWithoutActiveTasksNeedsNoBody(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")

	rec := postJSON(t, s.router, testUserID, "/local/conversations/worker:"+alice.ID+"/clear", nil)

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"evaluate-worker:" + alice.ID, "clear-worker:" + alice.ID}, s.clear.callList())
}

func TestClearFailureIs500(t *testing.T) {
	s := newLocalChatServer(t)
	s.clear.err = errors.New("boom")

	rec := postJSON(t, s.router, testUserID, "/local/conversations/bee/clear", nil)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func nextHubEvent(t *testing.T, ch <-chan string) local.Event {
	t.Helper()
	select {
	case data := <-ch:
		var ev local.Event
		require.NoError(t, json.Unmarshal([]byte(data), &ev))
		return ev
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
		return local.Event{}
	}
}

func resetRows(t *testing.T, s localChatTestServer, sessionKey string) []store.OutboundMessage {
	t.Helper()
	rows, err := s.outboundStore.ListBySessionKey(context.Background(), sessionKey, 0, 100)
	require.NoError(t, err)
	var resets []store.OutboundMessage
	for _, r := range rows {
		if r.Kind == store.OutboundKindReset {
			resets = append(resets, r)
		}
	}
	return resets
}

func TestClearAppendsResetDivider(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	cid := local.WorkerConversationID(alice.ID)
	key := local.WorkerSessionKey(testUserID, alice.ID)
	seedOutboundIn(t, s.outboundStore, key, store.OutboundMessage{ID: "r1", Content: "done", SourceType: store.SourceTypeWorker, SourceID: alice.ID, SentAt: 100})
	events, unsub := s.hub.Subscribe(testUserID)
	defer unsub()

	rec := postJSON(t, s.router, testUserID, "/local/conversations/"+cid+"/clear", nil)

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	res := getConversationMessages(t, s.router, testUserID, cid, "")
	require.Len(t, res.Messages, 2)
	divider := res.Messages[1]
	assert.Equal(t, local.KindReset, divider.Kind)
	assert.Equal(t, local.Sender{Type: store.SourceTypeSystem}, divider.Sender)
	assert.Empty(t, divider.Content)
	assert.Greater(t, divider.TS, int64(100))

	ev := nextHubEvent(t, events)
	assert.Equal(t, local.EventMessage, ev.Type)
	assert.Equal(t, cid, ev.ConversationID)
	require.NotNil(t, ev.Message)
	assert.Equal(t, divider, *ev.Message)

	convs := listConversations(t, s.router, testUserID)
	for _, c := range convs {
		if c.ID != cid {
			continue
		}
		require.NotNil(t, c.LastMessage)
		assert.Equal(t, "done", c.LastMessage.Preview)
		assert.Equal(t, int64(100), c.LastMessage.TS)
		assert.Equal(t, 1, c.Unread)
	}
}

func TestClearBeeAppendsResetDivider(t *testing.T) {
	s := newLocalChatServer(t)
	seedOutbound(t, s.outboundStore, "r1", 100)

	rec := postJSON(t, s.router, testUserID, "/local/conversations/bee/clear", nil)

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Len(t, resetRows(t, s, testSessionKey), 1)
}

func TestClearSkipsResetOnEmptyChat(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")

	rec := postJSON(t, s.router, testUserID, "/local/conversations/worker:"+alice.ID+"/clear", nil)

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Empty(t, resetRows(t, s, local.WorkerSessionKey(testUserID, alice.ID)))
}

func TestClearSkipsResetAfterReset(t *testing.T) {
	s := newLocalChatServer(t)
	seedOutbound(t, s.outboundStore, "r1", 100)

	for range 2 {
		rec := postJSON(t, s.router, testUserID, "/local/conversations/bee/clear", nil)
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	}

	assert.Len(t, resetRows(t, s, testSessionKey), 1)
}

func TestClearWithoutClearingAddsNoReset(t *testing.T) {
	s := newLocalChatServer(t)
	seedOutbound(t, s.outboundStore, "r1", 100)
	s.clear.sessionActive = []model.Task{{ID: "t1", Instruction: "build", Type: model.TaskTypeImmediate}}

	rec := postJSON(t, s.router, testUserID, "/local/conversations/bee/clear", nil)
	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Empty(t, resetRows(t, s, testSessionKey))

	s.clear.sessionActive = nil
	s.clear.err = errors.New("boom")
	rec = postJSON(t, s.router, testUserID, "/local/conversations/bee/clear", nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, resetRows(t, s, testSessionKey))
}

func TestStopBeeConversation400(t *testing.T) {
	s := newLocalChatServer(t)

	rec := postJSON(t, s.router, testUserID, "/local/conversations/bee/stop", nil)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, s.clear.callList())
}

func TestStopWorkerConversationStopsWorkerChat(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")

	rec := postJSON(t, s.router, testUserID, "/local/conversations/worker:"+alice.ID+"/stop", nil)

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"stop:" + alice.ID}, s.clear.callList())
	assert.Equal(t, []string{"local:user-1:worker:" + alice.ID}, s.clear.keyList())
}

func TestStopFailureIs500(t *testing.T) {
	s := newLocalChatServer(t)
	alice := s.createWorker(t, "alice")
	s.clear.err = errors.New("boom")

	rec := postJSON(t, s.router, testUserID, "/local/conversations/worker:"+alice.ID+"/stop", nil)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
