# Remove `openbee ctl system executions` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete the `openbee ctl system executions` CLI subcommand, the `list_bee_executions` MCP tool, and the `ExecutionStore.ListBeeExecutions` store method end-to-end, including tests, skill/CLI/i18n docs, and the changelog.

**Architecture:** Pure deletion across the layered stack (CLI → RPC tool dispatch → store → docs). Order tasks outermost-in so each intermediate compile remains green: CLI first (uses constant + store method indirectly via RPC), then RPC handler + dispatch + RPC test (uses constant and store method), then store method + store test, then the tool-name constant, then docs / i18n, then changelog.

**Tech Stack:** Go (cobra CLI, sqlite store), YAML (i18n + skill refs), Markdown (skill docs + CHANGELOG).

**Spec:** `docs/superpowers/specs/2026-05-31-remove-system-executions-design.md`

---

## File Structure

Files modified (deletions only — no new files):

- `cmd/openbee/ctl_system.go` — drop `executions` subcommand + flag.
- `internal/rpc/tools.go` — drop `toolListBeeExecutions` handler and its dispatch `case`.
- `internal/rpc/tools_test.go` — drop `TestCallTool_ListBeeExecutions`.
- `internal/infra/store/execution_store.go` — drop `ListBeeExecutions` method.
- `internal/infra/store/execution_store_test.go` — drop `TestExecutionStore_ListBeeExecutions`.
- `internal/infra/utils/toolnames.go` — drop `ListBeeExecutions` constant.
- `internal/infra/skillinstall/skills/openbee-bee/references/system-status.md` — drop command + self-reflection bullet.
- `internal/infra/skillinstall/skills/openbee-bee/references/cli-reference.md` — drop command entry.
- `internal/infra/i18n/locales/en.yaml` — rewrite `ctl_system.short`.
- `internal/infra/i18n/locales/zh.yaml` — rewrite `ctl_system.short`.
- `CHANGELOG.md` — append removal entry to `[0.0.38]` block.

---

### Task 1: Remove the `executions` CLI subcommand

**Files:**

- Modify: `cmd/openbee/ctl_system.go`

- [ ] **Step 1: Read the current file**

Run: read `cmd/openbee/ctl_system.go` to confirm current contents match the expected starting state below.

Expected starting state:

```go
package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlSystemCmd = &cobra.Command{Use: "system", Short: ""}

var ctlSystemOverviewCmd = &cobra.Command{
	Use:   "overview",
	Short: "Show system overview: worker status distribution, task stats, recent executions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.GetSystemOverview, nil)
	},
}

var executionsLimit int

var ctlSystemExecutionsCmd = &cobra.Command{
	Use:   "executions",
	Short: "List bee execution history",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{}
		if executionsLimit > 0 {
			a["limit"] = executionsLimit
		}
		return ctlRun(utils.ListBeeExecutions, a)
	},
}

func init() {
	ctlSystemExecutionsCmd.Flags().IntVar(&executionsLimit, "limit", 0, "Number of records to return (0 = server default of 10)")

	ctlSystemCmd.AddCommand(ctlSystemOverviewCmd, ctlSystemExecutionsCmd)
	ctlCmd.AddCommand(ctlSystemCmd)
}
```

- [ ] **Step 2: Rewrite the file**

Replace the entire file contents with:

```go
package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlSystemCmd = &cobra.Command{Use: "system", Short: ""}

var ctlSystemOverviewCmd = &cobra.Command{
	Use:   "overview",
	Short: "Show system overview: worker status distribution, task stats, recent executions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.GetSystemOverview, nil)
	},
}

func init() {
	ctlSystemCmd.AddCommand(ctlSystemOverviewCmd)
	ctlCmd.AddCommand(ctlSystemCmd)
}
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: succeeds with no output.

- [ ] **Step 4: Verify the subcommand is gone**

Run: `go run ./cmd/openbee ctl system --help 2>&1 | grep -i executions`
Expected: no output (exit status 1 from grep is fine; the `executions` line must not appear).

Also run: `go run ./cmd/openbee ctl system --help`
Expected: the `Available Commands:` block lists only `overview`.

- [ ] **Step 5: Commit**

```bash
git add cmd/openbee/ctl_system.go
git commit -m "feat: remove ctl system executions subcommand"
```

---

### Task 2: Remove the RPC dispatch case, handler, and its test

**Files:**

- Modify: `internal/rpc/tools.go` (drop dispatch `case` at `:91-92` and `toolListBeeExecutions` function at `:833-870`)
- Modify: `internal/rpc/tools_test.go` (drop `TestCallTool_ListBeeExecutions` at `:669-680`)

These are deleted together because removing only the handler leaves the dispatch `case` referring to a missing function (compile error), and removing only the dispatch leaves the test calling a tool name that the server cannot resolve.

- [ ] **Step 1: Delete the dispatch case in `internal/rpc/tools.go`**

Remove these two lines from `CallTool`'s switch (currently at lines 91-92):

```go
		case utils.ListBeeExecutions:
			return s.toolListBeeExecutions(args)
