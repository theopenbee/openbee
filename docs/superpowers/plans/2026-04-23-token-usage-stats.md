# Token Usage Statistics Storage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a background sync job that reads JSONL session files from Claude Code, Codex, and Pi agents and stores aggregated token usage in SQLite, keyed by `(session_id, model)`.

**Architecture:** A `TokenStatsSyncer` goroutine runs every 10 minutes, querying `bee_executions` for active sessions and dispatching per-engine JSONL parsers. On first run (empty `bee_token_stats` table), all historical sessions are processed; subsequently only sessions with activity in the last 30 days. Results are upserted via `ON CONFLICT`.

**Tech Stack:** Go 1.25, SQLite (`modernc.org/sqlite`), `database/sql`, `bufio.Scanner`, `encoding/json`, `go.uber.org/zap`

---

## File Map

```
internal/infra/store/db.go                       modify  — add migration #41
internal/infra/model/token_stats.go              create
internal/infra/store/token_stats_store.go        create
internal/infra/store/token_stats_store_test.go   create
internal/tokenstat/parser.go                     create
internal/tokenstat/claude.go                     create
internal/tokenstat/claude_test.go                create
internal/tokenstat/codex.go                      create
internal/tokenstat/codex_test.go                 create
internal/tokenstat/pi.go                         create
internal/tokenstat/pi_test.go                    create
internal/tokenstat/syncer.go                     create
internal/tokenstat/syncer_test.go                create
internal/app/app.go                              modify  — wire up syncer
```

---

## Task 1: DB Migration + Token Stats Model

**Files:**
- Modify: `internal/infra/store/db.go`
- Create: `internal/infra/model/token_stats.go`

- [ ] **Step 1: Add migration #41 in `internal/infra/store/db.go`**

Locate the `migrations` slice. The current last entry is `version: 40`. Append directly after it:

```go
{
    version: 41,
    name:    "create_bee_token_stats",
    sql: `
        CREATE TABLE IF NOT EXISTS bee_token_stats (
            id                    TEXT    PRIMARY KEY,
            session_id            TEXT    NOT NULL,
            agent_type            TEXT    NOT NULL,
            model                 TEXT    NOT NULL,
            input_tokens          INTEGER NOT NULL DEFAULT 0,
            output_tokens         INTEGER NOT NULL DEFAULT 0,
            cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
            cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
            synced_at             INTEGER NOT NULL
        );
        CREATE UNIQUE INDEX IF NOT EXISTS idx_bee_token_stats_session_model
            ON bee_token_stats(session_id, model);
        CREATE INDEX IF NOT EXISTS idx_bee_token_stats_session_id
            ON bee_token_stats(session_id);
    `,
},
```

- [ ] **Step 2: Create `internal/infra/model/token_stats.go`**

```go
package model

type TokenStats struct {
	ID                  string `json:"id" db:"id"`
	SessionID           string `json:"session_id" db:"session_id"`
	AgentType           string `json:"agent_type" db:"agent_type"`
	Model               string `json:"model" db:"model"`
	InputTokens         int64  `json:"input_tokens" db:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens" db:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens" db:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens" db:"cache_read_tokens"`
	SyncedAt            int64  `json:"synced_at" db:"synced_at"`
}
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/infra/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/store/db.go internal/infra/model/token_stats.go
git commit -m "feat(tokenstat): add bee_token_stats table migration and model"
```

---

## Task 2: Token Stats Store

**Files:**
- Create: `internal/infra/store/token_stats_store_test.go`
- Create: `internal/infra/store/token_stats_store.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/infra/store/token_stats_store_test.go`:

```go
package store_test

