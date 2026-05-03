# Linear Platform Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Linear platform receiver/sender so issues with the `openbee` label (and their comments) flow through the existing message-ingest pipeline as inbound messages, with bee/worker replies posted back as Linear comments.

**Architecture:** New `internal/platform/linear` package that mirrors `internal/platform/telegram` (Platform / Receiver / Sender). v0 uses polling (not webhooks). A self-written GraphQL client talks to `https://api.linear.app/graphql`. The `lastSyncAt` cursor is persisted in `bee_system_configs`.

**Tech Stack:** Go, standard library `net/http` + `encoding/json` for the GraphQL client, existing `store.SystemConfigStore` for cursor persistence, `httptest` for unit tests.

**Spec:** `docs/superpowers/specs/2026-05-03-linear-platform-design.md`

---

## File Structure

**New files:**

| File | Responsibility |
|---|---|
| `internal/platform/linear/client.go` | Linear GraphQL client: `Viewer`, `IssuesUpdatedSince`, `CreateComment`. Defines a `Client` interface for mockability. |
| `internal/platform/linear/cursor.go` | Read/write `linear.last_sync_at` via `SystemConfigStore`. |
| `internal/platform/linear/handler.go` | `LinearPlatform`, `LinearReceiver` (polling loop), `LinearSender` (post comment). |
| `internal/platform/linear/client_test.go` | `httptest`-based tests for the GraphQL client. |
| `internal/platform/linear/cursor_test.go` | Round-trip tests for the cursor. |
| `internal/platform/linear/handler_test.go` | Polling/dispatch behavior with a fake Client. |

**Modified files:**

| File | What changes |
|---|---|
| `internal/infra/config/config.go` | Add `LinearConfig`, register in `PlatformsConfig`, extend `BotNames`, defaults. |
| `internal/infra/config/config.yaml.tmpl` | Add `platforms.linear` block. |
| `internal/infra/model/system_config.go` | Add `SystemConfigKeyLinearLastSync` constant. |
| `internal/app/app.go` | Register the Linear platform in `buildPlatforms`; widen its signature with `*store.SystemConfigStore`; add `linear` to `WithPlatformBotNames`. |
| `cmd/openbee/config.go` | Add Linear fields to `configValues`, prompt block in interactive setup. |
| `internal/infra/i18n/messages.go` | Add `PlatformLinear`, `LinearAPIKey`, `LinearAPIKeyHelp`, `LinearLabel`. |
| `internal/infra/i18n/locales/en.yaml`, `zh.yaml` | New string entries. |
| `CHANGELOG.md` | English entry. |

---

## Task 1: Config schema and defaults

**Files:**
- Modify: `internal/infra/config/config.go`
- Modify: `internal/infra/config/config.yaml.tmpl`
- Modify: `internal/infra/model/system_config.go`

- [ ] **Step 1: Add `SystemConfigKeyLinearLastSync` constant**

Edit `internal/infra/model/system_config.go`, append after the existing constants:

```go
// SystemConfigKeyLinearLastSync is the key for the Linear poller's high-water cursor (RFC3339 timestamp).
const SystemConfigKeyLinearLastSync = "linear.last_sync_at"
```

- [ ] **Step 2: Add `LinearConfig` struct**

Edit `internal/infra/config/config.go`. After the `WeixinConfig` struct (around line 218), insert:

```go
type LinearConfig struct {
	Enabled      bool          `yaml:"enabled"`
	APIKey       string        `yaml:"api_key"`        // Linear personal API key (required when enabled)
	LabelName    string        `yaml:"label_name"`     // gating label; default "openbee"
	PollInterval time.Duration `yaml:"poll_interval"`  // default 10s
	BotName      string        `yaml:"bot_name"`       // for ingest @-mention strip; default "openbee"
}
```

- [ ] **Step 3: Register `Linear` field in `PlatformsConfig`**

In the same file, modify `PlatformsConfig` (around line 159) to add `Linear`:

```go
type PlatformsConfig struct {
	Feishu   FeishuConfig   `yaml:"feishu"`
	DingTalk DingTalkConfig `yaml:"dingtalk"`
	WeCom    WeComConfig    `yaml:"wecom"`
	Telegram TelegramConfig `yaml:"telegram"`
	Weixin   WeixinConfig   `yaml:"weixin"`
	Linear   LinearConfig   `yaml:"linear"`
}
```

- [ ] **Step 4: Extend `BotNames`**

In `PlatformsConfig.BotNames` (around line 167), append `p.Linear.BotName` to the slice:

```go
func (p PlatformsConfig) BotNames() []string {
	var names []string
	for _, n := range []string{
		p.Feishu.BotName,
		p.DingTalk.BotName,
		p.WeCom.BotName,
		p.Telegram.BotName,
		p.Weixin.BotName,
		p.Linear.BotName,
	} {
		if n != "" {
			names = append(names, n)
		}
	}
	return names
}
```

- [ ] **Step 5: Add defaults in `applyDefaults`**

In `applyDefaults` (around line 281), before the closing `return nil`, insert:

```go
	if cfg.Bee.Platforms.Linear.LabelName == "" {
		cfg.Bee.Platforms.Linear.LabelName = "openbee"
	}
	if cfg.Bee.Platforms.Linear.PollInterval == 0 {
		cfg.Bee.Platforms.Linear.PollInterval = 10 * time.Second
	}
	if cfg.Bee.Platforms.Linear.BotName == "" {
		cfg.Bee.Platforms.Linear.BotName = "openbee"
	}
```

