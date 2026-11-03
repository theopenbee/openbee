package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordedRequest struct {
	method string
	uri    string
	auth   string
	body   string
}

func serveCanned(t *testing.T, status int, response string) (*Client, func() recordedRequest) {
	t.Helper()
	var mu sync.Mutex
	var got recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = recordedRequest{method: r.Method, uri: r.RequestURI, auth: r.Header.Get("Authorization"), body: string(data)}
		mu.Unlock()
		writeJSON(w, status, response)
	}))
	t.Cleanup(srv.Close)
	c := newTestClient(t, srv, &fakeTokens{access: "tok", refresh: "r"}, nil)
	return c, func() recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
}

func sameJSON(t *testing.T, got, want string) bool {
	t.Helper()
	if got == "" || want == "" {
		return got == want
	}
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("request body is not JSON: %q", got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("expected body is not JSON: %q", want)
	}
	return reflect.DeepEqual(g, w)
}

func strPtr(s string) *string { return &s }

func int64Ptr(n int64) *int64 { return &n }

func mapPtr(m map[string]string) *map[string]string { return &m }

const workerJSON = `{"id":"w1","name":"Ada","description":"d","constraints":"c","work_dir":"/w","engine":"","engine_args":{},"status":"idle","permission_scopes":"","created_at":1700,"updated_at":1701}`

var workerValue = Worker{ID: "w1", Name: "Ada", Description: "d", Constraints: "c", WorkDir: "/w", EngineArgs: map[string]string{}, Status: "idle", CreatedAt: 1700, UpdatedAt: 1701}

const pluginStatusJSON = `{"id":"telegram","package":"@x/telegram","source":"npm","version":"1.0.0","path":"/p","name":{"en":"Telegram","zh":"电报"},"enabled":true,"bot_name":"tg","state":"running","has_setup":true}`

var pluginStatusValue = PluginStatus{
	ID: "telegram", Package: "@x/telegram", Source: "npm", Version: "1.0.0", Path: "/p",
	Name: LocalizedText{"en": "Telegram", "zh": "电报"}, Enabled: true, BotName: "tg", State: "running", HasSetup: true,
}

const operationJSON = `{"id":"op1","kind":"install","spec":"pkg@1","status":"succeeded","created_at":"2026-10-28T10:00:00Z","finished_at":"2026-10-28T10:00:05Z"}`

func operationValue() PluginOperation {
	created := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	finished := created.Add(5 * time.Second)
	return PluginOperation{ID: "op1", Kind: "install", Spec: "pkg@1", Status: "succeeded", CreatedAt: created, FinishedAt: &finished}
}