```

The surrounding case for `GetSystemOverview` (above) and `SaveConstraint` (below) must remain.

- [ ] **Step 2: Delete the `toolListBeeExecutions` function in `internal/rpc/tools.go`**

Remove the entire function (currently at lines 833-870), including the blank line that separates it from the next function:

```go
func (s *Server) toolListBeeExecutions(args json.RawMessage) (any, error) {
	var p struct {
		Limit int `json:"limit"`
	}
	if args != nil {
		json.Unmarshal(args, &p) //nolint
	}
	if p.Limit <= 0 {
		p.Limit = 10
	}

	execs, err := s.executionStore.ListBeeExecutions(p.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list bee executions: %w", err)
	}

	results := make([]map[string]any, 0, len(execs))
	for _, e := range execs {
		triggerInput := e.TriggerInput
		if len(triggerInput) > 200 {
			triggerInput = triggerInput[:200]
		}
		result := e.Result
		if len(result) > 200 {
			result = result[:200]
		}
		results = append(results, map[string]any{
			"id":            e.ID,
			"trigger_input": triggerInput,
			"status":        string(e.Status),
			"started_at":    e.StartedAt,
			"completed_at":  e.CompletedAt,
			"result":        result,
		})
	}

	return results, nil
}
```

The function immediately below it (`toolSaveConstraint`) must remain unchanged.

- [ ] **Step 3: Delete `TestCallTool_ListBeeExecutions` in `internal/rpc/tools_test.go`**

Remove the entire test (currently at lines 669-680), including the trailing blank line that separates it from the next test:

```go
func TestCallTool_ListBeeExecutions(t *testing.T) {
	s := setupServerWithMessaging(t)

	result, err := s.CallTool(context.Background(), utils.ListBeeExecutions, nil)
	if err != nil {
		t.Fatal(err)
	}
	execs := result.([]map[string]any)
	if len(execs) != 0 {
		t.Errorf("expected empty list, got %d", len(execs))
	}
}
```

The next test (`TestCallTool_SaveConstraint`) must remain unchanged.

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 5: Run the RPC tests**

Run: `go test ./internal/rpc/...`
Expected: PASS. No reference to `TestCallTool_ListBeeExecutions` in the output.

- [ ] **Step 6: Verify the dispatch is gone**

Run: `git grep -n "ListBeeExecutions\|list_bee_executions" -- 'internal/rpc/**'`
Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add internal/rpc/tools.go internal/rpc/tools_test.go
git commit -m "feat: remove list_bee_executions rpc tool"
```

---

### Task 3: Remove the `ListBeeExecutions` store method and its test

**Files:**

- Modify: `internal/infra/store/execution_store.go` (drop method at `:472-483`)
- Modify: `internal/infra/store/execution_store_test.go` (drop `TestExecutionStore_ListBeeExecutions` at `:292-322`)

- [ ] **Step 1: Delete the `ListBeeExecutions` method in `internal/infra/store/execution_store.go`**

Remove the entire method (currently at lines 472-483), including the comment line above it and the trailing blank line:

```go
// ListBeeExecutions returns the bee's own execution history (worker_id IS NULL).
func (s *ExecutionStore) ListBeeExecutions(limit int) ([]model.WorkerExecution, error) {
	rows, err := s.db.Query(
		execSelect+` WHERE e.worker_id IS NULL ORDER BY e.started_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExecutions(rows)
}
```

The method immediately below (`ListRecent`) must remain unchanged.

- [ ] **Step 2: Verify no interface declares the method**

Run: `git grep -n "ListBeeExecutions" -- 'internal/**/*.go'`
Expected: no output. If any interface (e.g., a mock or store interface) still declares `ListBeeExecutions`, remove that declaration as well.

- [ ] **Step 3: Delete `TestExecutionStore_ListBeeExecutions` in `internal/infra/store/execution_store_test.go`**

Remove the entire test (currently at lines 292-322), including the trailing blank line:

```go
func TestExecutionStore_ListBeeExecutions(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db, t.TempDir())

	// Create a bee execution (worker_id = NULL)
	bee1, err := es.Create("", "", "user said hello", "session1", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = bee1

	// Create a worker execution (should not appear)
	db.Exec(`INSERT INTO bee_workers (id, name, work_dir, status, created_at, updated_at) VALUES ('w1','test','/tmp','idle',0,0)`)
	_, err = es.Create("w1", "", "worker task", "session2", "claude")
	if err != nil {
		t.Fatal(err)
	}

	results, err := es.ListBeeExecutions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 bee execution, got %d", len(results))
	}
}
```

The next test (`TestExecutionStore_CreateBeeExecution`) must remain unchanged.

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 5: Run the store tests**

Run: `go test ./internal/infra/store/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/store/execution_store.go internal/infra/store/execution_store_test.go
git commit -m "feat: remove ExecutionStore.ListBeeExecutions"
```

---

### Task 4: Remove the `ListBeeExecutions` tool-name constant

**Files:**

- Modify: `internal/infra/utils/toolnames.go` (drop constant at `:17`)

By this point no code references `utils.ListBeeExecutions`, so removing the constant should compile clean.

- [ ] **Step 1: Delete the constant**

In `internal/infra/utils/toolnames.go`, remove this line (currently line 17):

```go
	ListBeeExecutions    = "list_bee_executions"
```

The surrounding constants (`GetSystemOverview` above, `SaveConstraint` below) must remain.

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 3: Test**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Repo-wide verification**

Run: `git grep -n "ListBeeExecutions\|list_bee_executions" -- '*.go'`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/utils/toolnames.go
git commit -m "feat: drop ListBeeExecutions tool-name constant"
```

---

### Task 5: Update the bee skill `system-status.md`

**Files:**

- Modify: `internal/infra/skillinstall/skills/openbee-bee/references/system-status.md`

Per the spec's chosen option (b), the self-reflection bullet is deleted entirely with no replacement guidance.

- [ ] **Step 1: Read current contents**

Expected current contents:

```markdown
# System Status Overview

You can view the system's running state to make better decisions.

```bash
# View worker current status
openbee ctl worker status <id>

# View overall system overview (worker distribution, task stats, recent executions)
openbee ctl system overview

# View your own execution history (can add --limit to restrict count)
openbee ctl system executions [--limit <n>]
```

## Usage Scenarios

- When the user asks about task status, use `worker status` or `system overview`
- When doing self-reflection, use `system executions` to review history, then directly read the log_path file in the returned result for details
- Before assigning tasks, you can check `system overview` to understand each worker's load
```

- [ ] **Step 2: Rewrite the file**

Replace the entire file contents with:

````markdown
# System Status Overview

You can view the system's running state to make better decisions.

```bash
# View worker current status
openbee ctl worker status <id>

# View overall system overview (worker distribution, task stats, recent executions)
openbee ctl system overview
```

## Usage Scenarios

- When the user asks about task status, use `worker status` or `system overview`
- Before assigning tasks, you can check `system overview` to understand each worker's load
````

- [ ] **Step 3: Verify**

Run: `git grep -n "system executions\|list_bee_executions" -- 'internal/infra/skillinstall/skills/openbee-bee/**'`
Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/skillinstall/skills/openbee-bee/references/system-status.md
git commit -m "docs: drop system executions from bee skill"
```

---

### Task 6: Update the bee skill `cli-reference.md`

**Files:**

- Modify: `internal/infra/skillinstall/skills/openbee-bee/references/cli-reference.md` (lines 86-91)

- [ ] **Step 1: Locate the `system` subcommand block**

Current block (lines 86-91):

```markdown
## system subcommand