- [ ] **Step 6: Add the `linear` block to the YAML template**

Edit `internal/infra/config/config.yaml.tmpl`. After the `weixin:` block (the line starting with `      bot_name: "{{.WeixinBotName}}"`), insert:

```yaml
    linear:
      enabled: {{.LinearEnabled}}
      api_key: "{{.LinearAPIKey}}"
      label_name: "{{.LinearLabelName}}"
      poll_interval: {{.LinearPollInterval}}
      bot_name: "{{.LinearBotName}}"
```

- [ ] **Step 7: Build to check the structs compile**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./internal/...`
Expected: clean build (the `cmd/openbee` package will now break because the template variables `LinearEnabled` etc. are not yet defined in `configValues` — that is fixed in Task 7).

- [ ] **Step 8: Commit**

```bash
git add internal/infra/config/config.go internal/infra/config/config.yaml.tmpl internal/infra/model/system_config.go
git commit -m "feat: add LinearConfig and last-sync system config key"
```

---

## Task 2: Linear types and Client interface

**Files:**
- Create: `internal/platform/linear/client.go`

This task lays down the data types and the public surface of the GraphQL client *as an interface*, so that handler.go can be written and tested against a fake. Concrete HTTP behavior is added in Task 3.

- [ ] **Step 1: Create `client.go` with types and interface only**

```go
package linear

import (
	"context"
	"time"
)

// User is the subset of Linear's User type we care about.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Team is the subset of Linear's Team type we care about.
type Team struct {
	Key string `json:"key"` // e.g. "ENG"
}

// IssueLabel carries the per-issue label assignment timestamp so we can detect
// when an issue first received the gating label.
type IssueLabel struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// Comment is the subset of Linear's Comment type we care about.
type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	User      User      `json:"user"`
	ParentID  *string   `json:"parentId"`
	IssueID   string    `json:"-"` // populated when read from Issue.Comments
}

// Issue is the subset of Linear's Issue type we care about.
type Issue struct {
	ID          string       `json:"id"`
	Identifier  string       `json:"identifier"` // e.g. "ENG-42"
	Title       string       `json:"title"`
	Description string       `json:"description"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	Team        Team         `json:"team"`
	Creator     User         `json:"creator"`
	Labels      []IssueLabel `json:"-"` // unwrapped from labels.nodes
	Comments    []Comment    `json:"-"` // unwrapped from comments.nodes
}

// Client is the Linear GraphQL client surface used by the receiver and sender.
// Tests substitute a fake.
type Client interface {
	Viewer(ctx context.Context) (User, error)
	IssuesUpdatedSince(ctx context.Context, since time.Time, label string) ([]Issue, error)
	CreateComment(ctx context.Context, issueID, body string, parentID *string) (Comment, error)
}
```

- [ ] **Step 2: Build to verify the package compiles**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./internal/platform/linear/...`
Expected: clean build (no concrete client yet, just types and interface).

- [ ] **Step 3: Commit**

```bash
git add internal/platform/linear/client.go
git commit -m "feat(linear): add Linear GraphQL types and Client interface"
```

---

## Task 3: Concrete GraphQL client

**Files:**
- Modify: `internal/platform/linear/client.go`
- Create: `internal/platform/linear/client_test.go`

- [ ] **Step 1: Write the failing test for `Viewer`**

Create `internal/platform/linear/client_test.go`:

```go
package linear

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newMockServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *httpClient) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := newHTTPClient("test-key")
	c.endpoint = srv.URL
	return srv, c
}

func TestClient_Viewer(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "test-key" {
			t.Errorf("Authorization header = %q, want test-key", got)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "viewer") {
			t.Errorf("query did not contain 'viewer': %s", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"viewer": map[string]string{"id": "U1", "name": "bot", "email": "bot@x"},
			},
		})
	})

	u, err := c.Viewer(context.Background())
	if err != nil {
		t.Fatalf("Viewer: %v", err)
	}
	if u.ID != "U1" || u.Name != "bot" {
		t.Errorf("got %+v", u)
	}
	_ = time.Now() // keep the time import live for later tests
}
```

- [ ] **Step 2: Run the test, expect a compile failure**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/platform/linear/...`
Expected: FAIL — `newHTTPClient` and the `Client.Viewer` method are not yet implemented.

- [ ] **Step 3: Add the concrete HTTP client and `Viewer` to `client.go`**

Append to `internal/platform/linear/client.go`:

```go
import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const defaultEndpoint = "https://api.linear.app/graphql"

type httpClient struct {
	apiKey   string
	endpoint string
	http     *http.Client
}

// NewClient returns a Client backed by Linear's GraphQL endpoint.
func NewClient(apiKey string) Client {
	return newHTTPClient(apiKey)
}

func newHTTPClient(apiKey string) *httpClient {
	return &httpClient{
		apiKey:   apiKey,
		endpoint: defaultEndpoint,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

func (c *httpClient) do(ctx context.Context, op string, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return fmt.Errorf("linear: marshal %s: %w", op, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("linear: build request %s: %w", op, err)
	}
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("linear: do %s: %w", op, err)
	}
	defer resp.Body.Close()

	var envelope gqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("linear: decode %s: %w", op, err)
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("linear: %s graphql error: %s", op, envelope.Errors[0].Message)
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("linear: decode %s data: %w", op, err)
		}
	}
	return nil
}

const viewerQuery = `query { viewer { id name email } }`

func (c *httpClient) Viewer(ctx context.Context) (User, error) {
	var data struct {
		Viewer User `json:"viewer"`
	}
	if err := c.do(ctx, "viewer", viewerQuery, nil, &data); err != nil {
		return User{}, err
	}
	return data.Viewer, nil
}
```