func TestEndpoints(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		call     func(c *Client) (any, error)
		method   string
		uri      string
		body     string
		public   bool
		status   int
		response string
		want     any
	}{
		{
			name:   "config",
			call:   func(c *Client) (any, error) { return c.Config(ctx) },
			method: "GET", uri: "/api/config", public: true,
			status: 200, response: `{"language":"zh","enabled_engines":["claude","pi"]}`,
			want: Config{Language: "zh", EnabledEngines: []string{"claude", "pi"}},
		},
		{
			name:   "config with engine plugins",
			call:   func(c *Client) (any, error) { return c.Config(ctx) },
			method: "GET", uri: "/api/config", public: true,
			status:   200,
			response: `{"language":"zh","enabled_engines":["pi"],"engines":[{"id":"pi","name":{"en":"Pi","zh":"Pi 引擎"}}]}`,
			want: Config{
				Language:       "zh",
				EnabledEngines: []string{"pi"},
				Engines:        []EngineInfo{{ID: "pi", Name: LocalizedText{"en": "Pi", "zh": "Pi 引擎"}}},
			},
		},
		{
			name:   "config without engines",
			call:   func(c *Client) (any, error) { return c.Config(ctx) },
			method: "GET", uri: "/api/config", public: true,
			status:   200,
			response: `{"language":"en","enabled_engines":[],"engines":[]}`,
			want:     Config{Language: "en", EnabledEngines: []string{}, Engines: []EngineInfo{}},
		},
		{
			name:   "setup status",
			call:   func(c *Client) (any, error) { return c.SetupStatus(ctx) },
			method: "GET", uri: "/api/setup/status", public: true,
			status: 200, response: `{"initialized":true}`,
			want: SetupStatus{Initialized: true},
		},
		{
			name: "setup",
			call: func(c *Client) (any, error) {
				return c.Setup(ctx, SetupRequest{Username: "admin", Password: "secret1", DisplayName: "Admin"})
			},
			method: "POST", uri: "/api/setup", public: true,
			body:   `{"username":"admin","password":"secret1","display_name":"Admin"}`,
			status: 200, response: `{"access_token":"a","refresh_token":"r","expires_in":7200}`,
			want: TokenPair{AccessToken: "a", RefreshToken: "r", ExpiresIn: 7200},
		},
		{
			name: "setup without display name",
			call: func(c *Client) (any, error) {
				return c.Setup(ctx, SetupRequest{Username: "admin", Password: "secret1"})
			},
			method: "POST", uri: "/api/setup", public: true,
			body:   `{"username":"admin","password":"secret1"}`,
			status: 200, response: `{"access_token":"a","refresh_token":"r","expires_in":7200}`,
			want: TokenPair{AccessToken: "a", RefreshToken: "r", ExpiresIn: 7200},
		},
		{
			name:   "login",
			call:   func(c *Client) (any, error) { return c.Login(ctx, "u", "p") },
			method: "POST", uri: "/api/auth/login", public: true,
			body:   `{"username":"u","password":"p"}`,
			status: 200, response: `{"access_token":"a","refresh_token":"r","expires_in":7200}`,
			want: TokenPair{AccessToken: "a", RefreshToken: "r", ExpiresIn: 7200},
		},
		{
			name:   "change password",
			call:   func(c *Client) (any, error) { return nil, c.ChangePassword(ctx, "old", "new") },
			method: "POST", uri: "/api/me/password",
			body:   `{"old_password":"old","new_password":"new"}`,
			status: 204,
		},
		{
			name:   "workers",
			call:   func(c *Client) (any, error) { return c.Workers(ctx) },
			method: "GET", uri: "/api/workers",
			status: 200, response: `[` + workerJSON + `]`,
			want: []Worker{workerValue},
		},
		{
			name:   "workers null",
			call:   func(c *Client) (any, error) { return c.Workers(ctx) },
			method: "GET", uri: "/api/workers",
			status: 200, response: `null`,
			want: []Worker{},
		},
		{
			name:   "worker",
			call:   func(c *Client) (any, error) { return c.Worker(ctx, "w1") },
			method: "GET", uri: "/api/workers/w1",
			status: 200, response: workerJSON,
			want: workerValue,
		},
		{
			name: "create worker",
			call: func(c *Client) (any, error) {
				return c.CreateWorker(ctx, CreateWorkerRequest{
					Name: "Ada", Engine: "claude", Description: "d", Constraints: "c", WorkDir: "/w",
					PermissionScopes: "read:workers", EngineArgs: map[string]string{"claude": "--x"},
				})
			},
			method: "POST", uri: "/api/workers",
			body:   `{"name":"Ada","engine":"claude","description":"d","constraints":"c","work_dir":"/w","permission_scopes":"read:workers","engine_args":{"claude":"--x"}}`,
			status: 201, response: workerJSON,
			want: workerValue,
		},
		{
			name:   "create worker minimal",
			call:   func(c *Client) (any, error) { return c.CreateWorker(ctx, CreateWorkerRequest{Name: "Ada"}) },
			method: "POST", uri: "/api/workers",
			body:   `{"name":"Ada","description":""}`,
			status: 201, response: workerJSON,
			want: workerValue,
		},
		{
			name: "update worker constraints only",
			call: func(c *Client) (any, error) {
				return c.UpdateWorker(ctx, "w1", UpdateWorkerRequest{Constraints: strPtr("be brief")})
			},
			method: "PUT", uri: "/api/workers/w1",
			body:   `{"constraints":"be brief"}`,
			status: 200, response: workerJSON,
			want: workerValue,
		},
		{
			name: "update worker clears engine args",
			call: func(c *Client) (any, error) {
				return c.UpdateWorker(ctx, "w1", UpdateWorkerRequest{Engine: strPtr(""), EngineArgs: mapPtr(map[string]string{})})
			},
			method: "PUT", uri: "/api/workers/w1",
			body:   `{"engine":"","engine_args":{}}`,
			status: 200, response: workerJSON,
			want: workerValue,
		},
		{
			name: "update worker all fields",
			call: func(c *Client) (any, error) {
				return c.UpdateWorker(ctx, "w1", UpdateWorkerRequest{
					Name: strPtr("Bo"), Description: strPtr("dd"), Constraints: strPtr("cc"), WorkDir: strPtr("/x"),
					PermissionScopes: strPtr("read:tasks"), Engine: strPtr("pi"), EngineArgs: mapPtr(map[string]string{"pi": ""}),
				})
			},
			method: "PUT", uri: "/api/workers/w1",
			body:   `{"name":"Bo","description":"dd","constraints":"cc","work_dir":"/x","permission_scopes":"read:tasks","engine":"pi","engine_args":{"pi":""}}`,
			status: 200, response: workerJSON,
			want: workerValue,
		},
		{
			name:   "delete worker with work dir",
			call:   func(c *Client) (any, error) { return nil, c.DeleteWorker(ctx, "w1", true) },
			method: "DELETE", uri: "/api/workers/w1?delete_work_dir=true",
			status: 200, response: `{"status":"deleted"}`,
		},
		{
			name:   "delete worker keeping work dir",
			call:   func(c *Client) (any, error) { return nil, c.DeleteWorker(ctx, "w1", false) },
			method: "DELETE", uri: "/api/workers/w1",
			status: 200, response: `{"status":"deleted"}`,
		},
		{
			name:   "random worker name",
			call:   func(c *Client) (any, error) { return c.RandomWorkerName(ctx) },
			method: "GET", uri: "/api/workers/random-name",
			status: 200, response: `{"name":"Nova"}`,
			want: RandomName{Name: "Nova"},
		},
		{
			name:   "random worker name exhausted",
			call:   func(c *Client) (any, error) { return c.RandomWorkerName(ctx) },
			method: "GET", uri: "/api/workers/random-name",
			status: 200, response: `{"exhausted":true}`,
			want: RandomName{Exhausted: true},
		},
		{
			name:   "conversations",
			call:   func(c *Client) (any, error) { return c.Conversations(ctx) },
			method: "GET", uri: "/api/local/conversations",
			status: 200,
			response: `[{"id":"bee","kind":"bee","unread":0,"activity":{"state":"idle"}},` +
				`{"id":"worker:w1","kind":"worker","worker_id":"w1","last_message":{"sender":{"type":"worker","id":"w1"},"kind":"handoff","preview":"hi","handoff_worker_id":"w1","ts":1700},"unread":2,"activity":{"state":"queued","queued_ahead":3}}]`,
			want: []ConversationSummary{
				{ID: "bee", Kind: "bee", Activity: ConversationActivity{State: "idle"}},
				{
					ID: "worker:w1", Kind: "worker", WorkerID: "w1", Unread: 2,
					LastMessage: &LastMessage{Sender: ChatSender{Type: "worker", ID: "w1"}, Kind: "handoff", Preview: "hi", HandoffWorkerID: "w1", TS: 1700},
					Activity:    ConversationActivity{State: "queued", QueuedAhead: 3},
				},
			},
		},
		{
			name:   "conversations null",
			call:   func(c *Client) (any, error) { return c.Conversations(ctx) },
			method: "GET", uri: "/api/local/conversations",
			status: 200, response: `null`,
			want: []ConversationSummary{},
		},
		{
			name:   "messages with paging",
			call:   func(c *Client) (any, error) { return c.Messages(ctx, "worker:w1", 1700, 50) },
			method: "GET", uri: "/api/local/conversations/worker%3Aw1/messages?before=1700&limit=50",
			status: 200,
			response: `{"messages":[` +
				`{"id":"m1","sender":{"type":"user"},"kind":"message","content":"hi","media_paths":["a.png"],"ts":1600},` +
				`{"id":"m2","sender":{"type":"bee"},"kind":"handoff","content":"do it","ts":1650,"handoff":{"worker_id":"w1","task_id":"t1","task_type":"countdown","scheduled_at":1800,"conversation_id":"worker:w1"}}],"has_more":true}`,
			want: MessagesPage{
				Messages: []ChatMessage{
					{ID: "m1", Sender: ChatSender{Type: "user"}, Kind: "message", Content: "hi", MediaPaths: []string{"a.png"}, TS: 1600},
					{
						ID: "m2", Sender: ChatSender{Type: "bee"}, Kind: "handoff", Content: "do it", TS: 1650,
						Handoff: &HandoffInfo{WorkerID: "w1", TaskID: "t1", TaskType: "countdown", ScheduledAt: int64Ptr(1800), ConversationID: "worker:w1"},
					},
				},
				HasMore: true,
			},
		},
		{
			name:   "messages latest page",
			call:   func(c *Client) (any, error) { return c.Messages(ctx, "bee", 0, 50) },
			method: "GET", uri: "/api/local/conversations/bee/messages?limit=50",
			status: 200, response: `{"messages":null,"has_more":false}`,
			want: MessagesPage{Messages: []ChatMessage{}},
		},
		{
			name: "send message",
			call: func(c *Client) (any, error) {
				return c.SendMessage(ctx, "worker:w1", SendMessageRequest{ID: "m1", Content: "hi", MediaPaths: []string{"a.png"}})
			},
			method: "POST", uri: "/api/local/conversations/worker%3Aw1/messages",
			body:   `{"id":"m1","content":"hi","media_paths":["a.png"]}`,
			status: 202, response: `{"id":"m1","ts":1700}`,
			want: SendMessageResponse{ID: "m1", TS: 1700},
		},
		{
			name: "send attachment only message",
			call: func(c *Client) (any, error) {
				return c.SendMessage(ctx, "bee", SendMessageRequest{ID: "m1", MediaPaths: []string{"a.png"}})
			},
			method: "POST", uri: "/api/local/conversations/bee/messages",
			body:   `{"id":"m1","content":" ","media_paths":["a.png"]}`,
			status: 202, response: `{"id":"m1","ts":1700}`,
			want: SendMessageResponse{ID: "m1", TS: 1700},
		},
		{
			name:   "mark read with timestamp",
			call:   func(c *Client) (any, error) { return nil, c.MarkRead(ctx, "worker:w1", 1700) },
			method: "POST", uri: "/api/local/conversations/worker%3Aw1/read",
			body:   `{"ts":1700}`,
			status: 204,
		},
		{
			name:   "mark read now",
			call:   func(c *Client) (any, error) { return nil, c.MarkRead(ctx, "bee", 0) },
			method: "POST", uri: "/api/local/conversations/bee/read",
			body:   `{}`,
			status: 204,
		},
		{
			name:   "clear conversation forced",
			call:   func(c *Client) (any, error) { return nil, c.ClearConversation(ctx, "worker:w1", true) },
			method: "POST", uri: "/api/local/conversations/worker%3Aw1/clear",
			body:   `{"force":true}`,
			status: 204,
		},
		{
			name:   "clear conversation",
			call:   func(c *Client) (any, error) { return nil, c.ClearConversation(ctx, "bee", false) },
			method: "POST", uri: "/api/local/conversations/bee/clear",
			status: 204,
		},
		{
			name:   "stop conversation",
			call:   func(c *Client) (any, error) { return nil, c.StopConversation(ctx, "worker:w1") },
			method: "POST", uri: "/api/local/conversations/worker%3Aw1/stop",
			status: 204,
		},
		{
			name:   "envs by worker",
			call:   func(c *Client) (any, error) { return c.Envs(ctx, "worker", "w1") },
			method: "GET", uri: "/api/envs?scope=worker&scope_id=w1",
			status:   200,
			response: `[{"id":"e1","scope":"worker","scope_id":"w1","key":"TOKEN","masked":"abcd****wxyz","created_at":1,"updated_at":2}]`,
			want:     []EnvConfig{{ID: "e1", Scope: "worker", ScopeID: strPtr("w1"), Key: "TOKEN", Masked: "abcd****wxyz", CreatedAt: 1, UpdatedAt: 2}},
		},
		{
			name:   "envs global",
			call:   func(c *Client) (any, error) { return c.Envs(ctx, "global", "") },
			method: "GET", uri: "/api/envs?scope=global",
			status: 200, response: `[{"id":"e2","scope":"global","scope_id":null,"key":"K","masked":"****","created_at":1,"updated_at":2}]`,
			want: []EnvConfig{{ID: "e2", Scope: "global", Key: "K", Masked: "****", CreatedAt: 1, UpdatedAt: 2}},
		},
		{
			name:   "envs null",
			call:   func(c *Client) (any, error) { return c.Envs(ctx, "bee", DefaultBeeID) },
			method: "GET", uri: "/api/envs?scope=bee&scope_id=default",
			status: 200, response: `null`,
			want: []EnvConfig{},
		},
		{
			name: "create env",
			call: func(c *Client) (any, error) {
				return c.CreateEnv(ctx, CreateEnvRequest{Scope: "worker", ScopeID: "w1", Key: "K", Value: "v"})
			},
			method: "POST", uri: "/api/envs",
			body:   `{"scope":"worker","scope_id":"w1","key":"K","value":"v"}`,
			status: 201, response: `{"id":"e1","scope":"worker","scope_id":"w1","key":"K","masked":"****","created_at":1,"updated_at":2}`,
			want: EnvConfig{ID: "e1", Scope: "worker", ScopeID: strPtr("w1"), Key: "K", Masked: "****", CreatedAt: 1, UpdatedAt: 2},
		},
		{
			name: "create global env",
			call: func(c *Client) (any, error) {
				return c.CreateEnv(ctx, CreateEnvRequest{Scope: "global", Key: "K", Value: "v"})
			},
			method: "POST", uri: "/api/envs",
			body:   `{"scope":"global","key":"K","value":"v"}`,
			status: 201, response: `{"id":"e1","scope":"global","scope_id":null,"key":"K","masked":"****","created_at":1,"updated_at":2}`,
			want: EnvConfig{ID: "e1", Scope: "global", Key: "K", Masked: "****", CreatedAt: 1, UpdatedAt: 2},
		},
		{
			name:   "update env",
			call:   func(c *Client) (any, error) { return nil, c.UpdateEnv(ctx, "e1", "secret") },
			method: "PUT", uri: "/api/envs/e1",
			body:   `{"value":"secret"}`,
			status: 200, response: `{"ok":true}`,
		},
		{
			name:   "delete env",
			call:   func(c *Client) (any, error) { return nil, c.DeleteEnv(ctx, "e1") },
			method: "DELETE", uri: "/api/envs/e1",
			status: 204,
		},
		{
			name:   "system configs",
			call:   func(c *Client) (any, error) { return c.SystemConfigs(ctx) },
			method: "GET", uri: "/api/system-configs",
			status: 200, response: `{"default_engine":"claude","engine_args_global":"","engine_args_bee":"{\"pi\":\"--x\"}"}`,
			want: map[string]string{"default_engine": "claude", "engine_args_global": "", "engine_args_bee": `{"pi":"--x"}`},
		},
		{
			name:   "set system config",
			call:   func(c *Client) (any, error) { return nil, c.SetSystemConfig(ctx, "default_engine", "") },
			method: "PUT", uri: "/api/system-configs/default_engine",
			body:   `{"value":""}`,
			status: 200, response: `{"ok":true}`,
		},
		{
			name:   "plugins",
			call:   func(c *Client) (any, error) { return c.Plugins(ctx) },
			method: "GET", uri: "/api/plugins",
			status: 200, response: `{"plugins":[` + pluginStatusJSON + `]}`,
			want: []PluginStatus{pluginStatusValue},
		},
		{
			name:   "plugins null",
			call:   func(c *Client) (any, error) { return c.Plugins(ctx) },
			method: "GET", uri: "/api/plugins",
			status: 200, response: `{"plugins":null}`,
			want: []PluginStatus{},
		},
		{
			name:   "plugin detail",
			call:   func(c *Client) (any, error) { return c.Plugin(ctx, "telegram") },
			method: "GET", uri: "/api/plugins/telegram",
			status: 200,
			response: `{"id":"telegram","source":"path","path":"/p","name":{"en":"telegram"},"enabled":false,"bot_name":"","state":"not_configured","has_setup":false,` +
				`"config":{"token":"****abcd","port":8080},"fields":[{"key":"token","type":"secret","required":true,"label":{"en":"Token"},"help":{"en":"From BotFather"}},{"key":"port","type":"int","default":8080,"random_default":true}],"setup_label":{"en":"Log in"}}`,
			want: PluginDetail{
				PluginStatus: PluginStatus{ID: "telegram", Source: "path", Path: "/p", Name: LocalizedText{"en": "telegram"}, State: "not_configured"},
				Config:       map[string]any{"token": "****abcd", "port": float64(8080)},
				Fields: []PluginConfigField{
					{Key: "token", Type: "secret", Required: true, Label: LocalizedText{"en": "Token"}, Help: LocalizedText{"en": "From BotFather"}},
					{Key: "port", Type: "int", Default: float64(8080), RandomDefault: true},
				},
				SetupLabel: LocalizedText{"en": "Log in"},
			},
		},
		{
			name:   "plugin id is escaped",
			call:   func(c *Client) (any, error) { return nil, c.EnablePlugin(ctx, "a b/c") },
			method: "POST", uri: "/api/plugins/a%20b%2Fc/enable",
			status: 204,
		},
		{
			name:   "plugin catalog",
			call:   func(c *Client) (any, error) { return c.PluginCatalog(ctx, false) },
			method: "GET", uri: "/api/plugins/catalog",
			status: 200,
			response: `{"schema_version":1,"categories":[{"id":"im","name":{"en":"IM"}}],"tags":[{"id":"chat","name":{"en":"Chat"}}],"plugins":[` +
				`{"category":"im","id":"telegram","package":"@x/telegram","version":"1.2.0","official":true,"name":{"en":"Telegram"},"protocol_version":1,"license":"MIT","repository":"https://r","maintainers":["a"],"tags":["chat"],"integrity":"sha512-x","tarball":"https://t","installed":true,"installed_version":"1.0.0","update_available":true,"installable":true}]}`,
			want: PluginCatalog{
				SchemaVersion: 1,
				Categories:    []CatalogTag{{ID: "im", Name: LocalizedText{"en": "IM"}}},
				Tags:          []CatalogTag{{ID: "chat", Name: LocalizedText{"en": "Chat"}}},
				Plugins: []CatalogPlugin{{
					Category: "im", ID: "telegram", Package: "@x/telegram", Version: "1.2.0", Official: true,
					Name: LocalizedText{"en": "Telegram"}, ProtocolVersion: 1, License: "MIT", Repository: "https://r",
					Maintainers: []string{"a"}, Tags: []string{"chat"}, Integrity: "sha512-x", Tarball: "https://t",
					Installed: true, InstalledVersion: "1.0.0", UpdateAvailable: true, Installable: true,
				}},
			},
		},
		{
			name:   "plugin catalog refresh",
			call:   func(c *Client) (any, error) { return c.PluginCatalog(ctx, true) },
			method: "GET", uri: "/api/plugins/catalog?refresh=1",
			status: 200, response: `{"schema_version":1,"categories":[],"tags":[],"plugins":[]}`,
			want: PluginCatalog{SchemaVersion: 1, Categories: []CatalogTag{}, Tags: []CatalogTag{}, Plugins: []CatalogPlugin{}},
		},
		{
			name:   "install plugin",
			call:   func(c *Client) (any, error) { return c.InstallPlugin(ctx, "pkg@1") },
			method: "POST", uri: "/api/plugins/install",
			body:   `{"spec":"pkg@1"}`,
			status: 202, response: operationJSON,
			want: operationValue(),
		},
		{
			name:   "plugin operations",
			call:   func(c *Client) (any, error) { return c.PluginOperations(ctx) },
			method: "GET", uri: "/api/plugins/operations",
			status: 200, response: `{"operations":[` + operationJSON + `]}`,
			want: []PluginOperation{operationValue()},
		},
		{
			name:   "plugin operations null",
			call:   func(c *Client) (any, error) { return c.PluginOperations(ctx) },
			method: "GET", uri: "/api/plugins/operations",
			status: 200, response: `{"operations":null}`,
			want: []PluginOperation{},
		},
		{
			name: "update plugin settings",
			call: func(c *Client) (any, error) {
				return nil, c.UpdatePluginSettings(ctx, "telegram", PluginSettingsUpdate{
					Config:  map[string]any{"token": "abc", "port": 8080, "tags": []string{"a"}, "old": nil},
					BotName: strPtr("tg"),
				})
			},
			method: "PUT", uri: "/api/plugins/telegram/settings",
			body:   `{"config":{"old":null,"port":8080,"tags":["a"],"token":"abc"},"bot_name":"tg"}`,
			status: 204,
		},
		{
			name: "update plugin settings without config",
			call: func(c *Client) (any, error) {
				return nil, c.UpdatePluginSettings(ctx, "telegram", PluginSettingsUpdate{BotName: strPtr("tg")})
			},
			method: "PUT", uri: "/api/plugins/telegram/settings",
			body:   `{"config":{},"bot_name":"tg"}`,
			status: 204,
		},
		{
			name:   "enable plugin",
			call:   func(c *Client) (any, error) { return nil, c.EnablePlugin(ctx, "telegram") },
			method: "POST", uri: "/api/plugins/telegram/enable",
			status: 204,
		},
		{
			name:   "disable plugin",
			call:   func(c *Client) (any, error) { return nil, c.DisablePlugin(ctx, "telegram") },
			method: "POST", uri: "/api/plugins/telegram/disable",
			status: 204,
		},
		{
			name:   "restart plugin",
			call:   func(c *Client) (any, error) { return nil, c.RestartPlugin(ctx, "telegram") },
			method: "POST", uri: "/api/plugins/telegram/restart",
			status: 204,
		},
		{
			name:   "uninstall plugin",
			call:   func(c *Client) (any, error) { return c.UninstallPlugin(ctx, "telegram") },
			method: "DELETE", uri: "/api/plugins/telegram",
			status: 202, response: `{"id":"op2","kind":"uninstall","plugin_id":"telegram","status":"queued","created_at":"2026-10-28T10:00:00Z"}`,
			want: PluginOperation{ID: "op2", Kind: "uninstall", PluginID: "telegram", Status: "queued", CreatedAt: time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)},
		},
		{
			name:   "start plugin setup",
			call:   func(c *Client) (any, error) { return c.StartPluginSetup(ctx, "wechat") },
			method: "POST", uri: "/api/plugins/wechat/setup",
			status: 201, response: `{"id":"sid-1"}`,
			want: "sid-1",
		},
		{
			name:   "plugin setup input",
			call:   func(c *Client) (any, error) { return nil, c.PluginSetupInput(ctx, "sid-1", "123456") },
			method: "POST", uri: "/api/plugins/setup/sid-1/input",
			body:   `{"text":"123456"}`,
			status: 204,
		},
		{
			name:   "cancel plugin setup",
			call:   func(c *Client) (any, error) { return nil, c.CancelPluginSetup(ctx, "sid-1") },
			method: "DELETE", uri: "/api/plugins/setup/sid-1",
			status: 204,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, last := serveCanned(t, tc.status, tc.response)
			got, err := tc.call(c)
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			req := last()
			if req.method != tc.method || req.uri != tc.uri {
				t.Fatalf("request = %s %s, want %s %s", req.method, req.uri, tc.method, tc.uri)
			}
			if !sameJSON(t, req.body, tc.body) {
				t.Fatalf("body = %q, want %q", req.body, tc.body)
			}
			wantAuth := "Bearer tok"
			if tc.public {
				wantAuth = ""
			}
			if req.auth != wantAuth {
				t.Fatalf("Authorization = %q, want %q", req.auth, wantAuth)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("result = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestConversationHelpers(t *testing.T) {
	if BeeConversation != "bee" || DefaultBeeID != "default" {
		t.Fatalf("constants = %q, %q", BeeConversation, DefaultBeeID)
	}
	if got := WorkerConversation("w1"); got != "worker:w1" {
		t.Fatalf("WorkerConversation = %q", got)
	}
	cases := []struct {
		cid    string
		wantID string
		wantOK bool
	}{
		{"worker:w1", "w1", true},
		{"bee", "", false},
		{"worker:", "", false},
		{"worker:a:b", "", false},
		{"other:w1", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		id, ok := ConversationWorkerID(tc.cid)
		if id != tc.wantID || ok != tc.wantOK {
			t.Errorf("ConversationWorkerID(%q) = %q, %v; want %q, %v", tc.cid, id, ok, tc.wantID, tc.wantOK)
		}
	}
}

func TestStreamPaths(t *testing.T) {
	if ChatStreamPath != "/local/stream" {
		t.Fatalf("ChatStreamPath = %q", ChatStreamPath)
	}
	if got := PluginSetupEventsPath("s 1"); got != "/plugins/setup/s%201/events" {
		t.Fatalf("PluginSetupEventsPath = %q", got)
	}
}

func TestActiveTasksFromConflict(t *testing.T) {
	c, _ := serveCanned(t, 409, `{"error":"x","active_tasks":[{"id":"t","instruction":"i","type":"scheduled"}]}`)
	err := c.ClearConversation(context.Background(), "worker:w1", false)
	if !IsStatus(err, http.StatusConflict) {
		t.Fatalf("err = %v", err)
	}
	got := ActiveTasks(err)
	want := []ActiveTask{{ID: "t", Instruction: "i", Type: "scheduled"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveTasks = %#v, want %#v", got, want)
	}
	if ActiveTasks(nil) != nil {
		t.Fatal("ActiveTasks(nil) is not nil")
	}
	if ActiveTasks(errors.New("boom")) != nil {
		t.Fatal("ActiveTasks(plain error) is not nil")
	}
	other := &Error{Status: 400, Details: map[string]any{"active_tasks": []any{map[string]any{"id": "t"}}}}
	if ActiveTasks(other) != nil {
		t.Fatal("ActiveTasks of a non-409 error is not nil")
	}
	empty := &Error{Status: 409, Details: map[string]any{"error": "active tasks"}}
	if ActiveTasks(empty) != nil {
		t.Fatal("ActiveTasks without active_tasks is not nil")
	}
}

func TestEndpointErrorsCarryServerMessage(t *testing.T) {
	c, _ := serveCanned(t, 404, `{"error":"worker not found","code":"worker_not_found"}`)
	_, err := c.Worker(context.Background(), "nope")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != 404 || apiErr.Code != "worker_not_found" {
		t.Fatalf("err = %v", err)
	}
}

func TestChatEventDecoding(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want ChatEvent
	}{
		{
			name: "message",
			raw:  `{"type":"message","seq":7,"conversation_id":"worker:w1","message":{"id":"m1","sender":{"type":"worker","id":"w1"},"kind":"message","content":"done","ts":1700}}`,
			want: ChatEvent{
				Type: "message", Seq: 7, ConversationID: "worker:w1",
				Message: &ChatMessage{ID: "m1", Sender: ChatSender{Type: "worker", ID: "w1"}, Kind: "message", Content: "done", TS: 1700},
			},
		},
		{
			name: "worker status",
			raw:  `{"type":"worker_status","seq":8,"worker_id":"w1","status":"working"}`,
			want: ChatEvent{Type: "worker_status", Seq: 8, WorkerID: "w1", Status: "working"},
		},
		{
			name: "task",
			raw:  `{"type":"task","seq":9,"conversation_id":"worker:w1","task_id":"t1","worker_id":"w1","state":"queued","queued_ahead":2}`,
			want: ChatEvent{Type: "task", Seq: 9, ConversationID: "worker:w1", TaskID: "t1", WorkerID: "w1", State: "queued", QueuedAhead: 2},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got ChatEvent
			if err := json.Unmarshal([]byte(tc.raw), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("event = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestSetupEventDecoding(t *testing.T) {
	var line, done, failed SetupEvent
	for raw, dst := range map[string]*SetupEvent{
		`{"type":"line","text":"scan the code"}`:    &line,
		`{"type":"done","ok":true}`:                 &done,
		`{"type":"done","error":"setup timed out"}`: &failed,
	} {
		if err := json.Unmarshal([]byte(raw), dst); err != nil {
			t.Fatal(err)
		}
	}
	if line != (SetupEvent{Type: "line", Text: "scan the code"}) {
		t.Fatalf("line = %+v", line)
	}
	if done != (SetupEvent{Type: "done", OK: true}) {
		t.Fatalf("done = %+v", done)
	}
	if failed != (SetupEvent{Type: "done", Error: "setup timed out"}) {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestUploadMedia(t *testing.T) {
	type upload struct {
		auth     string
		field    string
		filename string
		content  string
	}
	var mu sync.Mutex
	var uploads []upload
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"access_token":"new"}`)
	})
	mux.HandleFunc("/api/local/media", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
			writeJSON(w, 400, `{"error":"bad"}`)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			writeJSON(w, 400, `{"error":"missing 'file' field"}`)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		mu.Lock()
		uploads = append(uploads, upload{auth: r.Header.Get("Authorization"), field: "file", filename: header.Filename, content: string(data)})
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer new" {
			writeJSON(w, 401, `{"error":"unauthorized"}`)
			return
		}
		writeJSON(w, 200, `{"path":"7c1e_photo.png"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv, &fakeTokens{access: "old", refresh: "r1"}, nil)
	path, err := c.UploadMedia(context.Background(), "photo.png", strings.NewReader("PNGDATA"))
	if err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
	if path != "7c1e_photo.png" {
		t.Fatalf("path = %q", path)
	}
	mu.Lock()
	got := append([]upload(nil), uploads...)
	mu.Unlock()
	want := []upload{
		{auth: "Bearer old", field: "file", filename: "photo.png", content: "PNGDATA"},
		{auth: "Bearer new", field: "file", filename: "photo.png", content: "PNGDATA"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uploads = %+v, want %+v", got, want)
	}
}

func TestUploadMediaRejectsOversizedAndNamelessFiles(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		writeJSON(w, 200, `{"path":"x"}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv, &fakeTokens{access: "tok", refresh: "r"}, nil)

	old := maxUploadSize
	maxUploadSize = 8
	t.Cleanup(func() { maxUploadSize = old })

	if _, err := c.UploadMedia(context.Background(), "big.bin", bytes.NewReader(make([]byte, 9))); err == nil {
		t.Fatal("expected an error above the size cap")
	}
	if path, err := c.UploadMedia(context.Background(), "ok.bin", bytes.NewReader(make([]byte, 8))); err != nil || path != "x" {
		t.Fatalf("at the cap: %q, %v", path, err)
	}
	if _, err := c.UploadMedia(context.Background(), "", strings.NewReader("x")); err == nil {
		t.Fatal("expected an error for an empty file name")
	}
	if hits != 1 {
		t.Fatalf("server hits = %d, want 1", hits)
	}
}

func TestUploadMediaServerError(t *testing.T) {
	c, _ := serveCanned(t, 400, `{"error":"missing 'file' field"}`)
	_, err := c.UploadMedia(context.Background(), "a.txt", strings.NewReader("x"))
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != 400 || apiErr.Message != "missing 'file' field" {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadMedia(t *testing.T) {
	var mu sync.Mutex
	var uris []string
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		uris = append(uris, r.RequestURI)
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		if strings.HasSuffix(r.RequestURI, "missing.png") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = io.WriteString(w, "PNGBYTES")
	}))
	defer srv.Close()
	c := newTestClient(t, srv, &fakeTokens{access: "tok", refresh: "r"}, nil)

	cases := []struct {
		path    string
		wantURI string
	}{
		{"7c1e_my photo.png", "/api/local/media/7c1e_my%20photo.png"},
		{"/home/u/local-uploads/u1/7c1e_a.png", "/api/local/media/7c1e_a.png"},
		{`C:\uploads\7c1e_b.png`, "/api/local/media/7c1e_b.png"},
		{"7c1e_a?b#c.png", "/api/local/media/7c1e_a%3Fb%23c.png"},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		ct, err := c.DownloadMedia(context.Background(), tc.path, &buf)
		if err != nil {
			t.Fatalf("DownloadMedia(%q): %v", tc.path, err)
		}
		if ct != "image/png" || buf.String() != "PNGBYTES" {
			t.Fatalf("DownloadMedia(%q) = %q, %q", tc.path, ct, buf.String())
		}
		mu.Lock()
		gotURI, gotAuth := uris[len(uris)-1], auths[len(auths)-1]
		mu.Unlock()
		if gotURI != tc.wantURI || gotAuth != "Bearer tok" {
			t.Fatalf("DownloadMedia(%q) hit %q with %q, want %q", tc.path, gotURI, gotAuth, tc.wantURI)
		}
	}

	var buf bytes.Buffer
	_, err := c.DownloadMedia(context.Background(), "missing.png", &buf)
	if !IsStatus(err, http.StatusNotFound) || buf.Len() != 0 {
		t.Fatalf("missing file: err = %v, wrote %d bytes", err, buf.Len())
	}

	mu.Lock()
	before := len(uris)
	mu.Unlock()
	for _, bad := range []string{"", "dir/", "..", "."} {
		if _, err := c.DownloadMedia(context.Background(), bad, io.Discard); err == nil {
			t.Errorf("DownloadMedia(%q) returned no error", bad)
		}
	}
	mu.Lock()
	after := len(uris)
	mu.Unlock()
	if after != before {
		t.Fatalf("invalid paths reached the server (%d requests)", after-before)
	}
}
