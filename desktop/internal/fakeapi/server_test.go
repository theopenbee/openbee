package fakeapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/sse"
)

const waitLimit = 3 * time.Second

type memTokens struct {
	mu      sync.Mutex
	access  string
	refresh string
}

func (m *memTokens) Access() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.access
}

func (m *memTokens) Refresh() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.refresh
}

func (m *memTokens) SetAccess(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.access = token
}

type session struct {
	fake         *fakeapi.Server
	client       *api.Client
	tokens       *memTokens
	unauthorized *int
}

func newSession(t *testing.T) *session {
	t.Helper()
	fake := fakeapi.New(t)
	tokens := &memTokens{}
	count := new(int)
	var mu sync.Mutex
	client := api.New(api.Options{
		BaseURL: fake.URL,
		Tokens:  tokens,
		OnUnauthorized: func() {
			mu.Lock()
			defer mu.Unlock()
			*count++
		},
	})
	return &session{fake: fake, client: client, tokens: tokens, unauthorized: count}
}

func signedIn(t *testing.T) *session {
	t.Helper()
	s := newSession(t)
	pair, err := s.client.Login(context.Background(), "admin", "secret")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	s.tokens.access = pair.AccessToken
	s.tokens.refresh = pair.RefreshToken
	return s
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), waitLimit)
	t.Cleanup(cancel)
	return c
}

func apiError(t *testing.T, err error) *api.Error {
	t.Helper()
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *api.Error", err)
	}
	return apiErr
}

func mustWorker(t *testing.T, s *session, name string) api.Worker {
	t.Helper()
	w, err := s.client.CreateWorker(ctx(t), api.CreateWorkerRequest{Name: name})
	if err != nil {
		t.Fatalf("CreateWorker(%q): %v", name, err)
	}
	return w
}

type liveStream struct {
	events chan json.RawMessage
	done   chan struct{}
}