Also: at the top of `client.go` (where the package import block already lives from Task 2), make sure the imports include `bytes`, `encoding/json`, `fmt`, `net/http`. Consolidate into the existing `import (...)` block; do not create duplicates.

- [ ] **Step 4: Run the Viewer test, expect pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/platform/linear/... -run TestClient_Viewer -v`
Expected: PASS.

- [ ] **Step 5: Write the failing test for `CreateComment`**

Append to `client_test.go`:

```go
func TestClient_CreateComment(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		if !strings.Contains(s, "commentCreate") {
			t.Errorf("query missing commentCreate: %s", s)
		}
		if !strings.Contains(s, `"issueId":"I1"`) {
			t.Errorf("variables missing issueId: %s", s)
		}
		if !strings.Contains(s, `"parentId":"C0"`) {
			t.Errorf("variables missing parentId: %s", s)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"commentCreate": map[string]any{
					"comment": map[string]any{
						"id":        "C9",
						"body":      "hi",
						"createdAt": "2026-05-03T00:00:00Z",
						"user":      map[string]string{"id": "U1"},
					},
				},
			},
		})
	})

	parent := "C0"
	got, err := c.CreateComment(context.Background(), "I1", "hi", &parent)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if got.ID != "C9" {
		t.Errorf("got %+v", got)
	}
}
```

- [ ] **Step 6: Run the test to confirm it fails**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/platform/linear/... -run TestClient_CreateComment -v`
Expected: FAIL — `CreateComment` not implemented.

- [ ] **Step 7: Implement `CreateComment`**

Append to `client.go`:

```go
const createCommentMutation = `
mutation CreateComment($issueId: String!, $body: String!, $parentId: String) {
  commentCreate(input: { issueId: $issueId, body: $body, parentId: $parentId }) {
    comment { id body createdAt user { id } parentId }
  }
}`

func (c *httpClient) CreateComment(ctx context.Context, issueID, body string, parentID *string) (Comment, error) {
	vars := map[string]any{"issueId": issueID, "body": body, "parentId": parentID}
	var data struct {
		CommentCreate struct {
			Comment Comment `json:"comment"`
		} `json:"commentCreate"`
	}
	if err := c.do(ctx, "commentCreate", createCommentMutation, vars, &data); err != nil {
		return Comment{}, err
	}
	return data.CommentCreate.Comment, nil
}
```

- [ ] **Step 8: Run the test, expect pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/platform/linear/... -run TestClient_CreateComment -v`
Expected: PASS.

- [ ] **Step 9: Write the failing test for `IssuesUpdatedSince`**

Append to `client_test.go`:

```go
func TestClient_IssuesUpdatedSince(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		if !strings.Contains(s, `"label":"openbee"`) {
			t.Errorf("missing label var: %s", s)
		}
		if !strings.Contains(s, `"since":"`) {
			t.Errorf("missing since var: %s", s)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"nodes": []map[string]any{
						{
							"id":          "I1",
							"identifier":  "ENG-1",
							"title":       "first",
							"description": "body",
							"createdAt":   "2026-05-02T10:00:00Z",
							"updatedAt":   "2026-05-02T11:00:00Z",
							"team":        map[string]string{"key": "ENG"},
							"creator":     map[string]string{"id": "U2"},
							"labels": map[string]any{"nodes": []map[string]any{
								{"name": "openbee", "createdAt": "2026-05-02T10:30:00Z"},
							}},
							"comments": map[string]any{"nodes": []map[string]any{
								{"id": "C1", "body": "hi", "createdAt": "2026-05-02T10:45:00Z", "user": map[string]string{"id": "U2"}},
							}},
						},
					},
				},
			},
		})
	})

	since := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	out, err := c.IssuesUpdatedSince(context.Background(), since, "openbee")
	if err != nil {
		t.Fatalf("IssuesUpdatedSince: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(out))
	}
	if out[0].Identifier != "ENG-1" {
		t.Errorf("identifier = %q", out[0].Identifier)
	}
	if len(out[0].Labels) != 1 || out[0].Labels[0].Name != "openbee" {
		t.Errorf("labels = %+v", out[0].Labels)
	}
	if len(out[0].Comments) != 1 || out[0].Comments[0].ID != "C1" {
		t.Errorf("comments = %+v", out[0].Comments)
	}
	if out[0].Comments[0].IssueID != "I1" {
		t.Errorf("comment IssueID not back-filled: %q", out[0].Comments[0].IssueID)
	}
}
```

- [ ] **Step 10: Run to confirm failure**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/platform/linear/... -run TestClient_IssuesUpdatedSince -v`
Expected: FAIL — method not implemented.

- [ ] **Step 11: Implement `IssuesUpdatedSince`**

Append to `client.go`:

