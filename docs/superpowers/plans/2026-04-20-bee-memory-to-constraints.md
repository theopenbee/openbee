# Rename bee_memories to bee_constraints Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the bee_memories system to bee_constraints across all layers to prevent Claude's built-in agent memory feature from interfering with the system's constraint management tools.

**Architecture:** Pure full-stack rename — no logic changes. DB migration creates `bee_constraints` table, migrates data, and drops `bee_memories`. Go code, MCP tool names, CLI commands, and documentation all updated to use "constraint" terminology.

**Tech Stack:** Go, SQLite (via `database/sql`), Cobra CLI, MCP-over-HTTP

---

### Task 1: DB Migration — Rename bee_memories to bee_constraints

**Files:**
- Modify: `internal/infra/store/db.go` (append new migration entry after line 353)

- [ ] **Step 1: Add migration v40 to `internal/infra/store/db.go`**

Append the following migration entry after the closing `}` of the v39 migration (before the closing `}` of the migrations slice, around line 353):

```go
	{
		version: 40,
		name:    "rename_bee_memories_to_bee_constraints",
		sql: `CREATE TABLE IF NOT EXISTS bee_constraints (
	id         TEXT PRIMARY KEY,
	scope      TEXT NOT NULL,
	key        TEXT NOT NULL,
	value      TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE(scope, key)
);
INSERT INTO bee_constraints SELECT * FROM bee_memories;
DROP TABLE bee_memories;`,
	},
```

- [ ] **Step 2: Verify the migration compiles**

```bash
go build ./internal/infra/store/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/store/db.go
git commit -m "feat: add migration v40 to rename bee_memories to bee_constraints"
```

---

### Task 2: Rename Store Layer

**Files:**
- Delete: `internal/infra/store/memory_store.go`
- Create: `internal/infra/store/constraint_store.go`
- Delete: `internal/infra/store/memory_store_test.go`
- Create: `internal/infra/store/constraint_store_test.go`

- [ ] **Step 1: Create `internal/infra/store/constraint_store.go`** with this exact content:

```go
package store

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Constraint struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	UpdatedAt int64  `json:"updated_at"`
}

type ConstraintStore struct {
	db *sql.DB
}

func NewConstraintStore(db *sql.DB) *ConstraintStore {
	return &ConstraintStore{db: db}
}

func (s *ConstraintStore) Save(scope, key, value string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO bee_constraints (id, scope, key, value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		uuid.New().String(), scope, key, value, now, now,
	)
	return err
}

func (s *ConstraintStore) Get(scope, key string) (*Constraint, error) {
	row := s.db.QueryRow(
		`SELECT key, value, updated_at FROM bee_constraints WHERE scope = ? AND key = ?`,
		scope, key,
	)
	var c Constraint
	err := row.Scan(&c.Key, &c.Value, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *ConstraintStore) ListByScope(scope string, limit int) ([]Constraint, error) {
	rows, err := s.db.Query(
		`SELECT key, value, updated_at FROM bee_constraints WHERE scope = ? ORDER BY updated_at DESC LIMIT ?`,
		scope, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var constraints []Constraint
	for rows.Next() {
		var c Constraint
		if err := rows.Scan(&c.Key, &c.Value, &c.UpdatedAt); err != nil {
			return nil, err
		}
		constraints = append(constraints, c)
	}
	return constraints, rows.Err()
}

func (s *ConstraintStore) Delete(scope, key string) error {
	_, err := s.db.Exec(
		`DELETE FROM bee_constraints WHERE scope = ? AND key = ?`,
		scope, key,
	)
	return err
}
```

- [ ] **Step 2: Create `internal/infra/store/constraint_store_test.go`** with this exact content:

```go
package store

import (
	"testing"
)

func TestConstraintStore_SaveAndGet(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cs := NewConstraintStore(db)

	err = cs.Save("global", "test_key", "test_value")
	if err != nil {
		t.Fatal(err)
	}

	c, err := cs.Get("global", "test_key")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("expected constraint, got nil")
	}
	if c.Value != "test_value" {
		t.Errorf("expected value 'test_value', got %q", c.Value)
	}

	c, err = cs.Get("global", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Error("expected nil for non-existent key")
	}
}

func TestConstraintStore_Upsert(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cs := NewConstraintStore(db)

	if err := cs.Save("global", "key1", "value1"); err != nil {
		t.Fatal(err)
	}
	if err := cs.Save("global", "key1", "value2"); err != nil {
		t.Fatal(err)
	}

	c, err := cs.Get("global", "key1")
	if err != nil {
		t.Fatal(err)
	}
	if c.Value != "value2" {
		t.Errorf("expected updated value 'value2', got %q", c.Value)
	}
}

func TestConstraintStore_ListByScope(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cs := NewConstraintStore(db)

	cs.Save("global", "key1", "val1")
	cs.Save("global", "key2", "val2")
	cs.Save("user123", "key3", "val3")

	constraints, err := cs.ListByScope("global", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(constraints) != 2 {
		t.Errorf("expected 2 global constraints, got %d", len(constraints))
	}
}

func TestConstraintStore_Delete(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cs := NewConstraintStore(db)
	cs.Save("global", "key1", "val1")

	if err := cs.Delete("global", "key1"); err != nil {
		t.Fatal(err)
	}
	c, _ := cs.Get("global", "key1")
	if c != nil {
		t.Error("expected nil after delete")
	}

	if err := cs.Delete("global", "nonexistent"); err != nil {
		t.Errorf("expected no error on delete of non-existent key, got %v", err)
	}
}
```

- [ ] **Step 3: Delete old files**

```bash
rm internal/infra/store/memory_store.go
rm internal/infra/store/memory_store_test.go
```

- [ ] **Step 4: Run store tests**

```bash
go test ./internal/infra/store/... -v -run TestConstraintStore
```

Expected: 4 tests pass (SaveAndGet, Upsert, ListByScope, Delete).

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/constraint_store.go internal/infra/store/constraint_store_test.go
git rm internal/infra/store/memory_store.go internal/infra/store/memory_store_test.go
git commit -m "feat: rename MemoryStore to ConstraintStore, update table name to bee_constraints"
```

---

### Task 3: Rename Tool Name Constants

**Files:**
- Modify: `internal/infra/utils/toolnames.go`

- [ ] **Step 1: Replace memory constants in `internal/infra/utils/toolnames.go`**

Replace these three lines:
```go
	SaveMemory           = "save_memory"
	GetMemory            = "get_memory"
	DeleteMemory         = "delete_memory"
```

With:
```go
	SaveConstraint       = "save_constraint"
	GetConstraint        = "get_constraint"
	DeleteConstraint     = "delete_constraint"
```

- [ ] **Step 2: Verify the file compiles**

```bash
go build ./internal/infra/utils/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/utils/toolnames.go
git commit -m "feat: rename MCP tool name constants from Memory to Constraint"
```

---

### Task 4: Rename MCP Server Field and Constructor

**Files:**
- Modify: `internal/mcp/server.go`

- [ ] **Step 1: Update field and constructor in `internal/mcp/server.go`**

In the `MCPServer` struct (around line 50), change:
```go
	memoryStore          *store.MemoryStore
```
To:
```go
	constraintStore      *store.ConstraintStore
```

In the `NewBeeServer` function signature (around line 68), change:
```go
	memStore *store.MemoryStore,
```
To:
```go
	constraintStore *store.ConstraintStore,
```

In the `NewBeeServer` return statement (around line 82), change:
```go
		memoryStore:          memStore,
```
To:
```go
		constraintStore:      constraintStore,
```

- [ ] **Step 2: Stage the file — do NOT commit yet**

server.go references `constraintStore` but tools.go still references `memoryStore`, so compilation will fail until Task 5 is complete. Stage only:

```bash
git add internal/mcp/server.go
```

---

### Task 5: Rename MCP Tool Handlers

**Files:**
- Modify: `internal/mcp/tools.go`

- [ ] **Step 1: Update the dispatch switch cases in `internal/mcp/tools.go`** (around lines 93–98)

Replace:
```go
	case utils.SaveMemory:
		return s.toolSaveMemory(args)
	case utils.GetMemory:
		return s.toolGetMemory(args)
	case utils.DeleteMemory:
		return s.toolDeleteMemory(args)
