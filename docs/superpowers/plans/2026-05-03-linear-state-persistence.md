# Linear 平台状态文件持久化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Linear platform's poller cursor and bot self-comment-ID set from `system_configs` DB + in-memory ring buffer to file-based persistence under `~/.openbee/.linear/`.

**Architecture:**
- `cursor.json` — single JSON object `{"last_sync": "<RFC3339Nano>"}`, written atomically via tmp+rename.
- `self_comments.log` — append-only line file, one comment ID per line, opened with `O_APPEND`. No eviction; permanent retention.
- `linear.NewPlatform` becomes self-contained: computes the directory itself from `os.UserHomeDir()`, no longer accepts `*store.SystemConfigStore`. The DB key constant is removed.

**Tech Stack:** Go, stdlib only (`os`, `bufio`, `encoding/json`, `path/filepath`, `sync`).

**Spec:** `docs/superpowers/specs/2026-05-03-linear-state-persistence-design.md`

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `internal/platform/linear/cursor.go` | Rewrite | File-backed `Cursor` with `Load`/`Save` |
| `internal/platform/linear/cursor_test.go` | Rewrite | Tests for file-based cursor (tmp dir, fallback, atomicity) |
| `internal/platform/linear/handler.go` | Modify | `selfComments` persistent + `NewPlatform` signature |
| `internal/platform/linear/handler_test.go` | Modify | Update `newSelfComments` call sites; add persistence test |
| `internal/app/app.go` | Modify | `buildPlatforms` signature (returns error, drops `sysCfg`) |
| `internal/infra/model/system_config.go` | Modify | Delete `SystemConfigKeyLinearLastSync` constant |

---

## Task 1: File-Backed Cursor + NewPlatform/wiring updates

**Files:**
- Modify: `internal/platform/linear/cursor.go` (full rewrite of body)
- Modify: `internal/platform/linear/cursor_test.go` (full rewrite)
- Modify: `internal/platform/linear/handler.go:81-94` (`NewPlatform`)
- Modify: `internal/app/app.go:152-156` (caller) and `:315-348` (`buildPlatforms`)

This task changes the `NewCursor` signature, which forces the `NewPlatform` signature change, which forces the `buildPlatforms` signature change. They all ship in one commit so the package always compiles.

- [ ] **Step 1.1: Write failing tests for file-based Cursor**

Replace `internal/platform/linear/cursor_test.go` with:

```go
package linear

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCursor_LoadMissingReturnsBootstrapWindow(t *testing.T) {
	c := NewCursor(t.TempDir())
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
	dir := t.TempDir()
	c := NewCursor(dir)
	want := time.Date(2026, 5, 2, 12, 0, 0, 123456789, time.UTC)
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

func TestCursor_LoadCorruptFileReturnsBootstrapWindow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cursor.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := NewCursor(dir).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if delta := time.Since(got); delta < 30*time.Minute || delta > 90*time.Minute {
		t.Errorf("expected bootstrap fallback, got delta=%v", delta)
	}
}

func TestCursor_SaveLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	c := NewCursor(dir)
	if err := c.Save(context.Background(), time.Now()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cursor.json.tmp")); !os.IsNotExist(err) {
		t.Errorf("cursor.json.tmp should be removed after rename, stat err: %v", err)
	}
}

func TestCursor_SaveCreatesDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "nested", "linear")
	if err := NewCursor(dir).Save(context.Background(), time.Now()); err != nil {
		t.Fatalf("Save into missing dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cursor.json")); err != nil {
		t.Errorf("cursor.json not written: %v", err)
	}
}
```

- [ ] **Step 1.2: Run cursor tests to verify they fail**

Run: `go test ./internal/platform/linear/ -run TestCursor -v`
Expected: compile error or failures — `NewCursor` currently takes `*store.SystemConfigStore`, not `string`.

- [ ] **Step 1.3: Rewrite `cursor.go` to use a JSON file**

Replace `internal/platform/linear/cursor.go` with:

```go
package linear

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Cursor reads/writes the Linear poller high-water mark from a JSON file
// at <dir>/cursor.json. The write path uses tmp+rename for atomicity so a
// crash mid-write cannot leave a corrupted file.
type Cursor struct {
	dir string
}

// NewCursor constructs a Cursor that persists to <dir>/cursor.json.
func NewCursor(dir string) *Cursor {
	return &Cursor{dir: dir}
}

type cursorFile struct {
	LastSync string `json:"last_sync"`
}

// Load returns the saved high-water mark, or now-1h on first run / corrupt file.
func (c *Cursor) Load(ctx context.Context) (time.Time, error) {
	data, err := os.ReadFile(filepath.Join(c.dir, "cursor.json"))
	if errors.Is(err, os.ErrNotExist) {
		return time.Now().Add(-1 * time.Hour), nil
	}
	if err != nil {
		return time.Time{}, err
	}
	var cf cursorFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return time.Now().Add(-1 * time.Hour), nil
	}
	t, err := time.Parse(time.RFC3339Nano, cf.LastSync)
	if err != nil {
		return time.Now().Add(-1 * time.Hour), nil
	}
	return t, nil
}

// Save persists the high-water mark atomically via tmp+rename.
func (c *Cursor) Save(ctx context.Context, t time.Time) error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(cursorFile{LastSync: t.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	tmp := filepath.Join(c.dir, "cursor.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(c.dir, "cursor.json"))
}
```

- [ ] **Step 1.4: Update `NewPlatform` signature in `handler.go`**

Replace the `NewPlatform` function at `internal/platform/linear/handler.go:81-94` with:

```go
// NewPlatform constructs a Linear platform from configuration. Persistent
// state lives in ~/.openbee/.linear/.
func NewPlatform(cfg config.LinearConfig) (platform.Platform, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("linear: resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".linear")
	client := NewClient(cfg.APIKey)
	self := newSelfComments()
	return &LinearPlatform{
		receiver: &LinearReceiver{
			client:       client,
			cursor:       NewCursor(dir),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
			self:         self,
		},
		sender: &LinearSender{client: client, self: self},
	}, nil
}
```

Add to imports at the top of `handler.go` (the existing block at lines 3-18):

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/utils"
)
```

(removed `"github.com/theopenbee/openbee/internal/infra/store"`; added `"os"` and `"path/filepath"`.)

- [ ] **Step 1.5: Update `buildPlatforms` and its caller in `app.go`**

At `internal/app/app.go:315-348`, change the function signature and body:

```go
func buildPlatforms(
	fc config.FeishuConfig,
	dc config.DingTalkConfig,
	wc config.WeComConfig,
	tc config.TelegramConfig,
	wxc config.WeixinConfig,
	lc config.LinearConfig,
	mc config.MediaConfig,
) ([]platform.Platform, error) {
	mediaSvc := media.NewService()
	var result []platform.Platform
	if fc.Enabled {
		platform.RegisterExtractor(feishu.PlatformID, feishu.ExtractContext)
		result = append(result, feishu.NewPlatform(fc, mediaSvc))
	}
	if dc.Enabled {
		platform.RegisterExtractor(dingtalk.PlatformID, dingtalk.ExtractContext)
		result = append(result, dingtalk.NewPlatform(dc, mc, mediaSvc))
	}
	if wc.Enabled {
		platform.RegisterExtractor(wecom.PlatformID, wecom.ExtractContext)
		result = append(result, wecom.NewPlatform(wc, mediaSvc))
	}
	if tc.Enabled {
		result = append(result, telegram.NewPlatform(tc, mediaSvc))
	}
	if wxc.Enabled {
		result = append(result, weixin.NewPlatform(wxc, mc, mediaSvc))
	}
	if lc.Enabled {
		p, err := linear.NewPlatform(lc)
		if err != nil {
			return nil, fmt.Errorf("init linear platform: %w", err)
		}
		result = append(result, p)
	}
	return result, nil
}
```

At the caller `internal/app/app.go:152-156`, change:

```go
	platforms, err := buildPlatforms(
		cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom,
		cfg.Bee.Platforms.Telegram, cfg.Bee.Platforms.Weixin, cfg.Bee.Platforms.Linear,
		cfg.Bee.Media,
	)
	if err != nil {
		return nil, err
	}
```

(Removed the `s.systemConfigStore` argument; added error handling. The caller is inside a function that already returns `(*App, error)`, so propagation is straightforward.)

- [ ] **Step 1.6: Verify the linear package compiles and cursor tests pass**

Run: `go test ./internal/platform/linear/ -run TestCursor -v`
Expected: 5 tests PASS (`TestCursor_LoadMissingReturnsBootstrapWindow`, `TestCursor_SaveAndLoad`, `TestCursor_LoadCorruptFileReturnsBootstrapWindow`, `TestCursor_SaveLeavesNoTempFile`, `TestCursor_SaveCreatesDir`).

- [ ] **Step 1.7: Verify whole module still builds and tests pass**

Run: `go build ./...`
Expected: clean build, no errors.

Run: `go test ./internal/platform/linear/ ./internal/app/...`
Expected: all tests pass. The handler tests still use `newSelfComments()` (no arg) and that's intentional — they keep working unchanged in this task.

- [ ] **Step 1.8: Commit**

```bash
git add internal/platform/linear/cursor.go \
        internal/platform/linear/cursor_test.go \
        internal/platform/linear/handler.go \
        internal/app/app.go