```go
const issuesQuery = `
query Issues($since: DateTime!, $label: String!) {
  issues(
    filter: { updatedAt: { gt: $since }, labels: { name: { eq: $label } } }
    orderBy: updatedAt
  ) {
    nodes {
      id identifier title description createdAt updatedAt
      team { key }
      creator { id name email }
      labels(filter: { name: { eq: $label } }) {
        nodes { name createdAt }
      }
      comments {
        nodes { id body createdAt user { id name email } parentId }
      }
    }
  }
}`

func (c *httpClient) IssuesUpdatedSince(ctx context.Context, since time.Time, label string) ([]Issue, error) {
	vars := map[string]any{"since": since.UTC().Format(time.RFC3339), "label": label}
	var data struct {
		Issues struct {
			Nodes []struct {
				Issue
				Labels   struct{ Nodes []IssueLabel `json:"nodes"` } `json:"labels"`
				Comments struct{ Nodes []Comment    `json:"nodes"` } `json:"comments"`
			} `json:"nodes"`
		} `json:"issues"`
	}
	if err := c.do(ctx, "issues", issuesQuery, vars, &data); err != nil {
		return nil, err
	}
	out := make([]Issue, 0, len(data.Issues.Nodes))
	for _, n := range data.Issues.Nodes {
		issue := n.Issue
		issue.Labels = n.Labels.Nodes
		issue.Comments = n.Comments.Nodes
		for i := range issue.Comments {
			issue.Comments[i].IssueID = issue.ID
		}
		out = append(out, issue)
	}
	return out, nil
}
```

- [ ] **Step 12: Run all client tests, expect pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/platform/linear/... -v`
Expected: 3 passing tests.

- [ ] **Step 13: Commit**

```bash
git add internal/platform/linear/client.go internal/platform/linear/client_test.go
git commit -m "feat(linear): implement GraphQL client (viewer, issues, commentCreate)"
```

---

## Task 4: Cursor

**Files:**
- Create: `internal/platform/linear/cursor.go`
- Create: `internal/platform/linear/cursor_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/platform/linear/cursor_test.go`:

```go
package linear

import (
	"context"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/store"
)

func newCursorTestStore(t *testing.T) *store.SystemConfigStore {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store.NewSystemConfigStore(db)
}

func TestCursor_LoadMissingReturnsBootstrapWindow(t *testing.T) {
	c := NewCursor(newCursorTestStore(t))
	got, err := c.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	delta := time.Since(got)
	if delta < 30*time.Minute || delta > 90*time.Minute {
		t.Errorf("bootstrap window out of range: now-loaded=%v", delta)
	}
}