import (
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func newTokenStatsTestDB(t *testing.T) (*TokenStatsStore, func()) {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	return NewTokenStatsStore(db), func() { db.Close() }
}

func TestTokenStatsStore_IsEmpty_WhenEmpty(t *testing.T) {
	s, cleanup := newTokenStatsTestDB(t)
	defer cleanup()

	empty, err := s.IsEmpty()
	if err != nil {
		t.Fatalf("IsEmpty: %v", err)
	}
	if !empty {
		t.Error("expected empty store to return true")
	}
}

func TestTokenStatsStore_Upsert_InsertsRecord(t *testing.T) {
	s, cleanup := newTokenStatsTestDB(t)
	defer cleanup()

	if err := s.Upsert(model.TokenStats{
		SessionID:           "session-1",
		AgentType:           "claude",
		Model:               "claude-3-5-sonnet",
		InputTokens:         100,
		OutputTokens:        200,
		CacheCreationTokens: 50,
		CacheReadTokens:     30,
		SyncedAt:            time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	empty, _ := s.IsEmpty()
	if empty {
		t.Error("expected non-empty store after insert")
	}
}

func TestTokenStatsStore_Upsert_UpdatesOnConflict(t *testing.T) {
	s, cleanup := newTokenStatsTestDB(t)
	defer cleanup()

	base := model.TokenStats{
		SessionID: "session-1", AgentType: "claude", Model: "claude-3-5-sonnet",
		InputTokens: 100, OutputTokens: 200, SyncedAt: time.Now().UnixMilli(),
	}
	s.Upsert(base)

	updated := model.TokenStats{
		SessionID: "session-1", AgentType: "claude", Model: "claude-3-5-sonnet",
		InputTokens: 500, OutputTokens: 600, SyncedAt: time.Now().UnixMilli(),
	}
	if err := s.Upsert(updated); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	got, err := s.GetBySessionID("session-1")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	if got[0].InputTokens != 500 {
		t.Errorf("InputTokens: want 500, got %d", got[0].InputTokens)
	}
}

func TestTokenStatsStore_Upsert_MultipleModelsPerSession(t *testing.T) {
	s, cleanup := newTokenStatsTestDB(t)
	defer cleanup()

	for _, m := range []string{"claude-3-5-sonnet", "claude-3-opus"} {
		if err := s.Upsert(model.TokenStats{
			SessionID: "session-1", AgentType: "claude", Model: m,
			InputTokens: 100, SyncedAt: time.Now().UnixMilli(),
		}); err != nil {
			t.Fatalf("Upsert %s: %v", m, err)
		}
	}

	got, err := s.GetBySessionID("session-1")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 records (one per model), got %d", len(got))
	}
}
```

- [ ] **Step 2: Run to verify compile failure**

```bash
go test ./internal/infra/store/... -run TestTokenStatsStore -v
```

Expected: compile error — `TokenStatsStore`, `NewTokenStatsStore`, `GetBySessionID` not defined.

- [ ] **Step 3: Create `internal/infra/store/token_stats_store.go`**

```go
package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type TokenStatsStore struct {
	db *sql.DB
}

func NewTokenStatsStore(db *sql.DB) *TokenStatsStore {
	return &TokenStatsStore{db: db}
}

func (s *TokenStatsStore) IsEmpty() (bool, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats`).Scan(&count); err != nil {
		return false, fmt.Errorf("count token stats: %w", err)
	}
	return count == 0, nil
}

func (s *TokenStatsStore) Upsert(stat model.TokenStats) error {
	if stat.ID == "" {
		stat.ID = uuid.New().String()
	}
	if stat.SyncedAt == 0 {
		stat.SyncedAt = time.Now().UnixMilli()
	}
	_, err := s.db.Exec(
		`INSERT INTO bee_token_stats
		     (id, session_id, agent_type, model, input_tokens, output_tokens,
		      cache_creation_tokens, cache_read_tokens, synced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(session_id, model) DO UPDATE SET
		     agent_type            = excluded.agent_type,
		     input_tokens          = excluded.input_tokens,
		     output_tokens         = excluded.output_tokens,
		     cache_creation_tokens = excluded.cache_creation_tokens,
		     cache_read_tokens     = excluded.cache_read_tokens,
		     synced_at             = excluded.synced_at`,
		stat.ID, stat.SessionID, stat.AgentType, stat.Model,
		stat.InputTokens, stat.OutputTokens,
		stat.CacheCreationTokens, stat.CacheReadTokens,
		stat.SyncedAt,
	)
	return err
}

func (s *TokenStatsStore) GetBySessionID(sessionID string) ([]model.TokenStats, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, agent_type, model, input_tokens, output_tokens,
		        cache_creation_tokens, cache_read_tokens, synced_at
		 FROM bee_token_stats WHERE session_id = ?`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query token stats by session: %w", err)
	}
	defer rows.Close()
	var stats []model.TokenStats
	for rows.Next() {
		var st model.TokenStats
		if err := rows.Scan(
			&st.ID, &st.SessionID, &st.AgentType, &st.Model,
			&st.InputTokens, &st.OutputTokens,
			&st.CacheCreationTokens, &st.CacheReadTokens, &st.SyncedAt,
		); err != nil {
			return nil, fmt.Errorf("scan token stats: %w", err)
		}
		stats = append(stats, st)
	}
	return stats, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/infra/store/... -run TestTokenStatsStore -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/token_stats_store.go internal/infra/store/token_stats_store_test.go
git commit -m "feat(tokenstat): add TokenStatsStore with upsert and query"
```

---

## Task 3: Parser Types

**Files:**
- Create: `internal/tokenstat/parser.go`

- [ ] **Step 1: Create `internal/tokenstat/parser.go`**

```go
package tokenstat

// SessionTokenUsage holds aggregated token usage for one (session, model) pair.
type SessionTokenUsage struct {
	SessionID           string
	AgentType           string
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// Parser reads a session's JSONL file(s) and returns per-model token usage.
type Parser interface {
	Parse(sessionID string) ([]SessionTokenUsage, error)
}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/tokenstat/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/tokenstat/parser.go
git commit -m "feat(tokenstat): add Parser interface and SessionTokenUsage type"
```

---

## Task 4: Claude Code Parser

**Files:**
- Create: `internal/tokenstat/claude_test.go`
- Create: `internal/tokenstat/claude.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/tokenstat/claude_test.go`:

```go
package tokenstat_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/theopenbee/openbee/internal/tokenstat"
)

// writeTempFile creates a file at dir/name with content. Used by all parser tests.
func writeTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestClaudeParser_Parse_AggregatesByModel(t *testing.T) {
	base := t.TempDir()
	writeTempFile(t, base, "projects/test-session.jsonl", `{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}}
{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":5}}}
{"message":{"model":"claude-3-opus","usage":{"input_tokens":300,"output_tokens":150}}}
{"timestamp":"2025-01-01T00:00:00Z"}
`)
	t.Setenv("CLAUDE_CONFIG_DIR", base)
	parser := tokenstat.NewClaudeParser()

	usages, err := parser.Parse("test-session")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	byModel := map[string]tokenstat.SessionTokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	sonnet := byModel["claude-3-5-sonnet"]
	if sonnet.InputTokens != 300 {
		t.Errorf("sonnet InputTokens: want 300, got %d", sonnet.InputTokens)
	}
	if sonnet.OutputTokens != 150 {
		t.Errorf("sonnet OutputTokens: want 150, got %d", sonnet.OutputTokens)
	}
	if sonnet.CacheCreationTokens != 20 {
		t.Errorf("sonnet CacheCreationTokens: want 20, got %d", sonnet.CacheCreationTokens)
	}
	if sonnet.CacheReadTokens != 15 {
		t.Errorf("sonnet CacheReadTokens: want 15, got %d", sonnet.CacheReadTokens)
	}
	if sonnet.AgentType != "claude" {
		t.Errorf("sonnet AgentType: want claude, got %s", sonnet.AgentType)
	}

	opus := byModel["claude-3-opus"]
	if opus.InputTokens != 300 {
		t.Errorf("opus InputTokens: want 300, got %d", opus.InputTokens)
	}
}

func TestClaudeParser_Parse_FastSpeedSuffix(t *testing.T) {
	base := t.TempDir()
	writeTempFile(t, base, "projects/fast-session.jsonl",
		`{"message":{"model":"claude-3-5-sonnet","speed":"fast","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n")
	t.Setenv("CLAUDE_CONFIG_DIR", base)

	usages, err := tokenstat.NewClaudeParser().Parse("fast-session")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].Model != "claude-3-5-sonnet-fast" {
		t.Errorf("Model: want claude-3-5-sonnet-fast, got %s", usages[0].Model)
	}
}

func TestClaudeParser_Parse_FileNotFound(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	_, err := tokenstat.NewClaudeParser().Parse("nonexistent-session")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
```

- [ ] **Step 2: Run to verify compile failure**

```bash
go test ./internal/tokenstat/... -run TestClaudeParser -v
```

Expected: compile error — `tokenstat.NewClaudeParser` not defined.

- [ ] **Step 3: Create `internal/tokenstat/claude.go`**

```go
package tokenstat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type claudeParser struct {
	baseDirs []string
}

func NewClaudeParser() Parser {
	return &claudeParser{baseDirs: claudeBaseDirs()}
}

func claudeBaseDirs() []string {
	if env := os.Getenv("CLAUDE_CONFIG_DIR"); env != "" {
		parts := strings.Split(env, ",")
		dirs := make([]string, 0, len(parts))
		for _, p := range parts {
			dirs = append(dirs, strings.TrimSpace(p))
		}
		return dirs
	}
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".config", "claude"),
	}
}

type claudeJSONLLine struct {
	Message struct {
		Model string `json:"model"`
		Speed string `json:"speed"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func (p *claudeParser) Parse(sessionID string) ([]SessionTokenUsage, error) {
	for _, base := range p.baseDirs {
		path := filepath.Join(base, "projects", sessionID+".jsonl")
		if _, err := os.Stat(path); err == nil {
			return claudeParse(sessionID, path)
		}
	}
	return nil, fmt.Errorf("claude session file not found for %s", sessionID)
}

func claudeParse(sessionID, path string) ([]SessionTokenUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open claude session file: %w", err)
	}
	defer f.Close()

	agg := map[string]*SessionTokenUsage{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 16*1024*1024), 16*1024*1024)

	for scanner.Scan() {
		var line claudeJSONLLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if line.Message.Model == "" || line.Message.Usage == nil {
			continue
		}
		m := line.Message.Model
		if line.Message.Speed == "fast" {
			m += "-fast"
		}
		u := getOrCreate(agg, sessionID, "claude", m)
		u.InputTokens += line.Message.Usage.InputTokens
		u.OutputTokens += line.Message.Usage.OutputTokens
		u.CacheCreationTokens += line.Message.Usage.CacheCreationInputTokens
		u.CacheReadTokens += line.Message.Usage.CacheReadInputTokens
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan claude session file: %w", err)
	}
	return mapValues(agg), nil
}

// getOrCreate returns an existing aggregation entry or initializes a new one.
func getOrCreate(agg map[string]*SessionTokenUsage, sessionID, agentType, model string) *SessionTokenUsage {
	if u, ok := agg[model]; ok {
		return u
	}
	u := &SessionTokenUsage{SessionID: sessionID, AgentType: agentType, Model: model}
	agg[model] = u
	return u
}

// mapValues converts the aggregation map to a slice.
func mapValues(agg map[string]*SessionTokenUsage) []SessionTokenUsage {
	result := make([]SessionTokenUsage, 0, len(agg))
	for _, u := range agg {
		result = append(result, *u)
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tokenstat/... -run TestClaudeParser -v
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tokenstat/claude.go internal/tokenstat/claude_test.go
git commit -m "feat(tokenstat): add Claude Code JSONL parser"
```

---

## Task 5: Codex Parser

**Files:**
- Create: `internal/tokenstat/codex_test.go`
- Create: `internal/tokenstat/codex.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/tokenstat/codex_test.go`:

```go
package tokenstat_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/theopenbee/openbee/internal/tokenstat"
)

func TestCodexParser_Parse_WithLastTokenUsage(t *testing.T) {
	base := t.TempDir()
	mappingDir := filepath.Join(base, "mapping")
	codexBase := filepath.Join(base, "codex")
	os.MkdirAll(mappingDir, 0755)
	os.MkdirAll(filepath.Join(codexBase, "sessions"), 0755)

	os.WriteFile(filepath.Join(mappingDir, "openbee-sess-1"), []byte("codex-real-sess-1\n"), 0644)
	writeTempFile(t, filepath.Join(codexBase, "sessions"), "codex-real-sess-1.jsonl", `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","info":{"last_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":20}}}
{"type":"event_msg","info":{"last_token_usage":{"input_tokens":200,"output_tokens":80,"cached_input_tokens":10}}}
{"type":"turn_context","payload":{"model":"o1-mini"}}
{"type":"event_msg","info":{"last_token_usage":{"input_tokens":300,"output_tokens":100,"cached_input_tokens":0}}}
`)
	t.Setenv("CODEX_HOME", codexBase)
	parser := tokenstat.NewCodexParser(mappingDir)

	usages, err := parser.Parse("openbee-sess-1")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	byModel := map[string]tokenstat.SessionTokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	gpt4o := byModel["gpt-4o"]
	if gpt4o.InputTokens != 300 {
		t.Errorf("gpt-4o InputTokens: want 300, got %d", gpt4o.InputTokens)
	}
	if gpt4o.OutputTokens != 130 {
		t.Errorf("gpt-4o OutputTokens: want 130, got %d", gpt4o.OutputTokens)
	}
	if gpt4o.CacheReadTokens != 30 {
		t.Errorf("gpt-4o CacheReadTokens: want 30, got %d", gpt4o.CacheReadTokens)
	}
	if gpt4o.CacheCreationTokens != 0 {
		t.Errorf("gpt-4o CacheCreationTokens: want 0, got %d", gpt4o.CacheCreationTokens)
	}
	if gpt4o.AgentType != "codex" {
		t.Errorf("gpt-4o AgentType: want codex, got %s", gpt4o.AgentType)
	}

	o1mini := byModel["o1-mini"]
	if o1mini.InputTokens != 300 {
		t.Errorf("o1-mini InputTokens: want 300, got %d", o1mini.InputTokens)
	}
}

func TestCodexParser_Parse_DeltaFromTotalTokenUsage(t *testing.T) {
	base := t.TempDir()
	mappingDir := filepath.Join(base, "mapping")
	codexBase := filepath.Join(base, "codex")
	os.MkdirAll(mappingDir, 0755)
	os.MkdirAll(filepath.Join(codexBase, "sessions"), 0755)

	os.WriteFile(filepath.Join(mappingDir, "openbee-sess-2"), []byte("codex-real-sess-2"), 0644)
	writeTempFile(t, filepath.Join(codexBase, "sessions"), "codex-real-sess-2.jsonl", `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","info":{"total_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":10}}}
{"type":"event_msg","info":{"total_token_usage":{"input_tokens":250,"output_tokens":120,"cached_input_tokens":25}}}
`)
	t.Setenv("CODEX_HOME", codexBase)

	usages, err := tokenstat.NewCodexParser(mappingDir).Parse("openbee-sess-2")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	// total values are cumulative; final total is the result
	if usages[0].InputTokens != 250 {
		t.Errorf("InputTokens: want 250, got %d", usages[0].InputTokens)
	}
	if usages[0].OutputTokens != 120 {
		t.Errorf("OutputTokens: want 120, got %d", usages[0].OutputTokens)
	}
}

func TestCodexParser_Parse_MappingFileNotFound(t *testing.T) {
	mappingDir := t.TempDir()
	t.Setenv("CODEX_HOME", t.TempDir())
	_, err := tokenstat.NewCodexParser(mappingDir).Parse("nonexistent-session")
	if err == nil {
		t.Error("expected error for missing mapping file, got nil")
	}
}
```

- [ ] **Step 2: Run to verify compile failure**

```bash
go test ./internal/tokenstat/... -run TestCodexParser -v
```

Expected: compile error — `tokenstat.NewCodexParser` not defined.

- [ ] **Step 3: Create `internal/tokenstat/codex.go`**

```go
package tokenstat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type codexParser struct {
	mappingDir string
	codexBase  string
}

// NewCodexParser creates a Codex parser. mappingDir is the directory containing
// openbee→codex session ID mapping files (typically ~/.openbee/.codex/sessions).
func NewCodexParser(mappingDir string) Parser {
	codexBase := os.Getenv("CODEX_HOME")
	if codexBase == "" {
		home, _ := os.UserHomeDir()
		codexBase = filepath.Join(home, ".codex")
	}
	return &codexParser{mappingDir: mappingDir, codexBase: codexBase}
}

type codexJSONLLine struct {
	Type    string `json:"type"`
	Payload struct {
		Model string `json:"model"`
	} `json:"payload"`
	Info struct {
		Model     string `json:"model"`
		ModelName string `json:"model_name"`
		Metadata  struct {
			Model string `json:"model"`
		} `json:"metadata"`
		LastTokenUsage *struct {
			InputTokens       int64 `json:"input_tokens"`
			OutputTokens      int64 `json:"output_tokens"`
			CachedInputTokens int64 `json:"cached_input_tokens"`
		} `json:"last_token_usage"`
		TotalTokenUsage *struct {
			InputTokens       int64 `json:"input_tokens"`
			OutputTokens      int64 `json:"output_tokens"`
			CachedInputTokens int64 `json:"cached_input_tokens"`
		} `json:"total_token_usage"`
	} `json:"info"`
}

func (p *codexParser) Parse(sessionID string) ([]SessionTokenUsage, error) {
	data, err := os.ReadFile(filepath.Join(p.mappingDir, sessionID))
	if err != nil {
		return nil, fmt.Errorf("read codex mapping for session %s: %w", sessionID, err)
	}
	codexSessionID := strings.TrimSpace(string(data))
	if codexSessionID == "" {
		return nil, fmt.Errorf("empty codex session id in mapping for %s", sessionID)
	}
	path := filepath.Join(p.codexBase, "sessions", codexSessionID+".jsonl")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("codex session file not found: %s", path)
	}
	return codexParse(sessionID, path)
}

func codexParse(sessionID, path string) ([]SessionTokenUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open codex session file: %w", err)
	}
	defer f.Close()

	agg := map[string]*SessionTokenUsage{}
	currentModel := ""
	var prevInput, prevOutput, prevCached int64

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	for scanner.Scan() {
		var line codexJSONLLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		switch line.Type {
		case "turn_context":
			if line.Payload.Model != "" {
				currentModel = line.Payload.Model
			}
		case "event_msg":
			m := codexResolveModel(line, currentModel)
			if m == "" {
				continue
			}
			u := getOrCreate(agg, sessionID, "codex", m)
			if line.Info.LastTokenUsage != nil {
				ltu := line.Info.LastTokenUsage
				u.InputTokens += ltu.InputTokens
				u.OutputTokens += ltu.OutputTokens
				u.CacheReadTokens += ltu.CachedInputTokens
			} else if line.Info.TotalTokenUsage != nil {
				ttu := line.Info.TotalTokenUsage
				u.InputTokens += ttu.InputTokens - prevInput
				u.OutputTokens += ttu.OutputTokens - prevOutput
				u.CacheReadTokens += ttu.CachedInputTokens - prevCached
				prevInput = ttu.InputTokens
				prevOutput = ttu.OutputTokens
				prevCached = ttu.CachedInputTokens
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan codex session file: %w", err)
	}
	return mapValues(agg), nil
}

func codexResolveModel(line codexJSONLLine, currentModel string) string {
	if line.Info.Model != "" {
		return line.Info.Model
	}
	if line.Info.ModelName != "" {
		return line.Info.ModelName
	}
	if line.Info.Metadata.Model != "" {
		return line.Info.Metadata.Model
	}
	return currentModel
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tokenstat/... -run TestCodexParser -v
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tokenstat/codex.go internal/tokenstat/codex_test.go
git commit -m "feat(tokenstat): add Codex JSONL parser"
```

---

## Task 6: Pi Agent Parser

**Files:**
- Create: `internal/tokenstat/pi_test.go`
- Create: `internal/tokenstat/pi.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/tokenstat/pi_test.go`:

```go
package tokenstat_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/theopenbee/openbee/internal/tokenstat"
)

func TestPiParser_Parse_AggregatesByModel(t *testing.T) {
	sessionsDir := t.TempDir()
	sessionID := "pi-sess-abc123"

	writeTempFile(t, sessionsDir, "20250101_"+sessionID+".jsonl", `{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50,"cacheWrite":10,"cacheRead":5}}}
{"type":"message","message":{"role":"user","content":"hello"}}
{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":200,"output":80,"cacheWrite":0,"cacheRead":15}}}
{"type":"message","message":{"role":"assistant","model":"claude-3-opus","usage":{"input":300,"output":100,"cacheWrite":5,"cacheRead":0}}}
{"type":"other","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
`)
	t.Setenv("PI_AGENT_DIR", sessionsDir)
	parser := tokenstat.NewPiParser()

	usages, err := parser.Parse(sessionID)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	byModel := map[string]tokenstat.SessionTokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	sonnet := byModel["[pi]claude-3-5-sonnet"]
	if sonnet.InputTokens != 300 {
		t.Errorf("sonnet InputTokens: want 300, got %d", sonnet.InputTokens)
	}
	if sonnet.OutputTokens != 130 {
		t.Errorf("sonnet OutputTokens: want 130, got %d", sonnet.OutputTokens)
	}
	if sonnet.CacheCreationTokens != 10 {
		t.Errorf("sonnet CacheCreationTokens: want 10, got %d", sonnet.CacheCreationTokens)
	}
	if sonnet.CacheReadTokens != 20 {
		t.Errorf("sonnet CacheReadTokens: want 20, got %d", sonnet.CacheReadTokens)
	}
	if sonnet.AgentType != "pi" {
		t.Errorf("sonnet AgentType: want pi, got %s", sonnet.AgentType)
	}

	opus := byModel["[pi]claude-3-opus"]
	if opus.InputTokens != 300 {
		t.Errorf("opus InputTokens: want 300, got %d", opus.InputTokens)
	}
}

func TestPiParser_Parse_SkipsNonAssistantAndWrongType(t *testing.T) {
	sessionsDir := t.TempDir()
	sessionID := "skip-test"

	writeTempFile(t, sessionsDir, "ts_"+sessionID+".jsonl", `{"type":"message","message":{"role":"user","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50}}}
{"type":"other","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
`)
	t.Setenv("PI_AGENT_DIR", sessionsDir)

	usages, err := tokenstat.NewPiParser().Parse(sessionID)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage (only assistant+message), got %d", len(usages))
	}
	if usages[0].InputTokens != 100 {
		t.Errorf("InputTokens: want 100, got %d", usages[0].InputTokens)
	}
}

func TestPiParser_Parse_FileNotFound(t *testing.T) {
	t.Setenv("PI_AGENT_DIR", t.TempDir())
	_, err := tokenstat.NewPiParser().Parse("nonexistent-session")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
```

- [ ] **Step 2: Run to verify compile failure**

```bash
go test ./internal/tokenstat/... -run TestPiParser -v
```

Expected: compile error — `tokenstat.NewPiParser` not defined.

- [ ] **Step 3: Create `internal/tokenstat/pi.go`**

```go
package tokenstat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type piParser struct {
	sessionsDir string
}

func NewPiParser() Parser {
	dir := os.Getenv("PI_AGENT_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".pi", "agent", "sessions")
	}
	return &piParser{sessionsDir: dir}
}

type piJSONLLine struct {
	Type    string `json:"type"`
	Message struct {
		Role  string `json:"role"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input"`
			Output     int64 `json:"output"`
			CacheWrite int64 `json:"cacheWrite"`
			CacheRead  int64 `json:"cacheRead"`
		} `json:"usage"`
	} `json:"message"`
}

func (p *piParser) Parse(sessionID string) ([]SessionTokenUsage, error) {
	entries, err := os.ReadDir(p.sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("read pi sessions dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".jsonl")
		idx := strings.Index(name, "_")
		if idx == -1 || name[idx+1:] != sessionID {
			continue
		}
		return piParse(sessionID, filepath.Join(p.sessionsDir, entry.Name()))
	}
	return nil, fmt.Errorf("pi session file not found for session %s", sessionID)
}

func piParse(sessionID, path string) ([]SessionTokenUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open pi session file: %w", err)
	}
	defer f.Close()

	agg := map[string]*SessionTokenUsage{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	for scanner.Scan() {
		var line piJSONLLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if line.Type != "message" || line.Message.Role != "assistant" || line.Message.Usage == nil {
			continue
		}
		m := "[pi]" + line.Message.Model
		u := getOrCreate(agg, sessionID, "pi", m)
		u.InputTokens += line.Message.Usage.Input
		u.OutputTokens += line.Message.Usage.Output
		u.CacheCreationTokens += line.Message.Usage.CacheWrite
		u.CacheReadTokens += line.Message.Usage.CacheRead
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan pi session file: %w", err)
	}
	return mapValues(agg), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tokenstat/... -run TestPiParser -v
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tokenstat/pi.go internal/tokenstat/pi_test.go
git commit -m "feat(tokenstat): add Pi Agent JSONL parser"
```

---

## Task 7: Token Stats Syncer

**Files:**
- Create: `internal/tokenstat/syncer_test.go`
- Create: `internal/tokenstat/syncer.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/tokenstat/syncer_test.go`:

```go
package tokenstat_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/tokenstat"
)

func newSyncerTestDB(t *testing.T) (*sql.DB, *store.TokenStatsStore, func()) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	return db, store.NewTokenStatsStore(db), func() { db.Close() }
}

func insertTestWorker(t *testing.T, db *sql.DB, id, engine string) {
	t.Helper()
	now := time.Now().UnixMilli()
	_, err := db.Exec(
		`INSERT INTO bee_workers (id, name, description, constraints, work_dir, engine, status, permission_scopes, created_at, updated_at)
		 VALUES (?, ?, '', '', '/tmp', ?, 'idle', '', ?, ?)`,
		id, "worker-"+id, engine, now, now,
	)
	if err != nil {
		t.Fatalf("insert worker: %v", err)
	}
}

func insertTestExecution(t *testing.T, db *sql.DB, workerID, sessionID string, completedAt int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO bee_executions (id, worker_id, session_id, status, completed_at)
		 VALUES (?, ?, ?, 'completed', ?)`,
		"exec-"+sessionID, workerID, sessionID, completedAt,
	)
	if err != nil {
		t.Fatalf("insert execution: %v", err)
	}
}

func TestSyncer_SyncOnce_Claude(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "worker-1", "claude")
	insertTestExecution(t, db, "worker-1", "test-session", time.Now().UnixMilli())

	claudeBase := t.TempDir()
	os.MkdirAll(filepath.Join(claudeBase, "projects"), 0755)
	os.WriteFile(
		filepath.Join(claudeBase, "projects", "test-session.jsonl"),
		[]byte(`{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n"),
		0644,
	)
	t.Setenv("CLAUDE_CONFIG_DIR", claudeBase)

	syncer := tokenstat.NewSyncer(db, tokenStore)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("test-session")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat record, got %d", len(stats))
	}
	if stats[0].InputTokens != 100 {
		t.Errorf("InputTokens: want 100, got %d", stats[0].InputTokens)
	}
	if stats[0].AgentType != "claude" {
		t.Errorf("AgentType: want claude, got %s", stats[0].AgentType)
	}
}

func TestSyncer_SyncOnce_FullModeWhenTableEmpty(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	// Old execution (60 days ago) — only picked up in full mode, skipped in incremental
	insertTestWorker(t, db, "worker-old", "claude")
	oldTime := time.Now().AddDate(0, 0, -60).UnixMilli()
	insertTestExecution(t, db, "worker-old", "old-session", oldTime)

	claudeBase := t.TempDir()
	os.MkdirAll(filepath.Join(claudeBase, "projects"), 0755)
	os.WriteFile(
		filepath.Join(claudeBase, "projects", "old-session.jsonl"),
		[]byte(`{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":50,"output_tokens":25}}}`+"\n"),
		0644,
	)
	t.Setenv("CLAUDE_CONFIG_DIR", claudeBase)

	syncer := tokenstat.NewSyncer(db, tokenStore)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("old-session")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Errorf("expected 1 stat (full mode on empty table), got %d", len(stats))
	}
}
```

- [ ] **Step 2: Run to verify compile failure**

```bash
go test ./internal/tokenstat/... -run TestSyncer -v
```

Expected: compile error — `tokenstat.NewSyncer` not defined.

- [ ] **Step 3: Create `internal/tokenstat/syncer.go`**

```go
package tokenstat

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"go.uber.org/zap"
)