func openSSE(t *testing.T, s *session, path string) *liveStream {
	t.Helper()
	c, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(c, http.MethodGet, s.fake.URL+"/api"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.client.DoRaw(c, req)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	ls := &liveStream{events: make(chan json.RawMessage, 64), done: make(chan struct{})}
	go func() {
		defer close(ls.done)
		defer resp.Body.Close()
		_ = sse.Read(resp.Body, func(ev sse.Event) error {
			ls.events <- json.RawMessage(ev.Data)
			return nil
		})
	}()
	return ls
}

func (ls *liveStream) next(t *testing.T) json.RawMessage {
	t.Helper()
	select {
	case raw := <-ls.events:
		return raw
	case <-time.After(waitLimit):
		t.Fatal("timed out waiting for a stream event")
		return nil
	}
}

func (ls *liveStream) quiet(t *testing.T) {
	t.Helper()
	select {
	case raw := <-ls.events:
		t.Fatalf("unexpected stream event %s", raw)
	case <-time.After(100 * time.Millisecond):
	}
}

func (ls *liveStream) ended(t *testing.T) {
	t.Helper()
	select {
	case <-ls.done:
	case <-time.After(waitLimit):
		t.Fatal("stream did not end")
	}
}

func openChat(t *testing.T, s *session) *liveStream {
	t.Helper()
	ls := openSSE(t, s, api.ChatStreamPath)
	waitUntil(t, "stream registered", func() bool { return s.fake.Streams() == 1 })
	return ls
}

func (ls *liveStream) chatEvent(t *testing.T) api.ChatEvent {
	t.Helper()
	var ev api.ChatEvent
	if err := json.Unmarshal(ls.next(t), &ev); err != nil {
		t.Fatalf("decode chat event: %v", err)
	}
	return ev
}

func (ls *liveStream) setupEvent(t *testing.T) api.SetupEvent {
	t.Helper()
	var ev api.SetupEvent
	if err := json.Unmarshal(ls.next(t), &ev); err != nil {
		t.Fatalf("decode setup event: %v", err)
	}
	return ev
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitLimit)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLoginAndWorkersRoundTrip(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)

	workers, err := s.client.Workers(c)
	if err != nil || len(workers) != 0 {
		t.Fatalf("Workers = %v, %v; want empty", workers, err)
	}

	created, err := s.client.CreateWorker(c, api.CreateWorkerRequest{
		Name: "  sam ", Engine: "codex", Description: "builder", EngineArgs: map[string]string{"codex": "--fast"},
	})
	if err != nil {
		t.Fatalf("CreateWorker: %v", err)
	}
	if created.ID == "" || created.Name != "sam" || created.Status != "idle" || created.Engine != "codex" ||
		created.WorkDir == "" || created.CreatedAt == 0 || created.EngineArgs["codex"] != "--fast" {
		t.Fatalf("created worker = %+v", created)
	}

	got, err := s.client.Worker(c, created.ID)
	if err != nil || !reflect.DeepEqual(got, created) {
		t.Fatalf("Worker = %+v, %v; want %+v", got, err, created)
	}

	convs, err := s.client.Conversations(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(convs) != 2 || convs[0].ID != "bee" || convs[1].ID != api.WorkerConversation(created.ID) ||
		convs[1].Kind != "worker" || convs[1].WorkerID != created.ID {
		t.Fatalf("Conversations = %+v", convs)
	}

	name := "samuel"
	updated, err := s.client.UpdateWorker(c, created.ID, api.UpdateWorkerRequest{Name: &name})
	if err != nil || updated.Name != "samuel" || updated.UpdatedAt < created.UpdatedAt {
		t.Fatalf("UpdateWorker = %+v, %v", updated, err)
	}

	if err := s.client.DeleteWorker(c, created.ID, true); err != nil {
		t.Fatalf("DeleteWorker: %v", err)
	}
	if _, err := s.client.Worker(c, created.ID); apiError(t, err).Code != "worker_not_found" || apiError(t, err).Status != 404 {
		t.Fatalf("Worker after delete: %v", err)
	}
	convs, err = s.client.Conversations(c)
	if err != nil || len(convs) != 1 || convs[0].ID != "bee" {
		t.Fatalf("Conversations after delete = %+v, %v", convs, err)
	}
	if calls := s.fake.Calls("DELETE", "/api/workers/"); len(calls) != 1 || calls[0].Query != "delete_work_dir=true" {
		t.Fatalf("delete calls = %+v", calls)
	}
}

func TestWorkerValidation(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	mustWorker(t, s, "sam")

	_, err := s.client.CreateWorker(c, api.CreateWorkerRequest{Name: "sam"})
	if e := apiError(t, err); e.Status != 400 || e.Code != "worker_validation" {
		t.Fatalf("duplicate name: %v", err)
	}
	_, err = s.client.CreateWorker(c, api.CreateWorkerRequest{Name: ""})
	if e := apiError(t, err); e.Status != 400 || e.Code != "" {
		t.Fatalf("empty name: %v", err)
	}
	_, err = s.client.CreateWorker(c, api.CreateWorkerRequest{Name: "x", PermissionScopes: "read:workers,write:all"})
	if e := apiError(t, err); e.Status != 400 || !strings.Contains(e.Message, "write:all") {
		t.Fatalf("scopes: %v", err)
	}
	_, err = s.client.CreateWorker(c, api.CreateWorkerRequest{Name: "x", Engine: "gemini"})
	if e := apiError(t, err); e.Status != 400 || e.Message != `engine "gemini" is not enabled` {
		t.Fatalf("engine: %v", err)
	}

	other := mustWorker(t, s, "other")
	bad := "gemini"
	_, err = s.client.UpdateWorker(c, other.ID, api.UpdateWorkerRequest{Engine: &bad})
	if apiError(t, err).Status != 500 {
		t.Fatalf("update with disabled engine: %v", err)
	}
	blank := "  "
	_, err = s.client.UpdateWorker(c, other.ID, api.UpdateWorkerRequest{WorkDir: &blank})
	if e := apiError(t, err); e.Status != 400 || e.Code != "worker_validation" {
		t.Fatalf("blank work dir: %v", err)
	}
	taken := "sam"
	_, err = s.client.UpdateWorker(c, other.ID, api.UpdateWorkerRequest{Name: &taken})
	if e := apiError(t, err); e.Status != 400 || e.Code != "worker_validation" {
		t.Fatalf("taken name: %v", err)
	}
	if _, err := s.client.UpdateWorker(c, "missing", api.UpdateWorkerRequest{}); apiError(t, err).Code != "worker_not_found" {
		t.Fatalf("update missing: %v", err)
	}
}

func TestUpdateWorkerEngineArgsMerge(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	w, err := s.client.CreateWorker(c, api.CreateWorkerRequest{
		Name: "sam", EngineArgs: map[string]string{"claude": "--a", "codex": "--b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]string{"claude": "", "pi": "--c"}
	got, err := s.client.UpdateWorker(c, w.ID, api.UpdateWorkerRequest{EngineArgs: &args})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"codex": "--b", "pi": "--c"}; !reflect.DeepEqual(got.EngineArgs, want) {
		t.Fatalf("merged args = %v, want %v", got.EngineArgs, want)
	}
	cleared := map[string]string{}
	got, err = s.client.UpdateWorker(c, w.ID, api.UpdateWorkerRequest{EngineArgs: &cleared})
	if err != nil || got.EngineArgs == nil || len(got.EngineArgs) != 0 {
		t.Fatalf("cleared args = %#v, %v", got.EngineArgs, err)
	}
	unchanged, err := s.client.UpdateWorker(c, w.ID, api.UpdateWorkerRequest{})
	if err != nil || unchanged.UpdatedAt != got.UpdatedAt {
		t.Fatalf("no-op update changed updated_at: %+v vs %+v (%v)", unchanged, got, err)
	}
}

func TestRandomWorkerName(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	if got, err := s.client.RandomWorkerName(c); err != nil || !got.Exhausted || got.Name != "" {
		t.Fatalf("empty pool = %+v, %v", got, err)
	}
	s.fake.Update(func(st *fakeapi.State) { st.RandomNames = []string{"ada", "bob"} })
	if got, err := s.client.RandomWorkerName(c); err != nil || got.Name != "ada" {
		t.Fatalf("first = %+v, %v", got, err)
	}
	mustWorker(t, s, "ada")
	if got, err := s.client.RandomWorkerName(c); err != nil || got.Name != "bob" {
		t.Fatalf("second = %+v, %v", got, err)
	}
	mustWorker(t, s, "bob")
	if got, err := s.client.RandomWorkerName(c); err != nil || !got.Exhausted {
		t.Fatalf("exhausted = %+v, %v", got, err)
	}
}

func TestSetupAndLogin(t *testing.T) {
	s := newSession(t)
	c := ctx(t)

	cfg, err := s.client.Config(c)
	if err != nil || cfg.Language != "en" || !reflect.DeepEqual(cfg.EnabledEngines, []string{"claude", "codex", "pi"}) {
		t.Fatalf("Config = %+v, %v", cfg, err)
	}
	if st, err := s.client.SetupStatus(c); err != nil || !st.Initialized {
		t.Fatalf("SetupStatus = %+v, %v", st, err)
	}
	if _, err := s.client.Setup(c, api.SetupRequest{Username: "x", Password: "123456"}); apiError(t, err).Status != 409 {
		t.Fatalf("setup when initialized: %v", err)
	}
	_, err = s.client.Login(c, "admin", "nope")
	if e := apiError(t, err); e.Status != 401 || e.Message != "invalid username or password" {
		t.Fatalf("wrong password: %v", err)
	}
	_, err = s.client.Login(c, "other", "secret")
	if apiError(t, err).Status != 401 {
		t.Fatalf("wrong user: %v", err)
	}
	if _, err := s.client.Workers(c); !errors.Is(err, api.ErrUnauthorized) {
		t.Fatalf("Workers without tokens: %v, want ErrUnauthorized", err)
	}

	s.fake.Update(func(st *fakeapi.State) {
		st.Initialized = false
		st.Language = "zh"
		st.EnabledEngines = nil
	})
	if st, err := s.client.SetupStatus(c); err != nil || st.Initialized {
		t.Fatalf("SetupStatus uninitialised = %+v, %v", st, err)
	}
	cfg, err = s.client.Config(c)
	if err != nil || cfg.Language != "zh" || cfg.EnabledEngines == nil || len(cfg.EnabledEngines) != 0 {
		t.Fatalf("Config = %+v, %v", cfg, err)
	}
	if _, err := s.client.Setup(c, api.SetupRequest{Username: "root", Password: "123"}); apiError(t, err).Status != 400 {
		t.Fatalf("short password: %v", err)
	}
	pair, err := s.client.Setup(c, api.SetupRequest{Username: "root", Password: "123456", DisplayName: "Root"})
	if err != nil || pair.AccessToken == "" || pair.RefreshToken == "" || pair.ExpiresIn != 7200 {
		t.Fatalf("Setup = %+v, %v", pair, err)
	}
	snap := s.fake.Snapshot()
	if !snap.Initialized || snap.Username != "root" || snap.Password != "123456" {
		t.Fatalf("state after setup = %+v", snap)
	}
	if _, err := s.client.Login(c, "root", "123456"); err != nil {
		t.Fatalf("Login after setup: %v", err)
	}
}

func TestAccessTokenFromQuery(t *testing.T) {
	s := signedIn(t)
	for _, tc := range []struct {
		name  string
		token string
		want  int
	}{
		{"valid", s.tokens.Access(), 200},
		{"invalid", "access-1", 401},
		{"missing", "", 401},
	} {
		resp, err := http.Get(s.fake.URL + "/api/workers?token=" + tc.token)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, resp.StatusCode, tc.want)
		}
	}
}

func TestIssuedAccessTokenIsFreshJWT(t *testing.T) {
	s := signedIn(t)
	if err := s.client.EnsureFresh(ctx(t)); err != nil {
		t.Fatalf("EnsureFresh: %v", err)
	}
	if calls := s.fake.Calls("POST", "/api/auth/refresh"); len(calls) != 0 {
		t.Fatalf("fresh token triggered %d refresh calls", len(calls))
	}
}

func TestExpiredAccessTokenRefreshesTransparently(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	before := s.tokens.Access()
	s.fake.ExpireAccessTokens()

	if _, err := s.client.Workers(c); err != nil {
		t.Fatalf("Workers after expiry: %v", err)
	}
	if s.tokens.Access() == before {
		t.Fatal("access token was not replaced")
	}
	if calls := s.fake.Calls("POST", "/api/auth/refresh"); len(calls) != 1 {
		t.Fatalf("refresh calls = %d, want 1", len(calls))
	}
	if calls := s.fake.Calls("GET", "/api/workers"); len(calls) != 2 {
		t.Fatalf("workers calls = %d, want 2 (401 then retry)", len(calls))
	}
	if _, err := s.client.Workers(c); err != nil {
		t.Fatalf("Workers with refreshed token: %v", err)
	}
	if *s.unauthorized != 0 {
		t.Fatalf("OnUnauthorized fired %d times", *s.unauthorized)
	}
}

func TestRevokedRefreshTokenIsUnauthorized(t *testing.T) {
	s := signedIn(t)
	s.fake.ExpireAccessTokens()
	s.fake.RevokeRefreshTokens()

	if _, err := s.client.Workers(ctx(t)); !errors.Is(err, api.ErrUnauthorized) {
		t.Fatalf("Workers = %v, want ErrUnauthorized", err)
	}
	if *s.unauthorized != 1 {
		t.Fatalf("OnUnauthorized fired %d times, want 1", *s.unauthorized)
	}
	if _, err := s.client.Login(ctx(t), "admin", "secret"); err != nil {
		t.Fatalf("Login after revoke: %v", err)
	}
}

func TestChangePasswordRevokesTokens(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	err := s.client.ChangePassword(c, "wrong", "newsecret")
	if e := apiError(t, err); e.Status != 400 || e.Code != "old_password_incorrect" {
		t.Fatalf("wrong old password: %v", err)
	}
	if err := s.client.ChangePassword(c, "secret", "123"); apiError(t, err).Status != 400 {
		t.Fatalf("short new password: %v", err)
	}
	if err := s.client.ChangePassword(c, "secret", "newsecret"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := s.client.Workers(c); !errors.Is(err, api.ErrUnauthorized) {
		t.Fatalf("Workers after password change: %v, want ErrUnauthorized", err)
	}
	if _, err := s.client.Login(c, "admin", "secret"); apiError(t, err).Status != 401 {
		t.Fatalf("old password still works: %v", err)
	}
	if _, err := s.client.Login(c, "admin", "newsecret"); err != nil {
		t.Fatalf("new password: %v", err)
	}
}

func TestSendMessageThenMessages(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	stream := openChat(t, s)

	resp, err := s.client.SendMessage(c, "bee", api.SendMessageRequest{ID: "m-1", Content: "hello", MediaPaths: []string{"a.png"}})
	if err != nil || resp.ID != "m-1" || resp.TS <= 0 {
		t.Fatalf("SendMessage = %+v, %v", resp, err)
	}
	again, err := s.client.SendMessage(c, "bee", api.SendMessageRequest{ID: "m-1", Content: "hello"})
	if err != nil || again != resp {
		t.Fatalf("resend = %+v, %v; want %+v", again, err, resp)
	}
	generated, err := s.client.SendMessage(c, "bee", api.SendMessageRequest{Content: ""})
	if err != nil || generated.ID == "" || generated.TS <= resp.TS {
		t.Fatalf("generated = %+v, %v", generated, err)
	}

	page, err := s.client.Messages(c, "bee", 0, 0)
	if err != nil || page.HasMore || len(page.Messages) != 2 {
		t.Fatalf("Messages = %+v, %v", page, err)
	}
	first := page.Messages[0]
	if first.ID != "m-1" || first.TS != resp.TS || first.Content != "hello" || first.Kind != "message" ||
		first.Sender.Type != "user" || !reflect.DeepEqual(first.MediaPaths, []string{"a.png"}) {
		t.Fatalf("stored message = %+v", first)
	}
	if page.Messages[1].Content != " " {
		t.Fatalf("empty content stored as %q, want a single space", page.Messages[1].Content)
	}

	convs, err := s.client.Conversations(c)
	if err != nil || convs[0].LastMessage == nil || convs[0].LastMessage.Sender.Type != "user" ||
		convs[0].LastMessage.TS != generated.TS || convs[0].Unread != 0 {
		t.Fatalf("summary after send = %+v, %v", convs, err)
	}

	s.fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: "w", Status: "idle"})
	if ev := stream.chatEvent(t); ev.Type != "worker_status" || ev.Seq != 1 {
		t.Fatalf("first stream event = %+v; own messages must not be published", ev)
	}

	for _, tc := range []struct {
		name string
		cid  string
		req  api.SendMessageRequest
		want int
	}{
		{"bad id", "bee", api.SendMessageRequest{ID: "no spaces", Content: "x"}, 400},
		{"bad media", "bee", api.SendMessageRequest{Content: "x", MediaPaths: []string{"../a.png"}}, 400},
		{"unknown conversation", "nope", api.SendMessageRequest{Content: "x"}, 404},
		{"unknown worker", "worker:ghost", api.SendMessageRequest{Content: "x"}, 404},
	} {
		if _, err := s.client.SendMessage(c, tc.cid, tc.req); apiError(t, err).Status != tc.want {
			t.Errorf("%s: %v, want status %d", tc.name, err, tc.want)
		}
	}
}