func TestCursor_SaveAndLoad(t *testing.T) {
	c := NewCursor(newCursorTestStore(t))
	want := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	if err := c.Save(context.Background(), want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := c.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/platform/linear/... -run TestCursor -v`
Expected: FAIL — `NewCursor` not defined.

- [ ] **Step 3: Implement `cursor.go`**

Create `internal/platform/linear/cursor.go`:

```go
package linear

import (
	"context"
	"time"

	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

// Cursor reads/writes the Linear poller high-water mark from system_configs.
type Cursor struct {
	store *store.SystemConfigStore
}

// NewCursor constructs a Cursor backed by the given SystemConfigStore.
func NewCursor(s *store.SystemConfigStore) *Cursor {
	return &Cursor{store: s}
}

// Load returns the saved high-water mark, or now-1h on first run.
func (c *Cursor) Load(ctx context.Context) (time.Time, error) {
	cfg, found, err := c.store.Get(ctx, model.SystemConfigKeyLinearLastSync)
	if err != nil {
		return time.Time{}, err
	}
	if !found || cfg.Value == "" {
		return time.Now().Add(-1 * time.Hour), nil
	}
	t, err := time.Parse(time.RFC3339Nano, cfg.Value)
	if err != nil {
		// Fallback to bootstrap window if the saved value is malformed.
		return time.Now().Add(-1 * time.Hour), nil
	}
	return t, nil
}

// Save persists the high-water mark.
func (c *Cursor) Save(ctx context.Context, t time.Time) error {
	return c.store.Set(ctx, model.SystemConfigKeyLinearLastSync, t.UTC().Format(time.RFC3339Nano))
}
```

- [ ] **Step 4: Run tests, expect pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/platform/linear/... -run TestCursor -v`
Expected: 2 passing tests.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/cursor.go internal/platform/linear/cursor_test.go
git commit -m "feat(linear): add lastSyncAt cursor backed by system_configs"
```

---

## Task 5: Receiver and Sender

**Files:**
- Create: `internal/platform/linear/handler.go`
- Create: `internal/platform/linear/handler_test.go`

- [ ] **Step 1: Write the failing test for inbound mapping and bot-self filter**

Create `internal/platform/linear/handler_test.go`:

```go
package linear

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/platform"
)

// fakeClient is a Client that returns canned data per call.
type fakeClient struct {
	mu       sync.Mutex
	viewer   User
	calls    int
	issues   func(since time.Time) ([]Issue, error)
	created  []struct {
		IssueID, Body string
		ParentID      *string
	}
}

func (f *fakeClient) Viewer(ctx context.Context) (User, error) { return f.viewer, nil }

func (f *fakeClient) IssuesUpdatedSince(ctx context.Context, since time.Time, label string) ([]Issue, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.issues(since)
}

func (f *fakeClient) CreateComment(ctx context.Context, issueID, body string, parentID *string) (Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, struct {
		IssueID, Body string
		ParentID      *string
	}{issueID, body, parentID})
	return Comment{ID: "C-new"}, nil
}

type fakeCursor struct {
	last  time.Time
	saved time.Time
}

func (c *fakeCursor) Load(ctx context.Context) (time.Time, error) { return c.last, nil }
func (c *fakeCursor) Save(ctx context.Context, t time.Time) error { c.saved = t; return nil }

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestReceiver_TickOnce_DispatchesIssueAndComments(t *testing.T) {
	bot := User{ID: "BOT"}
	since := mustParse(t, "2026-05-02T09:00:00Z")
	issue := Issue{
		ID:          "I1",
		Identifier:  "ENG-42",
		Title:       "Title",
		Description: "Body",
		CreatedAt:   mustParse(t, "2026-05-02T10:00:00Z"),
		UpdatedAt:   mustParse(t, "2026-05-02T11:30:00Z"),
		Team:        Team{Key: "ENG"},
		Creator:     User{ID: "U2"},
		Labels: []IssueLabel{
			{Name: "openbee", CreatedAt: mustParse(t, "2026-05-02T10:30:00Z")},
		},
		Comments: []Comment{
			{ID: "C1", Body: "first", CreatedAt: mustParse(t, "2026-05-02T11:00:00Z"), User: User{ID: "U2"}, IssueID: "I1"},
			{ID: "C-bot", Body: "ignore me", CreatedAt: mustParse(t, "2026-05-02T11:15:00Z"), User: bot, IssueID: "I1"},
			{ID: "C2", Body: "second", CreatedAt: mustParse(t, "2026-05-02T11:30:00Z"), User: User{ID: "U2"}, IssueID: "I1"},
		},
	}
	fc := &fakeClient{
		viewer: bot,
		issues: func(_ time.Time) ([]Issue, error) { return []Issue{issue}, nil },
	}
	cur := &fakeCursor{last: since}

	r := &LinearReceiver{
		client:    fc,
		cursor:    cur,
		labelName: "openbee",
		botUserID: bot.ID,
	}

	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })

	// Expect 3 dispatches: issue body, C1, C2 (C-bot filtered).
	if len(got) != 3 {
		t.Fatalf("dispatched %d messages, want 3: %+v", len(got), got)
	}
	// Sort for stable assertions (dispatch order should already be chronological).
	sort.Slice(got, func(i, j int) bool { return got[i].MessageTime < got[j].MessageTime })
	if got[0].PlatformMessageID != "issue:I1" {
		t.Errorf("first dispatch should be issue body: %+v", got[0])
	}
	if got[0].SessionKey != "linear:ENG:ENG-42" {
		t.Errorf("session key: %q", got[0].SessionKey)
	}
	if got[1].PlatformMessageID != "comment:C1" || got[2].PlatformMessageID != "comment:C2" {
		t.Errorf("comment IDs out of order: %+v", got)
	}
	// Cursor advanced to issue.UpdatedAt or last comment, whichever later.
	if !cur.saved.Equal(issue.UpdatedAt) {
		t.Errorf("cursor saved = %v, want %v", cur.saved, issue.UpdatedAt)
	}
}

func TestReceiver_TickOnce_ErrorDoesNotAdvanceCursor(t *testing.T) {
	cur := &fakeCursor{last: mustParse(t, "2026-05-02T09:00:00Z")}
	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		issues: func(_ time.Time) ([]Issue, error) { return nil, errors.New("boom") },
	}
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", botUserID: "BOT"}

	r.tickOnce(context.Background(), func(platform.InboundMessage) {})
	if !cur.saved.IsZero() {
		t.Errorf("cursor advanced on error: %v", cur.saved)
	}
}

func TestSender_PostsCommentWithParentID(t *testing.T) {
	parent := "C0"
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1", ParentCommentID: &parent})

	fc := &fakeClient{viewer: User{ID: "BOT"}}
	s := &LinearSender{client: fc}
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hello",
		ReplyTo: platform.InboundMessage{Raw: string(rawBytes)},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(fc.created) != 1 {
		t.Fatalf("expected 1 CreateComment call, got %d", len(fc.created))
	}
	c := fc.created[0]
	if c.IssueID != "I1" || c.Body != "hello" || c.ParentID == nil || *c.ParentID != "C0" {
		t.Errorf("unexpected call: %+v", c)
	}
}