git commit -m "$(cat <<'EOF'
feat(linear): persist cursor to ~/.openbee/.linear/cursor.json

Replace SystemConfigStore-backed cursor with a tmp+rename JSON file.
NewPlatform now computes the state dir itself and no longer takes
*store.SystemConfigStore; buildPlatforms returns an error.
EOF
)"
```

---

## Task 2: Persistent self_comments log

**Files:**
- Modify: `internal/platform/linear/handler.go` (selfComments struct, newSelfComments, Add)
- Modify: `internal/platform/linear/handler.go` (NewPlatform — pass dir to newSelfComments)
- Modify: `internal/platform/linear/handler_test.go` (replace `newSelfComments()` calls with dir; add persistence test)

- [ ] **Step 2.1: Write failing persistence test**

Append to `internal/platform/linear/handler_test.go`:

```go
func TestSelfComments_PersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()

	a, err := newSelfComments(dir)
	if err != nil {
		t.Fatalf("newSelfComments: %v", err)
	}
	a.Add("C1")
	a.Add("C2")
	a.Add("C1") // duplicate — must not double-write
	if !a.Has("C1") || !a.Has("C2") {
		t.Fatal("set missing IDs after Add")
	}

	// Simulate restart: a fresh instance pointed at the same dir must
	// reload the previously-recorded IDs.
	b, err := newSelfComments(dir)
	if err != nil {
		t.Fatalf("newSelfComments restart: %v", err)
	}
	if !b.Has("C1") || !b.Has("C2") {
		t.Errorf("after restart, set missing recorded IDs")
	}
	if b.Has("C-other") {
		t.Errorf("unexpected ID survived restart")
	}

	// File should contain exactly two lines (no duplicates from the duplicate Add).
	data, err := os.ReadFile(filepath.Join(dir, "self_comments.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %q", len(lines), data)
	}
}

func TestSelfComments_ConcurrentAddsAreAtomic(t *testing.T) {
	dir := t.TempDir()
	s, err := newSelfComments(dir)
	if err != nil {
		t.Fatalf("newSelfComments: %v", err)
	}
	const N = 200
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Add(fmt.Sprintf("C%d", i))
		}(i)
	}
	wg.Wait()
	for i := 0; i < N; i++ {
		if !s.Has(fmt.Sprintf("C%d", i)) {
			t.Fatalf("missing ID C%d", i)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "self_comments.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != N {
		t.Errorf("expected %d lines, got %d", N, len(lines))
	}
	seen := map[string]struct{}{}
	for _, l := range lines {
		if _, dup := seen[l]; dup {
			t.Errorf("duplicate line in log: %q", l)
		}
		seen[l] = struct{}{}
	}
}
```

Add these imports to the existing `import (...)` block in `handler_test.go` if missing: `"fmt"`, `"os"`, `"path/filepath"`, `"strings"`. The block already imports `"sync"`.

- [ ] **Step 2.2: Update existing handler test setup to use the new constructor**

In `handler_test.go`, replace every `newSelfComments()` call with a `t.TempDir()`-based one. Specifically, three call sites:

- `TestReceiver_TickOnce_DispatchesIssueAndComments` (line ~90):
  ```go
  self, err := newSelfComments(t.TempDir())
  if err != nil {
      t.Fatal(err)
  }
  self.Add("C-bot")
  ```

- `TestReceiver_TickOnce_ErrorDoesNotAdvanceCursor` (line ~129) — inline:
  ```go
  self, err := newSelfComments(t.TempDir())
  if err != nil {
      t.Fatal(err)
  }
  r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", self: self}
  ```

- `TestReceiver_TickOnce_TruncatedDoesNotAdvanceCursor` (line ~150) — same pattern as above.

- `TestSender_PostsCommentWithParentID` (line ~163):
  ```go
  self, err := newSelfComments(t.TempDir())
  if err != nil {
      t.Fatal(err)
  }
  s := &LinearSender{client: fc, self: self}
  ```

- `TestReceiver_TickOnce_DispatchesHumanCommentSharingBotUserID` (line ~217):
  ```go
  self, err := newSelfComments(t.TempDir())
  if err != nil {
      t.Fatal(err)
  }
  self.Add("C-bot")
  ```

- [ ] **Step 2.3: Run new tests to verify they fail**

Run: `go test ./internal/platform/linear/ -run TestSelfComments -v`
Expected: compile error — `newSelfComments` takes no arg yet.

- [ ] **Step 2.4: Make `selfComments` persistent**

In `internal/platform/linear/handler.go`, replace the `selfCommentsCap` constant and the `selfComments` struct (lines 32-72) with:

```go
// selfComments tracks comment IDs the bot has posted so the receiver can skip
// them on the next poll. The set is persisted to <dir>/self_comments.log
// (one ID per line, append-only) so a restart does not cause the bot to
// re-process its own replies.
type selfComments struct {
	mu  sync.Mutex
	set map[string]struct{}
	f   *os.File
}