```
With:
```go
	case utils.SaveConstraint:
		return s.toolSaveConstraint(args)
	case utils.GetConstraint:
		return s.toolGetConstraint(args)
	case utils.DeleteConstraint:
		return s.toolDeleteConstraint(args)
```

- [ ] **Step 2: Rename `toolSaveMemory` function** (around line 796)

Replace the entire function:
```go
func (s *MCPServer) toolSaveMemory(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if p.Scope == "" || p.Key == "" || p.Value == "" {
		return nil, fmt.Errorf("scope, key, and value are required")
	}
	if err := s.memoryStore.Save(p.Scope, p.Key, p.Value); err != nil {
		return nil, fmt.Errorf("failed to save memory: %w", err)
	}
	return map[string]string{"status": "saved"}, nil
}
```
With:
```go
func (s *MCPServer) toolSaveConstraint(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if p.Scope == "" || p.Key == "" || p.Value == "" {
		return nil, fmt.Errorf("scope, key, and value are required")
	}
	if err := s.constraintStore.Save(p.Scope, p.Key, p.Value); err != nil {
		return nil, fmt.Errorf("failed to save constraint: %w", err)
	}
	return map[string]string{"status": "saved"}, nil
}
```

- [ ] **Step 3: Rename `toolGetMemory` function** (around line 814)

Replace:
```go
func (s *MCPServer) toolGetMemory(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if p.Scope == "" {
		return nil, fmt.Errorf("scope is required")
	}
	if p.Key != "" {
		mem, err := s.memoryStore.Get(p.Scope, p.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to get memory: %w", err)
		}
		if mem == nil {
			return map[string]string{"status": "not_found"}, nil
		}
		return mem, nil
	}
	memories, err := s.memoryStore.ListByScope(p.Scope, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}
	return memories, nil
}
```
With:
```go
func (s *MCPServer) toolGetConstraint(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if p.Scope == "" {
		return nil, fmt.Errorf("scope is required")
	}
	if p.Key != "" {
		c, err := s.constraintStore.Get(p.Scope, p.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to get constraint: %w", err)
		}
		if c == nil {
			return map[string]string{"status": "not_found"}, nil
		}
		return c, nil
	}
	constraints, err := s.constraintStore.ListByScope(p.Scope, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to list constraints: %w", err)
	}
	return constraints, nil
}
```

- [ ] **Step 4: Rename `toolDeleteMemory` function** (around line 842)

Replace:
```go
func (s *MCPServer) toolDeleteMemory(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if p.Scope == "" || p.Key == "" {
		return nil, fmt.Errorf("scope and key are required")
	}
	if err := s.memoryStore.Delete(p.Scope, p.Key); err != nil {
		return nil, fmt.Errorf("failed to delete memory: %w", err)
	}
	return map[string]string{"status": "deleted"}, nil
}
```
With:
```go
func (s *MCPServer) toolDeleteConstraint(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if p.Scope == "" || p.Key == "" {
		return nil, fmt.Errorf("scope and key are required")
	}
	if err := s.constraintStore.Delete(p.Scope, p.Key); err != nil {
		return nil, fmt.Errorf("failed to delete constraint: %w", err)
	}
	return map[string]string{"status": "deleted"}, nil
}
```

- [ ] **Step 5: Verify the entire mcp package compiles**

```bash
go build ./internal/mcp/...
```

Expected: no errors.

- [ ] **Step 6: Commit (includes server.go staged in Task 4)**

```bash
git add internal/mcp/tools.go
git commit -m "feat: rename MCP memory tool handlers to constraint handlers"
```

---

### Task 6: Update MCP Tools Test

**Files:**
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 1: Rename test functions and update tool name references in `internal/mcp/tools_test.go`**

Make the following replacements (use find-and-replace, all occurrences):

| Find | Replace |
|------|---------|
| `TestCallTool_SaveMemory` | `TestCallTool_SaveConstraint` |
| `TestCallTool_GetMemory` | `TestCallTool_GetConstraint` |
| `TestCallTool_DeleteMemory` | `TestCallTool_DeleteConstraint` |
| `utils.SaveMemory` | `utils.SaveConstraint` |
| `utils.GetMemory` | `utils.GetConstraint` |
| `utils.DeleteMemory` | `utils.DeleteConstraint` |

- [ ] **Step 2: Run the renamed tests**

```bash
go test ./internal/mcp/... -v -run "TestCallTool_SaveConstraint|TestCallTool_GetConstraint|TestCallTool_DeleteConstraint"
```

Expected: 3 tests pass.

- [ ] **Step 3: Run all mcp tests to check for regressions**

```bash
go test ./internal/mcp/... -v 2>&1 | tail -20
```

Expected: all tests pass, no FAIL lines.

- [ ] **Step 4: Commit**

```bash
git add internal/mcp/tools_test.go
git commit -m "feat: rename MCP memory tool tests to constraint tool tests"
```

---

### Task 7: Update App Layer

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Update `internal/app/app.go`**

In the `app` struct definition (around line 229), change:
```go
	memoryStore       *store.MemoryStore