func TestSender_RejectsMediaPath(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})
	s := &LinearSender{client: &fakeClient{}}
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content:   "x",
		MediaPath: "/tmp/foo.png",
		ReplyTo:   platform.InboundMessage{Raw: string(rawBytes)},
	})
	if err == nil {
		t.Error("expected error for MediaPath")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/platform/linear/... -run TestReceiver -v`
Expected: FAIL — types not implemented.

- [ ] **Step 3: Implement `handler.go`**

Create `internal/platform/linear/handler.go`:

```go
package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// PlatformID is the platform identifier used in SessionKey and ingest routing.
const PlatformID = "linear"

var log = logger.With(zap.String("component", "linear"))

// cursorAPI is satisfied by *Cursor and by test fakes.
type cursorAPI interface {
	Load(ctx context.Context) (time.Time, error)
	Save(ctx context.Context, t time.Time) error
}

// LinearPlatform implements platform.Platform.
type LinearPlatform struct {
	receiver *LinearReceiver
	sender   *LinearSender
}

// NewPlatform constructs a Linear platform from configuration.
func NewPlatform(cfg config.LinearConfig, sysCfg *store.SystemConfigStore) platform.Platform {
	client := NewClient(cfg.APIKey)
	return &LinearPlatform{
		receiver: &LinearReceiver{
			client:       client,
			cursor:       NewCursor(sysCfg),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
		},
		sender: &LinearSender{client: client},
	}
}

func (p *LinearPlatform) ID() string                                  { return PlatformID }
func (p *LinearPlatform) Receiver() platform.PlatformReceiverAdapter  { return p.receiver }
func (p *LinearPlatform) Sender() platform.PlatformSenderAdapter      { return p.sender }

// LinearReceiver polls Linear for issue/comment updates.
type LinearReceiver struct {
	client       Client
	cursor       cursorAPI
	labelName    string
	pollInterval time.Duration
	botUserID    string
}

// Start runs the polling loop until ctx is cancelled.
func (r *LinearReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	viewer, err := r.client.Viewer(ctx)
	if err != nil {
		return fmt.Errorf("linear receiver: viewer: %w", err)
	}
	r.botUserID = viewer.ID
	log.Info("linear receiver started", zap.String("bot_user_id", r.botUserID), zap.String("label", r.labelName))

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.tickOnce(ctx, dispatch)
		}
	}
}

// tickOnce performs one polling cycle. Errors are logged; the cursor only
// advances on success.
func (r *LinearReceiver) tickOnce(ctx context.Context, dispatch func(platform.InboundMessage)) {
	since, err := r.cursor.Load(ctx)
	if err != nil {
		log.Error("cursor load", zap.Error(err))
		return
	}
	issues, err := r.client.IssuesUpdatedSince(ctx, since, r.labelName)
	if err != nil {
		log.Error("issues fetch", zap.Error(err))
		return
	}
	highWater := since
	for _, issue := range issues {
		if isNewlyOwned(issue, since) {
			dispatch(buildIssueInbound(issue))
		}
		for _, c := range issue.Comments {
			if !c.CreatedAt.After(since) {
				continue
			}
			if c.User.ID == r.botUserID {
				continue
			}
			dispatch(buildCommentInbound(issue, c))
			if c.CreatedAt.After(highWater) {
				highWater = c.CreatedAt
			}
		}
		if issue.UpdatedAt.After(highWater) {
			highWater = issue.UpdatedAt
		}
	}
	if highWater.After(since) {
		if err := r.cursor.Save(ctx, highWater); err != nil {
			log.Error("cursor save", zap.Error(err))
		}
	}
}

func isNewlyOwned(issue Issue, since time.Time) bool {
	for _, l := range issue.Labels {
		if l.CreatedAt.After(since) {
			return true
		}
	}
	return issue.CreatedAt.After(since)
}

func buildSessionKey(teamKey, identifier string) string {
	return fmt.Sprintf("%s:%s:%s", PlatformID, teamKey, identifier)
}

// replyTarget is what we serialize into InboundMessage.Raw so the sender can
// resolve the comment target without re-querying Linear.
type replyTarget struct {
	IssueID         string  `json:"issue_id"`
	ParentCommentID *string `json:"parent_comment_id,omitempty"`
}

func buildIssueInbound(issue Issue) platform.InboundMessage {
	raw, _ := json.Marshal(replyTarget{IssueID: issue.ID})
	content := issue.Title
	if issue.Description != "" {
		content = issue.Title + "\n\n" + issue.Description
	}
	return platform.InboundMessage{
		Platform:          PlatformID,
		SenderID:          issue.Creator.ID,
		SessionKey:        buildSessionKey(issue.Team.Key, issue.Identifier),
		Content:           content,
		RawContent:        content,
		Raw:               string(raw),
		PlatformMessageID: "issue:" + issue.ID,
		MessageTime:       issue.CreatedAt.UnixMilli(),
	}
}

func buildCommentInbound(issue Issue, c Comment) platform.InboundMessage {
	parent := c.ParentID
	if parent == nil {
		// Top-level comment: replies should thread under the comment itself.
		id := c.ID
		parent = &id
	}
	raw, _ := json.Marshal(replyTarget{IssueID: issue.ID, ParentCommentID: parent})
	return platform.InboundMessage{
		Platform:          PlatformID,
		SenderID:          c.User.ID,
		SessionKey:        buildSessionKey(issue.Team.Key, issue.Identifier),
		Content:           c.Body,
		RawContent:        c.Body,
		Raw:               string(raw),
		PlatformMessageID: "comment:" + c.ID,
		MessageTime:       c.CreatedAt.UnixMilli(),
	}
}

// LinearSender posts replies as Linear comments.
type LinearSender struct {
	client Client
}

// Send posts msg.Content as a comment on the issue identified by msg.ReplyTo.Raw.
func (s *LinearSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	if msg.MediaPath != "" {
		return errors.New("linear: media attachments not supported in v0")
	}
	var target replyTarget
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &target); err != nil {
		return fmt.Errorf("linear: parse reply target: %w", err)
	}
	if target.IssueID == "" {
		return errors.New("linear: reply target missing issue_id")
	}
	_, err := s.client.CreateComment(ctx, target.IssueID, msg.Content, target.ParentCommentID)
	return err
}