```bash
openbee ctl system overview
openbee ctl system executions [--limit <count>]
```
```

- [ ] **Step 2: Edit the block**

Remove the `openbee ctl system executions [--limit <count>]` line so the block becomes:

```markdown
## system subcommand

```bash
openbee ctl system overview
```
```

Leave the surrounding markdown (e.g. the `## message subcommand` block that follows) untouched.

- [ ] **Step 3: Verify**

Run: `git grep -n "system executions" -- 'internal/infra/skillinstall/skills/openbee-bee/**'`
Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/skillinstall/skills/openbee-bee/references/cli-reference.md
git commit -m "docs: drop system executions from cli reference"
```

---

### Task 7: Update i18n short text for `ctl_system`

**Files:**

- Modify: `internal/infra/i18n/locales/en.yaml` (line 32)
- Modify: `internal/infra/i18n/locales/zh.yaml` (line 32)

- [ ] **Step 1: Edit `en.yaml`**

In `internal/infra/i18n/locales/en.yaml`, change:

```yaml
  ctl_system:
    short: "View system status and executions"
```

to:

```yaml
  ctl_system:
    short: "View system status overview"
```

- [ ] **Step 2: Edit `zh.yaml`**

In `internal/infra/i18n/locales/zh.yaml`, change:

```yaml
  ctl_system:
    short: "查看系统状态和执行记录"
```

to:

```yaml
  ctl_system:
    short: "查看系统状态概览"
```

- [ ] **Step 3: Build (i18n is embedded via Go embed)**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 4: Test**

Run: `go test ./internal/infra/i18n/...`
Expected: PASS (or no tests in package — that is also acceptable).

- [ ] **Step 5: Commit**

```bash
git add internal/infra/i18n/locales/en.yaml internal/infra/i18n/locales/zh.yaml
git commit -m "i18n: drop executions wording from ctl_system short"
```

---

### Task 8: Append CHANGELOG entry

**Files:**

- Modify: `CHANGELOG.md` (the `[0.0.38] - 2026-05-31 → ### Removed` block)

- [ ] **Step 1: Append to the `### Removed` list under `[0.0.38]`**

Current block (lines 5-8):

```markdown
### Removed

- Remove `openbee claude download` and `openbee claude env` subcommands
- Remove the `openbee ctl execution` subcommand, the `list_executions` tool, and the `read:executions` scope; execution records are now returned inline with each task by `ctl task list`
```

Add a new bullet at the end of that list so the block becomes:

```markdown
### Removed

- Remove `openbee claude download` and `openbee claude env` subcommands
- Remove the `openbee ctl execution` subcommand, the `list_executions` tool, and the `read:executions` scope; execution records are now returned inline with each task by `ctl task list`
- Remove the `openbee ctl system executions` subcommand, the `list_bee_executions` tool, and the bee self-reflection skill guidance that referenced them; bee execution history is no longer exposed via CLI or MCP, though `ctl system overview` still surfaces the most recent executions inline
```

Leave the `### Changed` block and the rest of the file untouched.

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog entry for system executions removal"
```

---

### Task 9: Final repo-wide verification

**Files:** none (verification only)

- [ ] **Step 1: Symbol scan**

Run: `git grep -n "ListBeeExecutions\|list_bee_executions" -- ':!docs/superpowers/' ':!CHANGELOG.md'`
Expected: no output. (The spec and plan under `docs/superpowers/` legitimately reference these names; the CHANGELOG bullet legitimately names `list_bee_executions`.)

- [ ] **Step 2: Command-string scan**

Run: `git grep -n "system executions" -- ':!docs/superpowers/' ':!CHANGELOG.md'`
Expected: no output.

- [ ] **Step 3: Full build**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 4: Full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Smoke-test the CLI**

Run: `go run ./cmd/openbee ctl system --help`
Expected: only the `overview` subcommand is listed.

Run: `go run ./cmd/openbee ctl system executions --help`
Expected: cobra error (`unknown command "executions" for "openbee ctl system"`).

- [ ] **Step 6: Confirm clean tree and inspect commit log**

Run: `git status`
Expected: working tree clean.

Run: `git log --oneline main..HEAD | head -15`
Expected: the new commits from Tasks 1-8 are visible at the top of the log in order.

No commit in this task — it is verification only.