func TestMessagesPagination(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	s.fake.Update(func(st *fakeapi.State) {
		for i := int64(5); i >= 1; i-- {
			st.Messages["bee"] = append(st.Messages["bee"], api.ChatMessage{
				ID: "m" + string(rune('0'+i)), Sender: api.ChatSender{Type: "bee"}, Kind: "message", TS: i * 100,
			})
		}
		for i := int64(1); i <= 60; i++ {
			st.Messages["worker:w"] = append(st.Messages["worker:w"], api.ChatMessage{ID: "x", TS: i})
		}
		st.Workers = append(st.Workers, api.Worker{ID: "w", Name: "w"})
	})

	ids := func(p api.MessagesPage) []string {
		var out []string
		for _, m := range p.Messages {
			out = append(out, m.ID)
		}
		return out
	}
	page, err := s.client.Messages(c, "bee", 0, 2)
	if err != nil || !page.HasMore || !reflect.DeepEqual(ids(page), []string{"m4", "m5"}) {
		t.Fatalf("latest = %v hasMore=%v, %v", ids(page), page.HasMore, err)
	}
	page, err = s.client.Messages(c, "bee", page.Messages[0].TS, 2)
	if err != nil || !page.HasMore || !reflect.DeepEqual(ids(page), []string{"m2", "m3"}) {
		t.Fatalf("older = %v hasMore=%v, %v", ids(page), page.HasMore, err)
	}
	page, err = s.client.Messages(c, "bee", page.Messages[0].TS, 2)
	if err != nil || page.HasMore || !reflect.DeepEqual(ids(page), []string{"m1"}) {
		t.Fatalf("oldest = %v hasMore=%v, %v", ids(page), page.HasMore, err)
	}
	page, err = s.client.Messages(c, "bee", 100, 5)
	if err != nil || page.HasMore || page.Messages == nil || len(page.Messages) != 0 {
		t.Fatalf("empty page = %+v, %v", page, err)
	}
	page, err = s.client.Messages(c, "worker:w", 0, 500)
	if err != nil || len(page.Messages) != 50 || !page.HasMore || page.Messages[49].TS != 60 {
		t.Fatalf("out-of-range limit: %d messages hasMore=%v, %v", len(page.Messages), page.HasMore, err)
	}
	if _, err := s.client.Messages(c, "nope", 0, 0); apiError(t, err).Status != 404 {
		t.Fatalf("unknown conversation: %v", err)
	}
}

func TestConversationsOrder(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	zed := mustWorker(t, s, "zed")
	amy := mustWorker(t, s, "amy")
	bob := mustWorker(t, s, "bob")
	s.fake.Reply(api.WorkerConversation(bob.ID), api.ChatMessage{Content: "older", TS: 100})
	s.fake.Reply(api.WorkerConversation(zed.ID), api.ChatMessage{Content: "newer", TS: 200})
	convs, err := s.client.Conversations(c)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, cv := range convs {
		got = append(got, cv.ID)
	}
	want := []string{"bee", api.WorkerConversation(zed.ID), api.WorkerConversation(bob.ID), api.WorkerConversation(amy.ID)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestEmitReachesStreamWithIncreasingSeq(t *testing.T) {
	s := signedIn(t)
	first := openChat(t, s)
	second := openSSE(t, s, api.ChatStreamPath)
	waitUntil(t, "two streams", func() bool { return s.fake.Streams() == 2 })

	s.fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: "w1", Status: "working"})
	s.fake.Emit(api.ChatEvent{Type: "task", ConversationID: "worker:w1", TaskID: "t1", State: "running", Seq: 99})
	for _, ls := range []*liveStream{first, second} {
		a, b := ls.chatEvent(t), ls.chatEvent(t)
		if a.Seq != 1 || a.Type != "worker_status" || a.WorkerID != "w1" || a.Status != "working" {
			t.Errorf("first event = %+v", a)
		}
		if b.Seq != 2 || b.Type != "task" || b.TaskID != "t1" || b.State != "running" {
			t.Errorf("second event = %+v", b)
		}
	}
}

func TestSeqCountsEventsWithoutSubscribers(t *testing.T) {
	s := signedIn(t)
	s.fake.Emit(api.ChatEvent{Type: "worker_status"})
	s.fake.Emit(api.ChatEvent{Type: "worker_status"})
	stream := openChat(t, s)
	s.fake.Emit(api.ChatEvent{Type: "worker_status"})
	if ev := stream.chatEvent(t); ev.Seq != 3 {
		t.Fatalf("seq = %d, want 3", ev.Seq)
	}
}

func TestStreamSendsOkCommentOnConnect(t *testing.T) {
	s := signedIn(t)
	req, err := http.NewRequestWithContext(ctx(t), http.MethodGet, s.fake.URL+"/api/local/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.client.DoRaw(ctx(t), req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil || line != ": ok\n" {
		t.Fatalf("first line = %q, %v", line, err)
	}
	if blank, _ := reader.ReadString('\n'); blank != "\n" {
		t.Fatalf("second line = %q", blank)
	}
	s.fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: "w"})
	line, err = reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, `data: {"type":"worker_status","seq":1`) {
		t.Fatalf("event line = %q, %v", line, err)
	}
}