// Interface compliance guards.
var _ platform.Platform                = (*LinearPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*LinearReceiver)(nil)
var _ platform.PlatformSenderAdapter   = (*LinearSender)(nil)
```

- [ ] **Step 4: Run all linear tests, expect pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/platform/linear/... -v`
Expected: all tests pass (client, cursor, receiver, sender).

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go
git commit -m "feat(linear): add polling Receiver and comment Sender"
```

---

## Task 6: Wire the platform into app.go

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add the import**

Edit `internal/app/app.go`. In the platform import group (around line 38–44), add:

```go
	"github.com/theopenbee/openbee/internal/platform/linear"
```

(Keep imports sorted alphabetically with the others.)

- [ ] **Step 2: Widen `buildPlatforms` to accept the system-config store**

Change the signature (around line 309):

```go
func buildPlatforms(
	fc config.FeishuConfig,
	dc config.DingTalkConfig,
	wc config.WeComConfig,
	tc config.TelegramConfig,
	wxc config.WeixinConfig,
	lc config.LinearConfig,
	mc config.MediaConfig,
	sysCfg *store.SystemConfigStore,
) []platform.Platform {
```

Add the Linear branch at the end of the function (just before `return result`):

```go
	if lc.Enabled {
		result = append(result, linear.NewPlatform(lc, sysCfg))
	}
```

- [ ] **Step 3: Update the call site**

Around line 151 in `BuildApp`, change:

```go
	platforms := buildPlatforms(cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom, cfg.Bee.Platforms.Telegram, cfg.Bee.Platforms.Weixin, cfg.Bee.Media)
```

to:

```go
	platforms := buildPlatforms(
		cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom,
		cfg.Bee.Platforms.Telegram, cfg.Bee.Platforms.Weixin, cfg.Bee.Platforms.Linear,
		cfg.Bee.Media, s.systemConfigStore,
	)
```

- [ ] **Step 4: Add the linear bot name to `WithPlatformBotNames`**

In the `msgingest.WithPlatformBotNames` map literal (around line 165), add:

```go
		linear.PlatformID:   cfg.Bee.Platforms.Linear.BotName,
```

- [ ] **Step 5: Build the whole app**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...`
Expected: clean build (the `cmd/openbee` package may still fail until Task 7 — that's fine for now if it's only the config command; rerun after Task 7).

If the build fails outside of `cmd/openbee/config.go`, fix the issue before continuing.

- [ ] **Step 6: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(linear): register Linear platform and bot name in app wiring"
```

---

## Task 7: Interactive `openbee config init` prompts

**Files:**
- Modify: `internal/infra/i18n/messages.go`
- Modify: `internal/infra/i18n/locales/en.yaml`
- Modify: `internal/infra/i18n/locales/zh.yaml`
- Modify: `cmd/openbee/config.go`

- [ ] **Step 1: Add new i18n keys to `messages.go`**

Edit `internal/infra/i18n/messages.go`. In `PromptMessages` (around line 64 where existing platform keys live), add:

```go
	PlatformLinear   string `yaml:"platform_linear"`
```

In the same struct, near the platform-specific input keys (around line 78), add:

```go
	LinearAPIKey      string `yaml:"linear_api_key"`
	LinearAPIKeyHelp  string `yaml:"linear_api_key_help"`
	LinearLabelName   string `yaml:"linear_label_name"`
```

- [ ] **Step 2: Add the new strings to both locale files**

Edit `internal/infra/i18n/locales/en.yaml`. Under `prompt:`, near the other `platform_*` keys, insert:

```yaml
  platform_linear: "Linear"
```

And near the platform-input keys:

```yaml
  linear_api_key: "Linear API Key:"
  linear_api_key_help: "Create a personal API key at https://linear.app/settings/api"
  linear_label_name: "Linear gating label (default: openbee):"
```

Repeat with Chinese strings in `zh.yaml`:

```yaml
  platform_linear: "Linear"
  linear_api_key: "Linear API Key："
  linear_api_key_help: "在 https://linear.app/settings/api 创建个人 API Key"
  linear_label_name: "Linear 触发标签（默认：openbee）："
```

- [ ] **Step 3: Add fields to `configValues`**

Edit `cmd/openbee/config.go`. In `configValues` (around line 74, after the Weixin block), add:

```go
	LinearEnabled      bool
	LinearAPIKey       string
	LinearLabelName    string
	LinearPollInterval string
	LinearBotName      string
```

- [ ] **Step 4: Populate defaults from cfg**

In the function that builds `vals` from `cfg` (around line 153, after the `WeixinBotName` line), add:

```go
		LinearEnabled:      cfg.Bee.Platforms.Linear.Enabled,
		LinearAPIKey:       cfg.Bee.Platforms.Linear.APIKey,
		LinearLabelName:    cfg.Bee.Platforms.Linear.LabelName,
		LinearPollInterval: cfg.Bee.Platforms.Linear.PollInterval.String(),
		LinearBotName:      cfg.Bee.Platforms.Linear.BotName,
```

- [ ] **Step 5: Add Linear to default selection list**

Around line 357 (the existing `if vals.TelegramEnabled` block), add a parallel block for Linear:

```go
	if vals.LinearEnabled {
		defaultPlatforms = append(defaultPlatforms, i18n.M.Prompt.PlatformLinear)
	}
```

- [ ] **Step 6: Add Linear to the multi-select options**

Around line 368 (`Options: []string{ ... }`), add:

```go
			i18n.M.Prompt.PlatformLinear,
```

after the existing entries (e.g. after `PlatformTelegram`).

- [ ] **Step 7: Reset Linear before re-asking**

Around line 380 (where the other `vals.XxxEnabled = false` lines live), add:

```go
	vals.LinearEnabled = false
```

- [ ] **Step 8: Add the Linear prompt switch case**

After the Telegram case (search for `case i18n.M.Prompt.PlatformTelegram:`), add a parallel case (place it after the Weixin case if Weixin's switch is after Telegram's; preserve existing ordering):

```go
		case i18n.M.Prompt.PlatformLinear:
			vals.LinearEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: i18n.M.Prompt.LinearAPIKey,
				Help:    i18n.M.Prompt.LinearAPIKeyHelp,
				Default: vals.LinearAPIKey,
			}, &vals.LinearAPIKey, survey.WithValidator(survey.Required)); err != nil {
				return err
			}
			labelDefault := vals.LinearLabelName
			if labelDefault == "" {
				labelDefault = "openbee"
			}
			if err := survey.AskOne(&survey.Input{
				Message: i18n.M.Prompt.LinearLabelName,
				Default: labelDefault,
			}, &vals.LinearLabelName); err != nil {
				return err
			}
			if vals.LinearPollInterval == "" {
				vals.LinearPollInterval = "10s"
			}
			if err := promptBotName(&vals.LinearBotName); err != nil {
				return err
			}