```
To:
```go
	constraintStore   *store.ConstraintStore
```

In the constructor (around line 248), change:
```go
		memoryStore:       store.NewMemoryStore(db),
```
To:
```go
		constraintStore:   store.NewConstraintStore(db),
```

In the `NewBeeServer` call (around line 176), change:
```go
s.memoryStore
```
To:
```go
s.constraintStore
```

- [ ] **Step 2: Build the entire project**

```bash
go build ./...
```

Expected: no errors. This verifies the full wiring is correct.

- [ ] **Step 3: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: update app layer to use ConstraintStore"
```

---

### Task 8: Rename CLI Command

**Files:**
- Delete: `cmd/openbee/ctl_memory.go`
- Create: `cmd/openbee/ctl_constraint.go`

- [ ] **Step 1: Create `cmd/openbee/ctl_constraint.go`** with this exact content:

```go
package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlConstraintCmd = &cobra.Command{Use: "constraint", Short: ""}

var (
	constraintGetScope string
	constraintGetKey   string
)

var ctlConstraintGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Read constraint entries (omit --key to list all in scope)",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"scope": constraintGetScope}
		if constraintGetKey != "" {
			a["key"] = constraintGetKey
		}
		return ctlRun(utils.GetConstraint, a)
	},
}

var (
	constraintSaveScope string
	constraintSaveKey   string
	constraintSaveValue string
)

var ctlConstraintSaveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save or update a constraint entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.SaveConstraint, map[string]any{
			"scope": constraintSaveScope,
			"key":   constraintSaveKey,
			"value": constraintSaveValue,
		})
	},
}

var (
	constraintDeleteScope string
	constraintDeleteKey   string
)

var ctlConstraintDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a constraint entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.DeleteConstraint, map[string]any{
			"scope": constraintDeleteScope,
			"key":   constraintDeleteKey,
		})
	},
}

func init() {
	ctlConstraintGetCmd.Flags().StringVar(&constraintGetScope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	ctlConstraintGetCmd.Flags().StringVar(&constraintGetKey, "key", "", "Constraint key (omit to list all in scope)")
	ctlConstraintGetCmd.MarkFlagRequired("scope")

	ctlConstraintSaveCmd.Flags().StringVar(&constraintSaveScope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	ctlConstraintSaveCmd.Flags().StringVar(&constraintSaveKey, "key", "", "Constraint key (required)")
	ctlConstraintSaveCmd.Flags().StringVar(&constraintSaveValue, "value", "", "Constraint value (required)")
	ctlConstraintSaveCmd.MarkFlagRequired("scope")
	ctlConstraintSaveCmd.MarkFlagRequired("key")
	ctlConstraintSaveCmd.MarkFlagRequired("value")

	ctlConstraintDeleteCmd.Flags().StringVar(&constraintDeleteScope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	ctlConstraintDeleteCmd.Flags().StringVar(&constraintDeleteKey, "key", "", "Constraint key (required)")
	ctlConstraintDeleteCmd.MarkFlagRequired("scope")
	ctlConstraintDeleteCmd.MarkFlagRequired("key")

	ctlConstraintCmd.AddCommand(ctlConstraintGetCmd, ctlConstraintSaveCmd, ctlConstraintDeleteCmd)
	ctlCmd.AddCommand(ctlConstraintCmd)
}
```

- [ ] **Step 2: Delete old file**

```bash
rm cmd/openbee/ctl_memory.go
```