func TestStreamRequiresAuthorization(t *testing.T) {
	s := newSession(t)
	resp, err := http.Get(s.fake.URL + "/api/local/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("status %d, content type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if s.fake.Streams() != 0 {
		t.Fatal("unauthorised request registered a stream")
	}
}

func TestDropStreamsClosesEveryStream(t *testing.T) {
	s := signedIn(t)
	first := openChat(t, s)
	second := openSSE(t, s, api.ChatStreamPath)
	waitUntil(t, "two streams", func() bool { return s.fake.Streams() == 2 })

	s.fake.DropStreams()
	if n := s.fake.Streams(); n != 0 {
		t.Fatalf("Streams after drop = %d", n)
	}
	first.ended(t)
	second.ended(t)

	reconnected := openChat(t, s)
	s.fake.Emit(api.ChatEvent{Type: "worker_status"})
	if ev := reconnected.chatEvent(t); ev.Seq != 1 {
		t.Fatalf("seq after reconnect = %d, want 1", ev.Seq)
	}
}

func TestReplyStoresUpdatesSummaryAndEmits(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	w := mustWorker(t, s, "sam")
	cid := api.WorkerConversation(w.ID)
	stream := openChat(t, s)

	s.fake.Reply(cid, api.ChatMessage{Content: "\x00[file] a.png\nhello\n  world "})
	ev := stream.chatEvent(t)
	if ev.Type != "message" || ev.ConversationID != cid || ev.Seq != 1 || ev.Message == nil {
		t.Fatalf("event = %+v", ev)
	}
	m := ev.Message
	if m.ID == "" || m.TS <= 0 || m.Kind != "message" || m.Sender.Type != "worker" || m.Sender.ID != w.ID {
		t.Fatalf("filled message = %+v", m)
	}

	page, err := s.client.Messages(c, cid, 0, 0)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != m.ID {
		t.Fatalf("Messages = %+v, %v", page, err)
	}
	convs, err := s.client.Conversations(c)
	if err != nil {
		t.Fatal(err)
	}
	sum := convs[1]
	if sum.ID != cid || sum.Unread != 1 || sum.LastMessage == nil || sum.LastMessage.Preview != "hello world" ||
		sum.LastMessage.TS != m.TS || sum.LastMessage.Sender.Type != "worker" {
		t.Fatalf("summary = %+v (last %+v)", sum, sum.LastMessage)
	}

	s.fake.Reply("bee", api.ChatMessage{
		ID: "h1", Kind: "handoff", Content: "do it", TS: 5000,
		Handoff: &api.HandoffInfo{WorkerID: w.ID, TaskID: "t", TaskType: "immediate", ConversationID: cid},
	})
	if ev := stream.chatEvent(t); ev.Seq != 2 || ev.Message.Sender.Type != "bee" || ev.Message.Kind != "handoff" {
		t.Fatalf("handoff event = %+v", ev)
	}
	convs, err = s.client.Conversations(c)
	if err != nil || convs[0].Unread != 1 || convs[0].LastMessage.HandoffWorkerID != w.ID || convs[0].LastMessage.Kind != "handoff" {
		t.Fatalf("bee summary = %+v, %v", convs[0], err)
	}

	if err := s.client.MarkRead(c, "bee", 0); err != nil {
		t.Fatal(err)
	}
	convs, _ = s.client.Conversations(c)
	if convs[0].Unread != 0 {
		t.Fatalf("unread after mark read = %d", convs[0].Unread)
	}
	if err := s.client.MarkRead(c, "nope", 0); apiError(t, err).Status != 404 {
		t.Fatalf("mark read unknown: %v", err)
	}
}

func TestMarkReadWithTimestampKeepsNewerUnread(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	for _, ts := range []int64{100, 200, 300} {
		s.fake.Reply("bee", api.ChatMessage{TS: ts, Content: "m"})
	}
	if convs, _ := s.client.Conversations(c); convs[0].Unread != 3 {
		t.Fatalf("unread = %d, want 3", convs[0].Unread)
	}
	if err := s.client.MarkRead(c, "bee", 200); err != nil {
		t.Fatal(err)
	}
	if convs, _ := s.client.Conversations(c); convs[0].Unread != 1 {
		t.Fatalf("unread after read at 200 = %d, want 1", convs[0].Unread)
	}
}

func TestClearWithActiveTasksConflicts(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	w := mustWorker(t, s, "sam")
	cid := api.WorkerConversation(w.ID)
	tasks := []api.ActiveTask{{ID: "t1", Instruction: "build", Type: "immediate"}, {ID: "t2", Instruction: "nightly", Type: "scheduled"}}
	s.fake.Update(func(st *fakeapi.State) { st.ActiveTasks[cid] = tasks })
	s.fake.Reply(cid, api.ChatMessage{Content: "working"})
	stream := openChat(t, s)

	err := s.client.ClearConversation(c, cid, false)
	if e := apiError(t, err); e.Status != 409 || e.Message != "active tasks" {
		t.Fatalf("clear = %v", err)
	}
	if got := api.ActiveTasks(err); !reflect.DeepEqual(got, tasks) {
		t.Fatalf("ActiveTasks = %+v, want %+v", got, tasks)
	}
	if page, _ := s.client.Messages(c, cid, 0, 0); len(page.Messages) != 1 {
		t.Fatalf("conflicting clear changed history: %+v", page.Messages)
	}
	stream.quiet(t)

	if err := s.client.ClearConversation(c, cid, true); err != nil {
		t.Fatalf("forced clear: %v", err)
	}
	ev := stream.chatEvent(t)
	if ev.Type != "message" || ev.ConversationID != cid || ev.Message.Kind != "reset" || ev.Message.Sender.Type != "system" {
		t.Fatalf("reset event = %+v", ev)
	}
	if left := s.fake.Snapshot().ActiveTasks[cid]; len(left) != 0 {
		t.Fatalf("active tasks after force = %+v", left)
	}
	page, err := s.client.Messages(c, cid, 0, 0)
	if err != nil || len(page.Messages) != 2 || page.Messages[1].Kind != "reset" || page.Messages[1].ID != ev.Message.ID {
		t.Fatalf("history after clear = %+v, %v", page.Messages, err)
	}
	convs, _ := s.client.Conversations(c)
	if convs[1].LastMessage == nil || convs[1].LastMessage.Kind != "message" {
		t.Fatalf("reset leaked into last_message: %+v", convs[1].LastMessage)
	}

	if err := s.client.ClearConversation(c, cid, false); err != nil {
		t.Fatalf("second clear: %v", err)
	}
	stream.quiet(t)
	if page, _ := s.client.Messages(c, cid, 0, 0); len(page.Messages) != 2 {
		t.Fatalf("second clear appended another divider: %+v", page.Messages)
	}
}

func TestClearEmptyConversationAddsNoDivider(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	stream := openChat(t, s)
	if err := s.client.ClearConversation(c, "bee", false); err != nil {
		t.Fatal(err)
	}
	stream.quiet(t)
	if page, _ := s.client.Messages(c, "bee", 0, 0); len(page.Messages) != 0 {
		t.Fatalf("history = %+v", page.Messages)
	}
	if err := s.client.ClearConversation(c, "ghost", false); apiError(t, err).Status != 404 {
		t.Fatalf("clear unknown: %v", err)
	}
}

func TestStopConversation(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	w := mustWorker(t, s, "sam")
	if err := s.client.StopConversation(c, api.WorkerConversation(w.ID)); err != nil {
		t.Fatalf("stop worker: %v", err)
	}
	if err := s.client.StopConversation(c, "bee"); apiError(t, err).Status != 400 {
		t.Fatalf("stop bee: %v", err)
	}
	if err := s.client.StopConversation(c, "worker:ghost"); apiError(t, err).Status != 404 {
		t.Fatalf("stop unknown: %v", err)
	}
}

func TestFailNextReturnsCannedErrorOnce(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	s.fake.FailNext("GET", "/api/workers", 503, map[string]any{"error": "maintenance", "code": "down"})

	_, err := s.client.Workers(c)
	if e := apiError(t, err); e.Status != 503 || e.Message != "maintenance" || e.Code != "down" {
		t.Fatalf("first Workers: %v", err)
	}
	if _, err := s.client.Workers(c); err != nil {
		t.Fatalf("second Workers: %v", err)
	}

	s.fake.FailNext("POST", "/api/local/conversations/worker:w/read", 500, "boom")
	s.fake.FailNext("GET", "/api/config", 418, nil)
	if _, err := s.client.Config(c); apiError(t, err).Status != 418 {
		t.Fatalf("Config: %v", err)
	}
	if err := s.client.MarkRead(c, "bee", 0); err != nil {
		t.Fatalf("unrelated path was failed: %v", err)
	}
	s.fake.Update(func(st *fakeapi.State) { st.Workers = append(st.Workers, api.Worker{ID: "w", Name: "w"}) })
	err = s.client.MarkRead(c, "worker:w", 0)
	if e := apiError(t, err); e.Status != 500 || e.Message != "Internal Server Error" {
		t.Fatalf("decoded-path match: %v", err)
	}
	if err := s.client.MarkRead(c, "worker:w", 0); err != nil {
		t.Fatalf("canned failure repeated: %v", err)
	}
}

func TestFailNextFifoPerRoute(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	s.fake.FailNext("GET", "/api/workers", 500, map[string]any{"error": "one"})
	s.fake.FailNext("GET", "/api/workers", 502, map[string]any{"error": "two"})
	for _, want := range []int{500, 502} {
		if _, err := s.client.Workers(c); apiError(t, err).Status != want {
			t.Fatalf("Workers: %v, want status %d", err, want)
		}
	}
	if _, err := s.client.Workers(c); err != nil {
		t.Fatal(err)
	}
}

func TestHoldNextDelaysOneRequest(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	release := s.fake.HoldNext("get", "/api/workers")
	t.Cleanup(release)
	done := make(chan error, 1)
	go func() {
		_, err := s.client.Workers(c)
		done <- err
	}()
	deadline := time.Now().Add(waitLimit)
	for len(s.fake.Calls("GET", "/api/workers")) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("held request never reached the fake")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := s.client.Config(c); err != nil {
		t.Fatalf("unrelated request blocked by the hold: %v", err)
	}
	select {
	case err := <-done:
		t.Fatalf("held request answered before release: %v", err)
	default:
	}
	release()
	release()
	if err := <-done; err != nil {
		t.Fatalf("released request: %v", err)
	}
	if _, err := s.client.Workers(c); err != nil {
		t.Fatalf("hold applied twice: %v", err)
	}
}

func TestUnknownPathReturnsHTML(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)

	if err := s.client.Do(c, http.MethodGet, "/no-such-route", nil, new(map[string]any)); !errors.Is(err, api.ErrNotOpenBee) {
		t.Fatalf("unknown path: %v, want ErrNotOpenBee", err)
	}
	if err := s.client.Do(c, http.MethodPatch, "/workers", nil, new(map[string]any)); !errors.Is(err, api.ErrNotOpenBee) {
		t.Fatalf("wrong method: %v, want ErrNotOpenBee", err)
	}
	for _, path := range []string{"/api/nope", "/", "/api/workers/a/b"} {
		resp, err := http.Get(s.fake.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") || !bytes.Contains(body, []byte("<html")) {
			t.Errorf("%s: status %d, content type %q", path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
	if _, err := api.Probe(c, nil, s.fake.URL); err != nil {
		t.Fatalf("Probe of the fake: %v", err)
	}
}

func TestCallsRecordRequests(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	w := mustWorker(t, s, "sam")
	if _, err := s.client.Messages(c, api.WorkerConversation(w.ID), 50, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.client.SendMessage(c, "bee", api.SendMessageRequest{ID: "a1", Content: "hi"}); err != nil {
		t.Fatal(err)
	}

	logins := s.fake.Calls("POST", "/api/auth")
	if len(logins) != 1 || logins[0].Path != "/api/auth/login" || !strings.Contains(string(logins[0].Body), `"username":"admin"`) {
		t.Fatalf("login calls = %+v", logins)
	}
	reads := s.fake.Calls("GET", "/api/local/conversations/")
	if len(reads) != 1 || reads[0].Path != "/api/local/conversations/worker:"+w.ID+"/messages" ||
		reads[0].Query != "before=50&limit=10" {
		t.Fatalf("message reads = %+v", reads)
	}
	sends := s.fake.Calls("post", "/api/local/conversations/bee")
	if len(sends) != 1 {
		t.Fatalf("sends = %+v", sends)
	}
	var sent api.SendMessageRequest
	if err := json.Unmarshal(sends[0].Body, &sent); err != nil || sent.ID != "a1" || sent.Content != "hi" {
		t.Fatalf("recorded body %q: %+v, %v", sends[0].Body, sent, err)
	}
	if all := s.fake.Calls("", "/api/"); len(all) != 4 {
		t.Fatalf("all calls = %d, want 4: %+v", len(all), all)
	}
	if none := s.fake.Calls("DELETE", "/api/"); len(none) != 0 {
		t.Fatalf("delete calls = %+v", none)
	}
	sends[0].Body[0] = 'X'
	if again := s.fake.Calls("POST", "/api/local/conversations/bee"); again[0].Body[0] != '{' {
		t.Fatal("Calls returned a shared body slice")
	}
}

func TestMediaUploadAndDownload(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	png := []byte("\x89PNG fake image bytes")
	path, err := s.client.UploadMedia(c, "my pic?.png", bytes.NewReader(png))
	if err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
	if path == "" || strings.ContainsAny(path, `/\ ?`) || !strings.HasSuffix(path, "_my_pic_.png") {
		t.Fatalf("path = %q", path)
	}
	if !bytes.Equal(s.fake.Snapshot().Media[path], png) {
		t.Fatal("upload not stored in State.Media")
	}

	var got bytes.Buffer
	ct, err := s.client.DownloadMedia(c, path, &got)
	if err != nil || ct != "image/png" || !bytes.Equal(got.Bytes(), png) {
		t.Fatalf("DownloadMedia = %q, %q, %v", ct, got.String(), err)
	}

	s.fake.Update(func(st *fakeapi.State) { st.Media["report.bin"] = []byte("data") })
	got.Reset()
	ct, err = s.client.DownloadMedia(c, "/data/media/report.bin", &got)
	if err != nil || ct != "application/octet-stream" || got.String() != "data" {
		t.Fatalf("attachment download = %q, %q, %v", ct, got.String(), err)
	}
	req, _ := http.NewRequestWithContext(c, http.MethodGet, s.fake.URL+"/api/local/media/report.bin?token="+s.tokens.Access(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if cd := resp.Header.Get("Content-Disposition"); cd != `attachment; filename=report.bin` ||
		resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Content-Security-Policy") != "sandbox" {
		t.Fatalf("headers = %v", resp.Header)
	}

	if _, err := s.client.DownloadMedia(c, "missing.png", io.Discard); apiError(t, err).Status != 404 {
		t.Fatalf("missing media: %v", err)
	}
}

func TestEnvs(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)

	if got, err := s.client.Envs(c, "global", ""); err != nil || len(got) != 0 {
		t.Fatalf("empty list = %v, %v", got, err)
	}
	resp, err := http.Get(s.fake.URL + "/api/envs?scope=global&token=" + s.tokens.Access())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.TrimSpace(string(raw)) != "null" {
		t.Fatalf("empty env list body = %q, want null", raw)
	}
	if _, err := s.client.Envs(c, "galaxy", ""); apiError(t, err).Code != "env_invalid_scope" {
		t.Fatalf("invalid scope: %v", err)
	}

	global, err := s.client.CreateEnv(c, api.CreateEnvRequest{Scope: "global", ScopeID: "ignored", Key: "TOKEN", Value: "abcdefghijkl"})
	if err != nil || global.ScopeID != nil || global.Masked != "abcd****ijkl" || global.ID == "" {
		t.Fatalf("global env = %+v, %v", global, err)
	}
	bee, err := s.client.CreateEnv(c, api.CreateEnvRequest{Scope: "bee", ScopeID: api.DefaultBeeID, Key: "K", Value: "short"})
	if err != nil || bee.ScopeID == nil || *bee.ScopeID != "default" || bee.Masked != "****" {
		t.Fatalf("bee env = %+v, %v", bee, err)
	}

	for _, tc := range []struct {
		name string
		req  api.CreateEnvRequest
		code string
		want int
	}{
		{"reserved", api.CreateEnvRequest{Scope: "global", Key: "OPENBEE_API_KEY", Value: "x"}, "env_reserved_key", 400},
		{"scope id", api.CreateEnvRequest{Scope: "worker", Key: "K", Value: "x"}, "env_scope_id_required", 400},
		{"scope", api.CreateEnvRequest{Scope: "galaxy", Key: "K", Value: "x"}, "", 400},
		{"empty key", api.CreateEnvRequest{Scope: "global", Value: "x"}, "", 400},
		{"empty value", api.CreateEnvRequest{Scope: "global", Key: "K"}, "", 400},
		{"duplicate", api.CreateEnvRequest{Scope: "bee", ScopeID: "default", Key: "K", Value: "y"}, "", 500},
	} {
		_, err := s.client.CreateEnv(c, tc.req)
		if e := apiError(t, err); e.Status != tc.want || e.Code != tc.code {
			t.Errorf("%s: %v (code %q), want %d %q", tc.name, err, e.Code, tc.want, tc.code)
		}
	}
	if _, err := s.client.CreateEnv(c, api.CreateEnvRequest{Scope: "global", Key: "TOKEN", Value: "again"}); err != nil {
		t.Fatalf("duplicate global key must be accepted: %v", err)
	}

	globals, err := s.client.Envs(c, "global", "")
	if err != nil || len(globals) != 2 {
		t.Fatalf("globals = %+v, %v", globals, err)
	}
	if bees, err := s.client.Envs(c, "bee", "default"); err != nil || len(bees) != 1 || bees[0].ID != bee.ID {
		t.Fatalf("bee envs = %+v, %v", bees, err)
	}
	if none, err := s.client.Envs(c, "bee", "other"); err != nil || len(none) != 0 {
		t.Fatalf("other scope id = %+v, %v", none, err)
	}

	if err := s.client.UpdateEnv(c, bee.ID, "a-much-longer-value"); err != nil {
		t.Fatal(err)
	}
	if bees, _ := s.client.Envs(c, "bee", "default"); bees[0].Masked != "a-mu****alue" {
		t.Fatalf("masked after update = %q", bees[0].Masked)
	}
	if err := s.client.UpdateEnv(c, "missing", "v"); apiError(t, err).Code != "env_not_found" {
		t.Fatalf("update missing: %v", err)
	}
	if err := s.client.DeleteEnv(c, bee.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.client.DeleteEnv(c, bee.ID); apiError(t, err).Code != "env_not_found" {
		t.Fatalf("delete twice: %v", err)
	}
	s.fake.Update(func(st *fakeapi.State) {
		st.Envs = append(st.Envs, api.EnvConfig{ID: "r", Scope: "global", Key: "OPENBEE_API_KEY"})
	})
	if err := s.client.DeleteEnv(c, "r"); apiError(t, err).Code != "env_reserved_key" {
		t.Fatalf("delete reserved: %v", err)
	}
}

func TestSystemConfigs(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	got, err := s.client.SystemConfigs(c)
	want := map[string]string{"default_engine": "", "engine_args_global": "", "engine_args_bee": ""}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("SystemConfigs = %v, %v", got, err)
	}
	if err := s.client.SetSystemConfig(c, "default_engine", "codex"); err != nil {
		t.Fatal(err)
	}
	if err := s.client.SetSystemConfig(c, "engine_args_global", `{"claude":"--x"}`); err != nil {
		t.Fatal(err)
	}
	got, _ = s.client.SystemConfigs(c)
	if got["default_engine"] != "codex" || got["engine_args_global"] != `{"claude":"--x"}` {
		t.Fatalf("after set = %v", got)
	}
	if err := s.client.SetSystemConfig(c, "engine_args_global", "{}"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.client.SystemConfigs(c); got["engine_args_global"] != "" {
		t.Fatalf("cleared args = %q", got["engine_args_global"])
	}
	for _, tc := range []struct{ key, value string }{
		{"default_engine", "gemini"},
		{"engine_args_bee", "not json"},
		{"engine_args_bee", `{"gemini":"--x"}`},
		{"mystery", "x"},
	} {
		if err := s.client.SetSystemConfig(c, tc.key, tc.value); apiError(t, err).Status != 400 {
			t.Errorf("%s=%q: %v, want 400", tc.key, tc.value, err)
		}
	}
	if err := s.client.SetSystemConfig(c, "default_engine", ""); err != nil {
		t.Fatalf("clearing default engine: %v", err)
	}
}

func seedPlugin(st *fakeapi.State) {
	st.Plugins = []api.PluginDetail{{
		PluginStatus: api.PluginStatus{
			ID: "tg", Package: "@openbee/tg", Source: "npm", Version: "1.0.0", Path: "/p/tg",
			Name: api.LocalizedText{"en": "Telegram"}, State: "not_configured", HasSetup: true,
		},
		Config: map[string]any{"token": "****wxyz"},
		Fields: []api.PluginConfigField{
			{Key: "token", Type: "secret", Required: true},
			{Key: "admin", Type: "string"},
			{Key: "retries", Type: "int"},
			{Key: "tags", Type: "list"},
			{Key: "salt", Type: "string", RandomDefault: true},
		},
	}, {
		PluginStatus: api.PluginStatus{ID: "plain", Source: "path", Path: "/p/plain", State: "disabled"},
		Fields:       []api.PluginConfigField{{Key: "name", Type: "string", Required: true}},
	}}
}

func TestPluginReadAndSettings(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	s.fake.Update(seedPlugin)

	list, err := s.client.Plugins(c)
	if err != nil || len(list) != 2 || list[0].ID != "tg" || list[1].Name["en"] != "plain" {
		t.Fatalf("Plugins = %+v, %v", list, err)
	}
	plain, err := s.client.Plugin(c, "plain")
	if err != nil || plain.Config == nil || plain.Fields == nil {
		t.Fatalf("Plugin(plain) = %+v, %v", plain, err)
	}
	if _, err := s.client.Plugin(c, "ghost"); apiError(t, err).Code != "plugin_not_found" {
		t.Fatalf("missing plugin: %v", err)
	}

	err = s.client.UpdatePluginSettings(c, "tg", api.PluginSettingsUpdate{Config: map[string]any{"admin": "root", "token": "****wxyz", "retries": 3}})
	if err != nil {
		t.Fatalf("UpdatePluginSettings: %v", err)
	}
	tg, _ := s.client.Plugin(c, "tg")
	if tg.Config["token"] != "****wxyz" || tg.Config["admin"] != "root" || tg.Config["retries"] != float64(3) {
		t.Fatalf("config = %v", tg.Config)
	}
	salt, ok := tg.Config["salt"].(string)
	if !ok || len(salt) != 16 {
		t.Fatalf("random default = %v", tg.Config["salt"])
	}

	bot := "  buddy "
	err = s.client.UpdatePluginSettings(c, "tg", api.PluginSettingsUpdate{
		BotName: &bot,
		Config:  map[string]any{"token": "a-brand-new-secret", "admin": nil, "tags": []string{"a", "b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tg, _ = s.client.Plugin(c, "tg")
	if tg.Config["token"] != "****cret" || tg.BotName != "buddy" || tg.Config["salt"] != salt {
		t.Fatalf("after second update = %+v", tg)
	}
	if _, has := tg.Config["admin"]; has {
		t.Fatalf("null did not remove admin: %v", tg.Config)
	}
	if !reflect.DeepEqual(tg.Config["tags"], []any{"a", "b"}) {
		t.Fatalf("tags = %#v", tg.Config["tags"])
	}
	if err := s.client.UpdatePluginSettings(c, "tg", api.PluginSettingsUpdate{Config: map[string]any{"token": ""}}); err != nil {
		t.Fatal(err)
	}
	if tg, _ = s.client.Plugin(c, "tg"); tg.Config["token"] != "****cret" {
		t.Fatalf("empty secret overwrote the stored one: %v", tg.Config["token"])
	}

	for _, tc := range []struct {
		name string
		id   string
		req  api.PluginSettingsUpdate
		code string
	}{
		{"unknown field", "tg", api.PluginSettingsUpdate{Config: map[string]any{"bogus": 1}}, "plugin_unknown_field"},
		{"wrong type", "tg", api.PluginSettingsUpdate{Config: map[string]any{"retries": "many"}}, "plugin_config_invalid"},
		{"missing required", "plain", api.PluginSettingsUpdate{Config: map[string]any{}}, "plugin_config_invalid"},
		{"missing plugin", "ghost", api.PluginSettingsUpdate{}, "plugin_not_found"},
	} {
		err := s.client.UpdatePluginSettings(c, tc.id, tc.req)
		if e := apiError(t, err); e.Code != tc.code {
			t.Errorf("%s: %v (code %q), want %q", tc.name, err, e.Code, tc.code)
		}
	}

	w := mustWorker(t, s, "taken")
	clash := w.Name
	err = s.client.UpdatePluginSettings(c, "tg", api.PluginSettingsUpdate{BotName: &clash})
	if apiError(t, err).Code != "plugin_bot_name_conflict" {
		t.Fatalf("bot name clash: %v", err)
	}
	if _, err := s.client.CreateWorker(c, api.CreateWorkerRequest{Name: "buddy"}); apiError(t, err).Code != "worker_validation" {
		t.Fatalf("worker named like a bot: %v", err)
	}
}

func TestPluginLifecycle(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	s.fake.Update(seedPlugin)

	if err := s.client.EnablePlugin(c, "plain"); apiError(t, err).Code != "plugin_not_configured" {
		t.Fatalf("enable unconfigured: %v", err)
	}
	if err := s.client.RestartPlugin(c, "tg"); apiError(t, err).Code != "plugin_not_configured" {
		t.Fatalf("restart disabled: %v", err)
	}
	if err := s.client.EnablePlugin(c, "tg"); err != nil {
		t.Fatal(err)
	}
	if tg, _ := s.client.Plugin(c, "tg"); !tg.Enabled || tg.State != "running" {
		t.Fatalf("after enable = %+v", tg.PluginStatus)
	}
	if err := s.client.RestartPlugin(c, "tg"); err != nil {
		t.Fatal(err)
	}
	if err := s.client.DisablePlugin(c, "tg"); err != nil {
		t.Fatal(err)
	}
	if tg, _ := s.client.Plugin(c, "tg"); tg.Enabled || tg.State != "disabled" {
		t.Fatalf("after disable = %+v", tg.PluginStatus)
	}
	for _, fn := range []func(context.Context, string) error{s.client.EnablePlugin, s.client.DisablePlugin, s.client.RestartPlugin} {
		if err := fn(c, "ghost"); apiError(t, err).Code != "plugin_not_found" {
			t.Fatalf("lifecycle on missing plugin: %v", err)
		}
	}
}

func TestPluginsRejectAPIKeyHeader(t *testing.T) {
	s := signedIn(t)
	req, _ := http.NewRequestWithContext(ctx(t), http.MethodGet, s.fake.URL+"/api/plugins", nil)
	req.Header.Set("Authorization", "Bearer "+s.tokens.Access())
	req.Header.Set("X-API-Key", "rpc-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
}

func TestPluginInstallAndUninstallOperations(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	s.fake.Update(func(st *fakeapi.State) {
		st.Catalog.Categories = []api.CatalogTag{{ID: "chat", Name: api.LocalizedText{"en": "Chat"}}}
		st.Catalog.Plugins = []api.CatalogPlugin{{
			ID: "slack", Package: "@openbee/slack", Version: "2.0.0", Name: api.LocalizedText{"en": "Slack"}, Installable: true,
		}}
	})

	if _, err := s.client.InstallPlugin(c, "   "); apiError(t, err).Status != 400 {
		t.Fatalf("blank spec: %v", err)
	}
	op, err := s.client.InstallPlugin(c, " @openbee/slack@2.0.0 ")
	if err != nil || op.ID == "" || op.Kind != "install" || op.Spec != "@openbee/slack@2.0.0" || op.Status != "running" || op.CreatedAt.IsZero() {
		t.Fatalf("InstallPlugin = %+v, %v", op, err)
	}
	cat, err := s.client.PluginCatalog(c, true)
	if err != nil || len(cat.Plugins) != 1 || cat.Plugins[0].Installed || cat.Plugins[0].Maintainers == nil || cat.Plugins[0].Tags == nil || cat.Tags == nil {
		t.Fatalf("catalog = %+v, %v", cat, err)
	}
	if calls := s.fake.Calls("GET", "/api/plugins/catalog"); len(calls) != 1 || calls[0].Query != "refresh=1" {
		t.Fatalf("catalog calls = %+v", calls)
	}

	s.fake.FinishOperation(op.ID, true, "")
	ops, err := s.client.PluginOperations(c)
	if err != nil || len(ops) != 1 || ops[0].Status != "succeeded" || ops[0].FinishedAt == nil || ops[0].Error != "" {
		t.Fatalf("operations = %+v, %v", ops, err)
	}
	plugins, _ := s.client.Plugins(c)
	if len(plugins) != 1 || plugins[0].ID != "slack" || plugins[0].Version != "2.0.0" || plugins[0].Package != "@openbee/slack" ||
		plugins[0].Name["en"] != "Slack" || plugins[0].State != "not_configured" {
		t.Fatalf("plugins after install = %+v", plugins)
	}
	cat, _ = s.client.PluginCatalog(c, false)
	if !cat.Plugins[0].Installed || cat.Plugins[0].InstalledVersion != "2.0.0" {
		t.Fatalf("catalog after install = %+v", cat.Plugins[0])
	}

	failed, err := s.client.InstallPlugin(c, "left-pad")
	if err != nil {
		t.Fatal(err)
	}
	s.fake.FinishOperation(failed.ID, false, "npm exploded")
	ops, _ = s.client.PluginOperations(c)
	if len(ops) != 2 || ops[0].ID != failed.ID || ops[0].Status != "failed" || ops[0].Error != "npm exploded" || ops[1].ID != op.ID {
		t.Fatalf("operations newest first = %+v", ops)
	}
	if plugins, _ = s.client.Plugins(c); len(plugins) != 1 {
		t.Fatalf("failed install added a plugin: %+v", plugins)
	}

	if _, err := s.client.UninstallPlugin(c, "ghost"); apiError(t, err).Code != "plugin_not_found" {
		t.Fatalf("uninstall missing: %v", err)
	}
	un, err := s.client.UninstallPlugin(c, "slack")
	if err != nil || un.Kind != "uninstall" || un.PluginID != "slack" || un.Status != "running" {
		t.Fatalf("UninstallPlugin = %+v, %v", un, err)
	}
	if plugins, _ = s.client.Plugins(c); len(plugins) != 1 {
		t.Fatal("plugin removed before the operation finished")
	}
	s.fake.FinishOperation(un.ID, true, "")
	if plugins, _ = s.client.Plugins(c); len(plugins) != 0 {
		t.Fatalf("plugins after uninstall = %+v", plugins)
	}
	if cat, _ = s.client.PluginCatalog(c, false); cat.Plugins[0].Installed {
		t.Fatal("catalog still marks the plugin installed")
	}

	var got api.PluginOperation
	if err := s.client.Do(c, http.MethodGet, "/plugins/operations/"+op.ID, nil, &got); err != nil || got.ID != op.ID {
		t.Fatalf("operation by id = %+v, %v", got, err)
	}
	if err := s.client.Do(c, http.MethodGet, "/plugins/operations/none", nil, &got); apiError(t, err).Code != "plugin_operation_not_found" {
		t.Fatalf("missing operation: %v", err)
	}
}

func TestPluginSetupStream(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	s.fake.Update(seedPlugin)

	if _, err := s.client.StartPluginSetup(c, "plain"); apiError(t, err).Code != "plugin_no_setup" {
		t.Fatalf("setup without step: %v", err)
	}
	if _, err := s.client.StartPluginSetup(c, "ghost"); apiError(t, err).Code != "plugin_not_found" {
		t.Fatalf("setup missing plugin: %v", err)
	}
	sid, err := s.client.StartPluginSetup(c, "tg")
	if err != nil || sid == "" {
		t.Fatalf("StartPluginSetup = %q, %v", sid, err)
	}

	s.fake.SetupEmit(sid, api.SetupEvent{Type: "line", Text: "scan this"})
	s.fake.SetupEmit(sid, api.SetupEvent{Type: "line", Text: "https://example.com/login"})
	stream := openSSE(t, s, api.PluginSetupEventsPath(sid))
	for _, want := range []string{"scan this", "https://example.com/login"} {
		if ev := stream.setupEvent(t); ev.Type != "line" || ev.Text != want {
			t.Fatalf("replayed event = %+v, want %q", ev, want)
		}
	}
	s.fake.SetupEmit(sid, api.SetupEvent{Type: "line", Text: "live line"})
	if ev := stream.setupEvent(t); ev.Text != "live line" {
		t.Fatalf("live event = %+v", ev)
	}

	if err := s.client.PluginSetupInput(c, sid, "123456\nsecond line"); err != nil {
		t.Fatalf("PluginSetupInput: %v", err)
	}
	inputs := s.fake.Calls("POST", "/api/plugins/setup/"+sid+"/input")
	if len(inputs) != 1 || !strings.Contains(string(inputs[0].Body), "123456") {
		t.Fatalf("input calls = %+v", inputs)
	}

	s.fake.SetupEmit(sid, api.SetupEvent{Type: "done", OK: true})
	if ev := stream.setupEvent(t); ev.Type != "done" || !ev.OK {
		t.Fatalf("done event = %+v", ev)
	}
	stream.ended(t)
	s.fake.SetupEmit(sid, api.SetupEvent{Type: "line", Text: "ignored after done"})

	replay := openSSE(t, s, api.PluginSetupEventsPath(sid))
	var types []string
	for i := 0; i < 4; i++ {
		types = append(types, replay.setupEvent(t).Type)
	}
	if !reflect.DeepEqual(types, []string{"line", "line", "line", "done"}) {
		t.Fatalf("finished session replay = %v", types)
	}
	replay.ended(t)

	if err := s.client.PluginSetupInput(c, sid, "late"); apiError(t, err).Code != "plugin_setup_finished" || apiError(t, err).Status != 409 {
		t.Fatalf("input after done: %v", err)
	}
	if err := s.client.PluginSetupInput(c, "nope", "x"); apiError(t, err).Code != "plugin_setup_not_found" {
		t.Fatalf("input unknown session: %v", err)
	}
	if err := s.client.CancelPluginSetup(c, "nope"); apiError(t, err).Code != "plugin_setup_not_found" {
		t.Fatalf("cancel unknown session: %v", err)
	}
	resp, err := s.client.DoRaw(c, mustRequest(t, c, s.fake.URL+"/api/plugins/setup/nope/events"))
	if err == nil {
		resp.Body.Close()
		t.Fatal("events of an unknown session must fail")
	}
	if e := apiError(t, err); e.Status != 404 || e.Code != "plugin_setup_not_found" {
		t.Fatalf("events unknown session: %v", err)
	}
}

func mustRequest(t *testing.T, c context.Context, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(c, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestPluginSetupCancelAndRestart(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	s.fake.Update(seedPlugin)

	first, err := s.client.StartPluginSetup(c, "tg")
	if err != nil {
		t.Fatal(err)
	}
	firstStream := openSSE(t, s, api.PluginSetupEventsPath(first))
	second, err := s.client.StartPluginSetup(c, "tg")
	if err != nil || second == first {
		t.Fatalf("second session = %q, %v", second, err)
	}
	if ev := firstStream.setupEvent(t); ev.Type != "done" || ev.Error != "setup was cancelled" {
		t.Fatalf("previous session event = %+v", ev)
	}
	firstStream.ended(t)

	secondStream := openSSE(t, s, api.PluginSetupEventsPath(second))
	if err := s.client.CancelPluginSetup(c, second); err != nil {
		t.Fatal(err)
	}
	if ev := secondStream.setupEvent(t); ev.Type != "done" || ev.Error != "setup was cancelled" || ev.OK {
		t.Fatalf("cancel event = %+v", ev)
	}
	secondStream.ended(t)
}

func unreadOf(t *testing.T, s *session, cid string) int {
	t.Helper()
	convs, err := s.client.Conversations(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, cv := range convs {
		if cv.ID == cid {
			return cv.Unread
		}
	}
	t.Fatalf("conversation %q not listed", cid)
	return 0
}

func TestReadMarkOnlyMovesForward(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	for _, ts := range []int64{100, 200, 300} {
		s.fake.Reply("bee", api.ChatMessage{TS: ts, Content: "m"})
	}
	steps := []struct {
		name string
		ts   int64
		want int
	}{
		{"mark at 200", 200, 1},
		{"older mark does not raise unread", 100, 1},
		{"same mark again", 200, 1},
		{"newer mark", 300, 0},
		{"older mark after catching up", 150, 0},
	}
	for _, step := range steps {
		if err := s.client.MarkRead(c, "bee", step.ts); err != nil {
			t.Fatal(err)
		}
		if got := unreadOf(t, s, "bee"); got != step.want {
			t.Fatalf("%s: unread = %d, want %d", step.name, got, step.want)
		}
	}
}

func TestReadMarkWithoutTimestampUsesNow(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	s.fake.Reply("bee", api.ChatMessage{TS: 100, Content: "old"})
	if err := s.client.MarkRead(c, "bee", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.client.MarkRead(c, "bee", 50); err != nil {
		t.Fatal(err)
	}
	if got := unreadOf(t, s, "bee"); got != 0 {
		t.Fatalf("unread after now-mark then older mark = %d, want 0", got)
	}

	s.fake.Reply("bee", api.ChatMessage{Content: "after the mark"})
	if got := unreadOf(t, s, "bee"); got != 1 {
		t.Fatalf("unread after a new message = %d, want 1", got)
	}
	s.fake.Reply("bee", api.ChatMessage{TS: 150, Content: "stamped before the mark"})
	if got := unreadOf(t, s, "bee"); got != 1 {
		t.Fatalf("a message older than the mark counted as unread: %d", got)
	}
	if err := s.client.MarkRead(c, "bee", 0); err != nil {
		t.Fatal(err)
	}
	if got := unreadOf(t, s, "bee"); got != 0 {
		t.Fatalf("unread after reading = %d, want 0", got)
	}
}

func TestOwnMessagesDoNotCountAsUnread(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	if _, err := s.client.SendMessage(c, "bee", api.SendMessageRequest{Content: "mine"}); err != nil {
		t.Fatal(err)
	}
	s.fake.Reply("bee", api.ChatMessage{Content: "theirs"})
	if err := s.client.MarkRead(c, "bee", 1); err != nil {
		t.Fatal(err)
	}
	if got := unreadOf(t, s, "bee"); got != 1 {
		t.Fatalf("unread = %d, want 1 (only the reply)", got)
	}
}

func TestSkipSeqCreatesGapOnLiveStream(t *testing.T) {
	s := signedIn(t)
	stream := openChat(t, s)

	s.fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: "w"})
	s.fake.SkipSeq(2)
	s.fake.SkipSeq(0)
	s.fake.SkipSeq(-3)
	s.fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: "w"})

	if first := stream.chatEvent(t); first.Seq != 1 {
		t.Fatalf("first seq = %d, want 1", first.Seq)
	}
	if second := stream.chatEvent(t); second.Seq != 4 {
		t.Fatalf("second seq = %d, want 4", second.Seq)
	}
	stream.quiet(t)
}

func TestEmitWorkerStatusUpdatesWorker(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	w := mustWorker(t, s, "sam")
	other := mustWorker(t, s, "ann")

	s.fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: w.ID, Status: "working"})
	if got, err := s.client.Worker(c, w.ID); err != nil || got.Status != "working" {
		t.Fatalf("Worker = %+v, %v", got, err)
	}
	if got, _ := s.client.Worker(c, other.ID); got.Status != "idle" {
		t.Fatalf("other worker status = %q", got.Status)
	}
	if list, _ := s.client.Workers(c); list[0].Status != "working" || list[1].Status != "idle" {
		t.Fatalf("Workers = %+v", list)
	}

	s.fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: w.ID, Status: "error"})
	if got, _ := s.client.Worker(c, w.ID); got.Status != "error" {
		t.Fatalf("status after error = %q", got.Status)
	}
	s.fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: "ghost", Status: "working"})
	if list, _ := s.client.Workers(c); len(list) != 2 {
		t.Fatalf("unknown worker event changed the list: %+v", list)
	}
}

func TestEmitTaskUpdatesConversationActivity(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	w := mustWorker(t, s, "sam")
	cid := api.WorkerConversation(w.ID)
	activity := func() api.ConversationActivity {
		t.Helper()
		convs, err := s.client.Conversations(c)
		if err != nil {
			t.Fatal(err)
		}
		for _, cv := range convs {
			if cv.ID == cid {
				return cv.Activity
			}
		}
		t.Fatalf("conversation %q not listed", cid)
		return api.ConversationActivity{}
	}

	steps := []struct {
		name string
		ev   api.ChatEvent
		want api.ConversationActivity
	}{
		{"queued", api.ChatEvent{Type: "task", ConversationID: cid, TaskID: "t1", State: "queued", QueuedAhead: 2}, api.ConversationActivity{State: "queued", QueuedAhead: 2}},
		{"running", api.ChatEvent{Type: "task", ConversationID: cid, TaskID: "t1", State: "running"}, api.ConversationActivity{State: "running"}},
		{"completed", api.ChatEvent{Type: "task", ConversationID: cid, TaskID: "t1", State: "completed"}, api.ConversationActivity{State: "idle"}},
		{"running by worker id", api.ChatEvent{Type: "task", WorkerID: w.ID, TaskID: "t2", State: "running"}, api.ConversationActivity{State: "running"}},
		{"failed", api.ChatEvent{Type: "task", ConversationID: cid, TaskID: "t2", State: "failed"}, api.ConversationActivity{State: "idle"}},
		{"cancelled", api.ChatEvent{Type: "task", ConversationID: cid, TaskID: "t3", State: "cancelled"}, api.ConversationActivity{State: "idle"}},
	}
	for _, step := range steps {
		s.fake.Emit(step.ev)
		if got := activity(); got != step.want {
			t.Fatalf("%s: activity = %+v, want %+v", step.name, got, step.want)
		}
	}

	s.fake.Emit(api.ChatEvent{Type: "task", ConversationID: "worker:ghost", State: "running"})
	convs, _ := s.client.Conversations(c)
	if len(convs) != 2 {
		t.Fatalf("task for an unknown conversation created a summary: %+v", convs)
	}
}

func TestEmittedEventsStillReachStreamsAfterApplying(t *testing.T) {
	s := signedIn(t)
	w := mustWorker(t, s, "sam")
	stream := openChat(t, s)
	s.fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: w.ID, Status: "working"})
	if ev := stream.chatEvent(t); ev.Seq != 1 || ev.Status != "working" {
		t.Fatalf("event = %+v", ev)
	}
	if got := s.fake.Snapshot().Workers[0].Status; got != "working" {
		t.Fatalf("Snapshot status = %q", got)
	}
}