```

(`promptBotName` is the existing helper used by all other platforms.)

- [ ] **Step 9: Build the whole binary**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...`
Expected: clean build.

- [ ] **Step 10: Run all unit tests**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./...`
Expected: all tests pass.

- [ ] **Step 11: Commit**

```bash
git add internal/infra/i18n/messages.go internal/infra/i18n/locales/en.yaml internal/infra/i18n/locales/zh.yaml cmd/openbee/config.go
git commit -m "feat(linear): add interactive prompts to openbee config init"
```

---

## Task 8: CHANGELOG

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add the entry**

Open `CHANGELOG.md`. In the next-release section (or `## Unreleased` if it exists; otherwise add one above the latest version), add an English bullet:

```markdown
- Add Linear platform integration: issues with the `openbee` label (and their comments) flow into the existing message pipeline; bee/worker replies are posted back as Linear comments. Polling-based receiver; configuration under `bee.platforms.linear`.
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog entry for Linear platform integration"
```

---

## Task 9: End-to-end smoke (manual)

This task is not automated — Linear's API requires a real workspace.

- [ ] **Step 1: Configure a test workspace**

In a Linear workspace where you have admin access:
1. Create a personal API key (https://linear.app/settings/api).
2. Create a label named `openbee` on at least one team.

- [ ] **Step 2: Edit local config**

In your `~/.openbee/config.yaml` (or wherever the deployment reads from), set:

```yaml
bee:
  platforms:
    linear:
      enabled: true
      api_key: "<your key>"
      label_name: "openbee"
      poll_interval: 10s
      bot_name: "openbee"
```

- [ ] **Step 3: Start the server**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go run ./cmd/openbee server`
Expected log line: `linear receiver started bot_user_id=… label=openbee`

- [ ] **Step 4: Trigger an issue**

Create a Linear issue with the `openbee` label, body `hello bee`. Within ~poll_interval seconds, expect a comment posted by the bot user containing the bee's reply.

- [ ] **Step 5: Trigger a worker**

In a new issue with the `openbee` label, set the body to `@<workerName> do task X` (using a worker that exists). Expect the reply to come from that worker rather than the bee.

- [ ] **Step 6: Verify dedup**

Restart the server with the same config. The same issue/comment must not be reprocessed (the cursor in `bee_system_configs.linear.last_sync_at` should prevent it; the `msg_store.PlatformMessageID` unique constraint backs it up).

If anything misbehaves, capture the failure mode and open a follow-up before declaring the task done.

---

## Self-Review Notes

- Spec §1 background → covered by Tasks 5 + 6 (Receiver/Sender + wiring).
- Spec §2 non-goals (no webhook, no media, no status mutation, single workspace) → no tasks needed; v0 deliberately does not include them. Task 5 enforces the no-media constraint with an explicit error.
- Spec §3 architecture → Task 2/3/4/5.
- Spec §4 configuration → Task 1 (yaml + struct) + Task 7 (interactive prompts).
- Spec §5 components → Tasks 2, 3, 4, 5.
- Spec §6 polling flow → Task 5 (`tickOnce`).
- Spec §7 inbound mapping → Task 5 (`buildIssueInbound`, `buildCommentInbound`).
- Spec §8 sender flow → Task 5.
- Spec §9 loop prevention → Task 5 receiver-side filter; backstop is the existing `msg_store` unique constraint.
- Spec §10 startup registration → Task 6.
- Spec §11 testing → Tasks 3, 4, 5 each ship tests alongside.
- Spec §12 files touched → all listed in this plan's File Structure section.
- Spec §13 future work (webhook, status feedback, attachments) → out of scope; flagged for follow-up.