const (
	syncInterval    = 10 * time.Minute
	incrementalDays = 30
)

type Syncer struct {
	db         *sql.DB
	tokenStore *store.TokenStatsStore
	parsers    map[string]Parser
}

func NewSyncer(db *sql.DB, tokenStore *store.TokenStatsStore) *Syncer {
	home, _ := os.UserHomeDir()
	mappingDir := filepath.Join(home, ".openbee", ".codex", "sessions")
	return &Syncer{
		db:         db,
		tokenStore: tokenStore,
		parsers: map[string]Parser{
			"claude": NewClaudeParser(),
			"codex":  NewCodexParser(mappingDir),
			"pi":     NewPiParser(),
		},
	}
}

func (s *Syncer) Run(ctx context.Context) {
	s.SyncOnce(ctx)
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.SyncOnce(ctx)
		}
	}
}

func (s *Syncer) SyncOnce(ctx context.Context) {
	sessions, err := s.collectSessions(ctx)
	if err != nil {
		logger.Error("tokenstat: collect sessions", zap.Error(err))
		return
	}
	for _, item := range sessions {
		if err := s.syncSession(item.sessionID, item.engine); err != nil {
			logger.Warn("tokenstat: sync session",
				zap.String("session_id", item.sessionID),
				zap.String("engine", item.engine),
				zap.Error(err))
		}
	}
}