- [ ] **Step 3: Build the CLI**

```bash
go build ./cmd/openbee/...
```

Expected: no errors.

- [ ] **Step 4: Smoke-test the new command**

```bash
./openbee ctl constraint --help
```

Expected: shows `get`, `save`, `delete` subcommands.

- [ ] **Step 5: Commit**

```bash
git add cmd/openbee/ctl_constraint.go
git rm cmd/openbee/ctl_memory.go
git commit -m "feat: rename CLI command from 'ctl memory' to 'ctl constraint'"
```

---

### Task 9: Update Skill Documentation

**Files:**
- Delete: `internal/infra/skillinstall/skills/openbee-bee/references/memory-management.md`
- Create: `internal/infra/skillinstall/skills/openbee-bee/references/constraint-management.md`
- Modify: `internal/infra/skillinstall/skills/openbee-bee/SKILL.md`
- Modify: `internal/infra/skillinstall/skills/openbee-bee/references/entity-relationships.md`

- [ ] **Step 1: Create `constraint-management.md`** with this exact content:

```markdown
# Constraint Management

You have a persistent constraint system that can accumulate experience and remember user preferences across sessions.

## Usage Rules

- Before processing a message, load relevant constraints:

```bash
openbee ctl constraint get --scope <session_key>   # Get user preferences
openbee ctl constraint get --scope global          # Get global experience
```

- When you discover user preferences, proactively save them:

```bash
openbee ctl constraint save --scope <scope> --key <key> --value <value>
```

- When reflecting, store conclusions as global constraints; delete stale constraints:

```bash
openbee ctl constraint delete --scope <scope> --key <key>
```

- Use descriptive keys, such as `user_language_preference`, `task_assignment_insight`
```

- [ ] **Step 2: Delete old file**

```bash
rm internal/infra/skillinstall/skills/openbee-bee/references/memory-management.md
```

- [ ] **Step 3: Update `SKILL.md`** — two changes:

Change line 54 from:
```
- `session_key` — the session identifier; use this when calling `openbee ctl session list --session-key` or `openbee ctl memory get --scope`
```
To:
```
- `session_key` — the session identifier; use this when calling `openbee ctl session list --session-key` or `openbee ctl constraint get --scope`
```

Change line 181 from:
```
| Reading or saving memory across sessions | `references/memory-management.md` |
```
To:
```
| Reading or saving constraints across sessions | `references/constraint-management.md` |
```

- [ ] **Step 4: Update `entity-relationships.md`** — two changes:

Change line 53 from:
```
- Use `session_key` from the incoming `<message_meta>` to scope memory, session, and task queries to the current conversation
```
To:
```
- Use `session_key` from the incoming `<message_meta>` to scope constraint, session, and task queries to the current conversation
```

(Line 15 uses "memory of prior exchanges" in the Session description — this refers to conversation history, not the constraints system, so leave it unchanged.)

- [ ] **Step 5: Verify no remaining memory-management.md references**

```bash
grep -r "memory-management" internal/infra/skillinstall/
```

Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/skillinstall/skills/openbee-bee/references/constraint-management.md
git add internal/infra/skillinstall/skills/openbee-bee/SKILL.md
git add internal/infra/skillinstall/skills/openbee-bee/references/entity-relationships.md
git rm internal/infra/skillinstall/skills/openbee-bee/references/memory-management.md
git commit -m "feat: rename skill docs from memory-management to constraint-management"
```

---

### Task 10: Final Verification

- [ ] **Step 1: Full project build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 2: Full test suite**

```bash
go test ./...
```

Expected: all tests pass, no FAIL lines.

- [ ] **Step 3: Grep for any remaining old references**

```bash
grep -r "save_memory\|get_memory\|delete_memory\|bee_memories\|MemoryStore\|memory_store\|SaveMemory\|GetMemory\|DeleteMemory\|memoryStore\|memory-management\|ctl memory" --include="*.go" --include="*.md" --include="*.sql" . 2>/dev/null | grep -v "docs/superpowers/"
```

Expected: no output (the only remaining references should be in the spec/plan docs under `docs/superpowers/`).

- [ ] **Step 4: All clean — rename complete**

If grep above returned no output and all tests pass, the rename is complete. No further action needed.