func newSelfComments(dir string) (*selfComments, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("self_comments: mkdir: %w", err)
	}
	path := filepath.Join(dir, "self_comments.log")
	set := make(map[string]struct{})
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line != "" {
				set[line] = struct{}{}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("self_comments: read %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("self_comments: open %s: %w", path, err)
	}
	return &selfComments{set: set, f: f}, nil
}

func (s *selfComments) Add(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.set[id]; ok {
		return
	}
	if _, err := s.f.Write([]byte(id + "\n")); err != nil {
		// Log and skip — the in-memory set isn't updated either, so we'll
		// retry persisting if the same ID comes back through a later Send.
		log.Error("self_comments: write", zap.Error(err), zap.String("id", id))
		return
	}
	s.set[id] = struct{}{}
}

func (s *selfComments) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.set[id]
	return ok
}
```

Add `"strings"` to the import block. (`os`, `path/filepath`, `fmt`, `errors` are already present from Task 1.)

- [ ] **Step 2.5: Update `NewPlatform` to plumb dir into selfComments**

In `internal/platform/linear/handler.go`, change the body of `NewPlatform` so that:

```go
	self, err := newSelfComments(dir)
	if err != nil {
		return nil, err
	}
```

…replaces the previous `self := newSelfComments()` call. The full function now reads:

```go
func NewPlatform(cfg config.LinearConfig) (platform.Platform, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("linear: resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".linear")
	client := NewClient(cfg.APIKey)
	self, err := newSelfComments(dir)
	if err != nil {
		return nil, err
	}
	return &LinearPlatform{
		receiver: &LinearReceiver{
			client:       client,
			cursor:       NewCursor(dir),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
			self:         self,
		},
		sender: &LinearSender{client: client, self: self},
	}, nil
}
```

- [ ] **Step 2.6: Verify all linear package tests pass**

Run: `go test ./internal/platform/linear/ -v`
Expected: all tests pass, including the two new `TestSelfComments_*` tests and the updated existing tests.

- [ ] **Step 2.7: Verify whole module builds**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 2.8: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go
git commit -m "$(cat <<'EOF'
feat(linear): persist self-comment IDs to append-only log

Records bot-posted comment IDs to ~/.openbee/.linear/self_comments.log
so the receiver still recognizes them as bot output after a restart.
Removes the in-memory FIFO cap; retention is now permanent.
EOF
)"
```

---

## Task 3: Drop the obsolete DB key

**Files:**
- Modify: `internal/infra/model/system_config.go:19-20` (delete the constant)

- [ ] **Step 3.1: Confirm there are no remaining references**

Run: `grep -rn "SystemConfigKeyLinearLastSync\|linear\.last_sync_at" --include='*.go' .`
Expected: only `internal/infra/model/system_config.go:19-20` (and possibly the design/plan markdown files, which are fine).

If anything else shows up, stop and re-evaluate before deleting.

- [ ] **Step 3.2: Delete the constant**

In `internal/infra/model/system_config.go`, delete lines 19-20:

```go
// SystemConfigKeyLinearLastSync is the key for the Linear poller's high-water cursor (RFC3339 timestamp).
const SystemConfigKeyLinearLastSync = "linear.last_sync_at"
```

- [ ] **Step 3.3: Verify build and tests are clean**

Run: `go build ./... && go test ./internal/...`
Expected: clean build and all tests pass.

- [ ] **Step 3.4: Commit**

```bash
git add internal/infra/model/system_config.go
git commit -m "chore(linear): drop unused SystemConfigKeyLinearLastSync constant"
```

---

## Final Verification

- [ ] **Step 4.1: Full project build + test**

Run: `go build ./... && go test ./...`
Expected: clean build, all tests pass.

- [ ] **Step 4.2: Manual smoke check (optional, requires Linear creds)**

If a `config.yaml` is set up with Linear enabled, start the daemon, post a comment from the bot, then `cat ~/.openbee/.linear/self_comments.log` and `cat ~/.openbee/.linear/cursor.json`. Both files should exist and reflect the activity.