type sessionItem struct {
	sessionID string
	engine    string
}

func (s *Syncer) collectSessions(ctx context.Context) ([]sessionItem, error) {
	empty, err := s.tokenStore.IsEmpty()
	if err != nil {
		return nil, err
	}

	var (
		rows  *sql.Rows
		query string
		args  []any
	)
	if empty {
		query = `
			SELECT DISTINCT e.session_id, w.engine
			FROM bee_executions e
			JOIN bee_workers w ON w.id = e.worker_id
			WHERE w.engine IN ('claude', 'codex', 'pi') AND e.worker_id IS NOT NULL`
	} else {
		since := time.Now().AddDate(0, 0, -incrementalDays).UnixMilli()
		query = `
			SELECT DISTINCT e.session_id, w.engine
			FROM bee_executions e
			JOIN bee_workers w ON w.id = e.worker_id
			WHERE w.engine IN ('claude', 'codex', 'pi')
			  AND e.worker_id IS NOT NULL
			  AND e.completed_at > ?`
		args = []any{since}
	}

	rows, err = s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []sessionItem
	for rows.Next() {
		var item sessionItem
		if err := rows.Scan(&item.sessionID, &item.engine); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Syncer) syncSession(sessionID, engine string) error {
	parser, ok := s.parsers[engine]
	if !ok {
		return nil
	}
	usages, err := parser.Parse(sessionID)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, u := range usages {
		if err := s.tokenStore.Upsert(model.TokenStats{
			SessionID:           u.SessionID,
			AgentType:           u.AgentType,
			Model:               u.Model,
			InputTokens:         u.InputTokens,
			OutputTokens:        u.OutputTokens,
			CacheCreationTokens: u.CacheCreationTokens,
			CacheReadTokens:     u.CacheReadTokens,
			SyncedAt:            now,
		}); err != nil {
			logger.Error("tokenstat: upsert",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tokenstat tests to verify they pass**

```bash
go test ./internal/tokenstat/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tokenstat/syncer.go internal/tokenstat/syncer_test.go
git commit -m "feat(tokenstat): add TokenStatsSyncer background job"
```

---

## Task 8: Wire Up in App

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add `tokenStatsStore` to `appStores` struct**

In `internal/app/app.go`, locate the `appStores` struct (line ~215). Add the new field after `statsStore`:

```go
type appStores struct {
	workerStore       *store.WorkerStore
	envConfigStore    *store.EnvConfigStore
	systemConfigStore *store.SystemConfigStore
	execStore         *store.ExecutionStore
	msgStore          *store.MessageStore
	taskStore         *store.TaskStore
	sessionStore      *store.SessionStore
	outboundMsgStore  *store.OutboundMessageStore
	constraintStore   *store.ConstraintStore
	departmentStore   *store.DepartmentStore
	statsStore        *store.StatsStore
	tokenStatsStore   *store.TokenStatsStore
}
```

- [ ] **Step 2: Add `NewTokenStatsStore` in `buildStores`**

In the `buildStores` function return value (line ~234), add after `statsStore`:

```go
return db, appStores{
	workerStore:       store.NewWorkerStore(db),
	envConfigStore:    store.NewEnvConfigStore(db),
	systemConfigStore: store.NewSystemConfigStore(db),
	execStore:         store.NewExecutionStore(db, config.DefaultLogsDir()),
	msgStore:          store.NewMessageStore(db),
	taskStore:         store.NewTaskStore(db),
	sessionStore:      store.NewSessionStore(db),
	outboundMsgStore:  store.NewOutboundMessageStore(db),
	constraintStore:   store.NewConstraintStore(db),
	departmentStore:   store.NewDepartmentStore(db),
	statsStore:        store.NewStatsStore(db),
	tokenStatsStore:   store.NewTokenStatsStore(db),
}, nil
```

- [ ] **Step 3: Add syncer to runners**

In the function that assembles `runners` (line ~177), add after the `for _, p := range platforms` loop (line ~196) and before `localChatHandler`:

```go
tokenSyncer := tokenstat.NewSyncer(db, s.tokenStatsStore)
runners = append(runners, func(ctx context.Context) { tokenSyncer.Run(ctx) })
```

Also add the import at the top of the file:

```go
"github.com/theopenbee/openbee/internal/tokenstat"
```

- [ ] **Step 4: Build entire project**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Run all tests**

```bash
go test ./... -count=1
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(tokenstat): wire TokenStatsSyncer into app startup"
```
