# Worker WorkDir Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow modifying a worker's `WorkDir` after creation, from both CLI (`openbee ctl worker update --work-dir`) and the web UI (Edit Worker Info sheet on the detail page).

**Architecture:** Add `WorkDir *string` to `UpdateWorkerParams`. CLI binds a new `--work-dir` flag. Web reuses the existing `EditWorkerInfoSheet`. New directory is created lazily on the next execution via `os.MkdirAll`. Old directory is left untouched. Validation is minimal (non-empty after trim).

**Tech Stack:** Go (domain + CLI), React + TypeScript + react-i18next (web), embedded skill docs (markdown).

**Spec:** [docs/superpowers/specs/2026-06-15-worker-workdir-update-design.md](../specs/2026-06-15-worker-workdir-update-design.md)

---

## File Map

| File | Action | Responsibility |
| --- | --- | --- |
| `internal/domain/worker/worker.go` | Modify | Add `WorkDir *string` to `UpdateWorkerParams`; update `HasChanges`, `Validate`, `ApplyTo` |
| `internal/domain/worker/execution.go` | Modify | Add `os.MkdirAll(worker.WorkDir, 0755)` before `engine.Prepare` |
| `internal/domain/worker/manager_test.go` | Modify | Add unit tests for new WorkDir update behavior |
| `cmd/openbee/internal/cli/ctlcmd/worker.go` | Modify | Add `--work-dir` flag to `worker update` |
| `internal/infra/skillinstall/skills/openbee-bee/references/cli-reference.md` | Modify | Document the new `--work-dir` flag on `worker update` |
| `web/src/components/edit-worker-info-sheet.tsx` | Modify | Add WorkDir field to the edit sheet |
| `web/src/locales/zh.json` | Modify | i18n labels (e.g. `workers.form.workDir`, helper) |
| `web/src/locales/en.json` | Modify | i18n labels |

**No new files.** `Worker` type, `useUpdateWorker` hook, and `api.workers.update` already accept `Partial<Worker>` (which contains `work_dir`), so types and API client need no changes.

---

## Task 1: Domain — extend UpdateWorkerParams with WorkDir

**Files:**
- Modify: `internal/domain/worker/worker.go:31-45,65-104`
- Test: `internal/domain/worker/manager_test.go` (append)

- [ ] **Step 1: Read the existing test file to learn the in-test Manager / store setup**

Run: `cat internal/domain/worker/manager_test.go | head -120`
Expected: shows how a `Manager` is instantiated for tests (likely via a helper such as `newTestManager(t)`), and how an existing worker is created in tests. Reuse the same helpers in the new tests below — do NOT invent a new helper.

If the file uses a helper like `newTestManager(t)` returning `(*Manager, …)`, use it. If it builds a Manager inline, copy that inline construction verbatim.

- [ ] **Step 2: Write the failing tests**

Append the following block to `internal/domain/worker/manager_test.go`. If the file already imports `strings`, `testing`, `os`, `path/filepath` (it likely does), omit duplicate imports.

```go
func TestUpdateWorker_WorkDir_Success(t *testing.T) {
    m, cleanup := newTestManager(t)
    defer cleanup()

    w, err := m.CreateWorker(CreateWorkerParams{Name: "wd-success", Engine: "claude"})
    if err != nil {
        t.Fatalf("create worker: %v", err)
    }

    newDir := filepath.Join(t.TempDir(), "moved")
    newDirPtr := newDir
    got, err := m.UpdateWorker(w.ID, UpdateWorkerParams{WorkDir: &newDirPtr})
    if err != nil {
        t.Fatalf("update worker: %v", err)
    }
    if got.WorkDir != newDir {
        t.Fatalf("WorkDir not updated: got %q want %q", got.WorkDir, newDir)
    }
}

func TestUpdateWorker_WorkDir_TrimWhitespace(t *testing.T) {
    m, cleanup := newTestManager(t)
    defer cleanup()

    w, err := m.CreateWorker(CreateWorkerParams{Name: "wd-trim", Engine: "claude"})
    if err != nil {
        t.Fatalf("create worker: %v", err)
    }

    padded := "   /tmp/openbee-x   "
    got, err := m.UpdateWorker(w.ID, UpdateWorkerParams{WorkDir: &padded})
    if err != nil {
        t.Fatalf("update worker: %v", err)
    }
    if got.WorkDir != "/tmp/openbee-x" {
        t.Fatalf("WorkDir not trimmed: got %q", got.WorkDir)
    }
}

func TestUpdateWorker_WorkDir_EmptyAfterTrim(t *testing.T) {
    m, cleanup := newTestManager(t)
    defer cleanup()

    w, err := m.CreateWorker(CreateWorkerParams{Name: "wd-empty", Engine: "claude"})
    if err != nil {
        t.Fatalf("create worker: %v", err)
    }

    blank := "   "
    _, err = m.UpdateWorker(w.ID, UpdateWorkerParams{WorkDir: &blank})
    if err == nil || !errors.Is(err, ErrValidation) {
        t.Fatalf("expected ErrValidation, got %v", err)
    }
}

func TestUpdateWorker_WorkDir_Nil_NoChange(t *testing.T) {
    m, cleanup := newTestManager(t)
    defer cleanup()

    w, err := m.CreateWorker(CreateWorkerParams{Name: "wd-nil", Engine: "claude"})
    if err != nil {
        t.Fatalf("create worker: %v", err)
    }
    originalDir := w.WorkDir

    newName := "wd-nil-renamed"
    got, err := m.UpdateWorker(w.ID, UpdateWorkerParams{Name: &newName})
    if err != nil {
        t.Fatalf("update worker: %v", err)
    }
    if got.WorkDir != originalDir {
        t.Fatalf("WorkDir changed unexpectedly: got %q want %q", got.WorkDir, originalDir)
    }
}

func TestUpdateWorker_WorkDir_OldDirUntouched(t *testing.T) {
    m, cleanup := newTestManager(t)
    defer cleanup()

    w, err := m.CreateWorker(CreateWorkerParams{Name: "wd-old", Engine: "claude"})
    if err != nil {
        t.Fatalf("create worker: %v", err)
    }
    if err := os.WriteFile(filepath.Join(w.WorkDir, "marker.txt"), []byte("keep me"), 0644); err != nil {
        t.Fatalf("write marker: %v", err)
    }

    newDir := filepath.Join(t.TempDir(), "elsewhere")
    _, err = m.UpdateWorker(w.ID, UpdateWorkerParams{WorkDir: &newDir})
    if err != nil {
        t.Fatalf("update worker: %v", err)
    }
    if _, err := os.Stat(filepath.Join(w.WorkDir, "marker.txt")); err != nil {
        t.Fatalf("old workdir file disappeared: %v", err)
    }
}
```

If the existing test file does not have a `newTestManager` helper, replace those calls with the same inline Manager construction the existing tests already use, then run those tests yourself to confirm — do NOT skip this verification.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/domain/worker/ -run TestUpdateWorker_WorkDir -v`
Expected: All five tests **fail to compile** with `unknown field WorkDir in struct literal of type UpdateWorkerParams`. This is the right kind of failure — proves the field does not exist yet.

- [ ] **Step 4: Add the field to UpdateWorkerParams**

In `internal/domain/worker/worker.go`, modify the `UpdateWorkerParams` struct (currently lines 31-39) and the three methods.

Locate this block:

```go
// UpdateWorkerParams holds the inputs for a partial worker update.
type UpdateWorkerParams struct {
    Name             *string `json:"name"`
    Description      *string `json:"description"`
    Constraints      *string `json:"constraints"`
    PermissionScopes *string `json:"permission_scopes"`
    Engine           *string `json:"engine"`
    // nil = no change; empty map clears all; per-engine empty string deletes that entry.
    EngineArgs map[string]string `json:"engine_args"`
}
```

Replace with:

```go
// UpdateWorkerParams holds the inputs for a partial worker update.
type UpdateWorkerParams struct {
    Name             *string `json:"name"`
    Description      *string `json:"description"`
    Constraints      *string `json:"constraints"`
    WorkDir          *string `json:"work_dir"`
    PermissionScopes *string `json:"permission_scopes"`
    Engine           *string `json:"engine"`
    // nil = no change; empty map clears all; per-engine empty string deletes that entry.
    EngineArgs map[string]string `json:"engine_args"`
}
```

- [ ] **Step 5: Extend HasChanges**

Locate `HasChanges` (currently lines 41-44):

```go
func (p UpdateWorkerParams) HasChanges() bool {
    return p.Name != nil || p.Description != nil || p.Constraints != nil ||
        p.PermissionScopes != nil || p.Engine != nil || p.EngineArgs != nil
}
```

Replace with:

```go
func (p UpdateWorkerParams) HasChanges() bool {
    return p.Name != nil || p.Description != nil || p.Constraints != nil ||
        p.WorkDir != nil || p.PermissionScopes != nil || p.Engine != nil ||
        p.EngineArgs != nil
}
```

- [ ] **Step 6: Extend Validate**

Locate `Validate` (currently lines 46-63). Add a `WorkDir` check at the top before the existing checks:

```go
func (p UpdateWorkerParams) Validate(m *Manager) error {
    if p.WorkDir != nil {
        if strings.TrimSpace(*p.WorkDir) == "" {
            return fmt.Errorf("work_dir cannot be empty: %w", ErrValidation)
        }
    }
    if p.PermissionScopes != nil {
        if err := auth.ValidatePermissionScopes(*p.PermissionScopes); err != nil {
            return err
        }
    }
    if p.Engine != nil {
        if err := m.ValidateEngine(*p.Engine); err != nil {
            return err
        }
    }
    if p.EngineArgs != nil {
        if err := m.ValidateEngineArgs(p.EngineArgs); err != nil {
            return err
        }
    }
    return nil
}
```

- [ ] **Step 7: Extend ApplyTo**

Locate `ApplyTo` (currently lines 65-104). Add the WorkDir branch right after `Constraints`:

```go
func (p UpdateWorkerParams) ApplyTo(w *model.Worker) error {
    if p.Name != nil {
        w.Name = *p.Name
    }
    if p.Description != nil {
        w.Description = *p.Description
    }
    if p.Constraints != nil {
        w.Constraints = *p.Constraints
    }
    if p.WorkDir != nil {
        w.WorkDir = strings.TrimSpace(*p.WorkDir)
    }
    if p.PermissionScopes != nil {
        w.PermissionScopes = *p.PermissionScopes
    }
    if p.Engine != nil {
        w.Engine = *p.Engine
    }
    if p.EngineArgs == nil {
        return nil
    }
    if len(p.EngineArgs) == 0 {
        w.EngineArgs = "{}"
        return nil
    }
    existing := make(map[string]string)
    if w.EngineArgs != "" && w.EngineArgs != "{}" {
        if err := json.Unmarshal([]byte(w.EngineArgs), &existing); err != nil {
            return fmt.Errorf("parse existing engine_args: %w", err)
        }
    }
    for engine, args := range p.EngineArgs {
        if args == "" {
            delete(existing, engine)
        } else {
            existing[engine] = args
        }
    }
    b, _ := json.Marshal(existing)
    w.EngineArgs = string(b)
    return nil
}
```

`strings` is already imported in this file — do not duplicate the import.

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/domain/worker/ -run TestUpdateWorker_WorkDir -v`
Expected: All five tests **PASS**.

- [ ] **Step 9: Run full domain tests for safety**

Run: `go test ./internal/domain/worker/...`
Expected: PASS. (No regressions.)

- [ ] **Step 10: Commit**

```bash
git add internal/domain/worker/worker.go internal/domain/worker/manager_test.go
git commit -m "feat(worker): allow updating WorkDir via UpdateWorkerParams"
```

---

## Task 2: Domain — lazy MkdirAll on execute

**Files:**
- Modify: `internal/domain/worker/execution.go:1-14,50-54`
- Test: `internal/domain/worker/manager_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `internal/domain/worker/manager_test.go`:

```go
func TestExecute_CreatesNewWorkDirIfMissing(t *testing.T) {
    m, cleanup := newTestManager(t)
    defer cleanup()

    w, err := m.CreateWorker(CreateWorkerParams{Name: "wd-exec-mkdir", Engine: "claude"})
    if err != nil {
        t.Fatalf("create worker: %v", err)
    }

    // Point WorkDir at a path that does NOT exist yet.
    missing := filepath.Join(t.TempDir(), "deep", "nested", "missing")
    if _, err := m.UpdateWorker(w.ID, UpdateWorkerParams{WorkDir: &missing}); err != nil {
        t.Fatalf("update worker: %v", err)
    }
    if _, err := os.Stat(missing); !os.IsNotExist(err) {
        t.Fatalf("precondition: missing dir should not exist, got err=%v", err)
    }

    // ExecuteWorker may fail later (no real engine in tests), but it MUST create the dir first.
    _, _ = m.ExecuteWorker(context.Background(), ExecuteRequest{
        WorkerID:     w.ID,
        SessionID:    "test-session",
        TriggerInput: "noop",
    })

    if _, err := os.Stat(missing); err != nil {
        t.Fatalf("expected execute to create WorkDir %q, got err=%v", missing, err)
    }
}
```

If `newTestManager` does not register a usable in-test engine adapter, ExecuteWorker may return an error before reaching the MkdirAll. In that case, adjust the test to call only the smallest reachable wrapper that runs MkdirAll, OR refactor execution.go to extract a tiny `ensureWorkDir(path)` helper and test that helper directly. Prefer the second option — it keeps the test deterministic.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/worker/ -run TestExecute_CreatesNewWorkDirIfMissing -v`
Expected: FAIL — the directory does not exist after ExecuteWorker returns, because no code path creates it.

- [ ] **Step 3: Implement MkdirAll before Prepare**

In `internal/domain/worker/execution.go`, ensure `os` is imported. Current imports (lines 3-14):

```go
import (
    "context"
    "fmt"
    "time"

    ai "github.com/theopenbee/openbee/internal/ai"
    "github.com/theopenbee/openbee/internal/infra/auth"
    "github.com/theopenbee/openbee/internal/infra/model"
    "github.com/theopenbee/openbee/internal/infra/store"
    "github.com/theopenbee/openbee/internal/infra/utils"
    "go.uber.org/zap"
)
```

Add `"os"` to the standard-library group:

```go
import (
    "context"
    "fmt"
    "os"
    "time"

    ai "github.com/theopenbee/openbee/internal/ai"
    "github.com/theopenbee/openbee/internal/infra/auth"
    "github.com/theopenbee/openbee/internal/infra/model"
    "github.com/theopenbee/openbee/internal/infra/store"
    "github.com/theopenbee/openbee/internal/infra/utils"
    "go.uber.org/zap"
)
```

Then locate lines 50-53:

```go
    if err := engine.Prepare(worker.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
        log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
    }
    timeout := m.workerTimeout
```

Replace with:

```go
    if err := os.MkdirAll(worker.WorkDir, 0755); err != nil {
        log.Error("ensure worker workdir", zap.String("op", "execute"), zap.String("work_dir", worker.WorkDir), zap.Error(err))
        m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
        m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
        return exec, fmt.Errorf("ensure worker workdir: %w", err)
    }
    if err := engine.Prepare(worker.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
        log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
    }
    timeout := m.workerTimeout
```

Rationale: MkdirAll failure is a real configuration error (bad path, permissions, disk full). Unlike `Prepare`, we fail fast — Prepare's logged-and-continued behavior is preserved as-is from the existing code.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/worker/ -run TestExecute_CreatesNewWorkDirIfMissing -v`
Expected: PASS — the directory exists after the call.

- [ ] **Step 5: Run full domain tests for regressions**

Run: `go test ./internal/domain/worker/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/worker/execution.go internal/domain/worker/manager_test.go
git commit -m "feat(worker): ensure WorkDir exists before each execution"
```

---

## Task 3: CLI — add --work-dir flag to `worker update`

**Files:**
- Modify: `cmd/openbee/internal/cli/ctlcmd/worker.go:134-174`

- [ ] **Step 1: Add the local variable and flag binding**

Locate `newWorkerUpdateCommand` (lines 134-174). The current vars block is:

```go
    var (
        name        string
        description string
        constraints string
        engine      string
        department  string
        scopes      string
        engineArgs  []string
    )
```

Replace with:

```go
    var (
        name        string
        description string
        constraints string
        workDir     string
        engine      string
        department  string
        scopes      string
        engineArgs  []string
    )
```

Inside the existing `RunE` closure, add a `setIfFlagChanged` line right after the `constraints` line (line 152):

```go
            setIfFlagChanged(c, a, "constraints", "constraints", constraints)
            setIfFlagChanged(c, a, "work-dir", "work_dir", workDir)
            setIfFlagChanged(c, a, "engine", "engine", engine)
```

Then register the flag (after the `--constraints` flag, around line 168):

```go
    cmd.Flags().StringVar(&constraints, "constraints", "", "New constraints content")
    cmd.Flags().StringVar(&workDir, "work-dir", "", "New working directory path")
    cmd.Flags().StringVar(&engine, "engine", "", "AI engine to use (e.g. claude, codex, pi); leave empty to keep unchanged")
```

- [ ] **Step 2: Build openbee to verify the CLI compiles**

Run: `go build ./...`
Expected: build succeeds, no errors.

- [ ] **Step 3: Smoke test — verify the flag is wired**

Run: `go run ./cmd/openbee ctl worker update --help`
Expected: output includes a line `--work-dir string   New working directory path`.

- [ ] **Step 4: Commit**

```bash
git add cmd/openbee/internal/cli/ctlcmd/worker.go
git commit -m "feat(cli): add --work-dir flag to openbee ctl worker update"
```

---

## Task 4: Skill docs — update embedded openbee-bee reference

**Files:**
- Modify: `internal/infra/skillinstall/skills/openbee-bee/references/cli-reference.md:12,~22-23`

- [ ] **Step 1: Update the `worker update` signature**

Locate line 12:

```
openbee ctl worker update <id> [--name <name>] [--description <description>] [--constraints <constraints>] [--engine <engine>] [--department <id|name>] [--scopes <scopes>] [--engine-args <engine=args>]
```

Replace with (insert `--work-dir`):

```
openbee ctl worker update <id> [--name <name>] [--description <description>] [--constraints <constraints>] [--work-dir <directory>] [--engine <engine>] [--department <id|name>] [--scopes <scopes>] [--engine-args <engine=args>]
```

- [ ] **Step 2: Add a usage example to the worker section**

In the worker subcommand section (immediately after the closing ``` of the existing "Clear all scopes" example block — at the spot just before `## department subcommand`), append a new fenced block:

```bash
# Update a worker's working directory
openbee ctl worker update <id> --work-dir /path/to/new/dir
```

This goes inline with the existing examples (no new heading) — read the file once before inserting to pick the natural location.

- [ ] **Step 3: Skim SKILL.md and entity-relationships.md for stale claims**

Run: `grep -n "work_dir\|WorkDir\|工作目录" internal/infra/skillinstall/skills/openbee-bee/SKILL.md internal/infra/skillinstall/skills/openbee-bee/references/entity-relationships.md`
Expected: zero lines OR a small number of mentions.

If you find a line that says something like "WorkDir is immutable" or "set at create time only", update it to reflect that `WorkDir` is now mutable via `openbee ctl worker update --work-dir`. If no such claims exist, no further changes needed.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/skillinstall/skills/openbee-bee/references/cli-reference.md
# also add SKILL.md or entity-relationships.md if you edited them
git commit -m "docs(skill): document --work-dir flag on worker update"
```

---

## Task 5: Web — add WorkDir field to EditWorkerInfoSheet

**Files:**
- Modify: `web/src/components/edit-worker-info-sheet.tsx`
- Modify: `web/src/locales/zh.json`
- Modify: `web/src/locales/en.json`

- [ ] **Step 1: Add i18n labels**

In both locale files, locate the `workers.form` block. Add two new keys (Chinese and English respectively). Pick the exact path used by sibling labels — e.g. `workers.form.engineHelper`.

In `web/src/locales/zh.json`, inside `workers.form`:

```json
"workDir": "工作目录",
"workDirPlaceholder": "/path/to/work/dir",
"workDirHelper": "下次执行任务时，若该目录不存在会自动创建。旧目录的文件不会被移动或删除。"
```

In `web/src/locales/en.json`, inside `workers.form`:

```json
"workDir": "Work directory",
"workDirPlaceholder": "/path/to/work/dir",
"workDirHelper": "Created on next execution if missing. Files in the old directory are left untouched."
```

Keep existing key ordering / trailing-comma rules consistent with the surrounding entries.

- [ ] **Step 2: Add `workDir` state to the sheet**

In `web/src/components/edit-worker-info-sheet.tsx`, add a state variable next to the others (after the `description` line, ~line 48):

```tsx
  const [workDir, setWorkDir] = useState("")
```

- [ ] **Step 3: Initialize `workDir` when the sheet opens**

In the `useEffect` block (lines 55-65), add an init line next to the others:

```tsx
  useEffect(() => {
    if (open) {
      setName(worker.name ?? "")
      setDescription(worker.description ?? "")
      setWorkDir(worker.work_dir ?? "")
      setEngine(pickDefaultEngine(worker.engine, enabledEngines))
      setSelectedDeptIds(new Set(worker.departments?.map((d) => d.id) ?? []))
      setEngineArgs(worker.engine_args ?? {})
      setDeptSearch("")
      setSubmitError("")
    }
  }, [open, worker, enabledEngines])
```

- [ ] **Step 4: Wire workDir into change detection and submit payload**

In `handleSubmit` (lines 67-99), update the change detection and payload:

```tsx
  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setSubmitError("")
    try {
      const originalDeptIds = worker.departments?.map((d) => d.id).sort().join(",") ?? ""
      const newDeptIds = [...selectedDeptIds].sort().join(",")
      const engineArgsChanged = !engineArgsEqual(
        stripEmptyEngineArgs(engineArgs),
        worker.engine_args ?? {},
      )
      const nameChanged = name !== worker.name
      const trimmedWorkDir = workDir.trim()
      const workDirChanged = trimmedWorkDir !== (worker.work_dir ?? "")
      const workerChanged =
        nameChanged ||
        description !== (worker.description ?? "") ||
        workDirChanged ||
        engine !== pickDefaultEngine(worker.engine, enabledEngines) ||
        engineArgsChanged
      const deptsChanged = newDeptIds !== originalDeptIds

      const ops: Promise<unknown>[] = []
      if (workerChanged) {
        const data: Record<string, unknown> = { description, engine, engine_args: engineArgs }
        if (nameChanged) data.name = name
        if (workDirChanged) data.work_dir = trimmedWorkDir
        ops.push(updateWorker.mutateAsync({ id: worker.id, data }))
      }
      if (deptsChanged) {
        ops.push(setWorkerDepts.mutateAsync({ workerId: worker.id, departmentIds: [...selectedDeptIds] }))
      }
      await Promise.all(ops)
      onOpenChange(false)
    } catch (err) {
      setSubmitError(getErrorMessage(err))
    }
  }
```

- [ ] **Step 5: Render the WorkDir input field**

Inside the form body, place the field between the description section and the engine section. The existing description block ends with `</div>` around line 148 and the engine block starts with `<div className="space-y-1.5">` for `ewis-engine`. Insert this block in between:

```tsx
            <div className="space-y-1.5">
              <Label htmlFor="ewis-workdir">{t("workers.form.workDir")}</Label>
              <Input
                id="ewis-workdir"
                value={workDir}
                onChange={(e) => setWorkDir(e.target.value)}
                placeholder={t("workers.form.workDirPlaceholder")}
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.workDirHelper")}</p>
            </div>
```

- [ ] **Step 6: Block submit when WorkDir is empty after trim**

In the Save button disabled prop (line 235):

```tsx
            disabled={isPending || !name.trim() || !workDir.trim()}
```

Rationale: a worker's `work_dir` is required by the backend (validation rejects empty after trim) — this prevents a confusing roundtrip error.

- [ ] **Step 7: Run web build / type-check**

Run: `cd web && npm run build` (or `pnpm build` / `yarn build` — match what `web/package.json` declares)
Expected: build succeeds.

If a separate type-check script exists (e.g. `npm run typecheck`), run it as well and expect PASS.

- [ ] **Step 8: Manual smoke (developer's machine — no automation expected)**

Optional but recommended:

1. Start backend: `go run ./cmd/openbee server` (or whatever the project's run command is)
2. Start web: `cd web && npm run dev`
3. Create a worker; open detail page → "Edit"; change Work Directory; Save
4. Reload the detail page; verify the new path is shown
5. Trigger a task on the worker; verify the new directory gets created on disk

- [ ] **Step 9: Commit**

```bash
git add web/src/components/edit-worker-info-sheet.tsx web/src/locales/zh.json web/src/locales/en.json
git commit -m "feat(web): edit worker WorkDir in EditWorkerInfoSheet"
```

---

## Task 6: Integration verification

**Files:** (no changes)

- [ ] **Step 1: Full Go test suite**

Run: `go test ./...`
Expected: PASS (no regressions in any package).

- [ ] **Step 2: End-to-end CLI smoke**

Build the binary, then run the sequence below. Replace `<bin>` with the local build (e.g. `./openbee` after `go build -o openbee ./cmd/openbee`).

```bash
<bin> ctl worker create -n wd-smoketest --work-dir /tmp/openbee-wd-smoke-a
# capture the returned id, then:
<bin> ctl worker update <id> --work-dir /tmp/openbee-wd-smoke-b
<bin> ctl worker get <id>
```

Expected: the `get` output shows `work_dir: /tmp/openbee-wd-smoke-b`. The original `/tmp/openbee-wd-smoke-a` directory still exists on disk.

- [ ] **Step 3: Confirm skill doc renders correctly post-embed**

The CLI reference is embedded into the binary via `internal/infra/skillinstall/skills/`. Rebuild:

```bash
go build -o openbee ./cmd/openbee
```

If the project ships a way to print the embedded skill content (check `internal/infra/skillinstall/` for a debug command), use it to confirm the new `--work-dir` line is in the embedded copy. If no such command exists, just verify a clean `go build` succeeds — `go:embed` will fail at build time if the file is missing.

- [ ] **Step 4: Cleanup smoketest workers**

```bash
<bin> ctl worker delete <id> --delete-work-dir
# also remove the leftover old dir if it wasn't tracked:
rm -rf /tmp/openbee-wd-smoke-a
```

- [ ] **Step 5: Final commit (only if anything changed during smoke)**

If steps 1-4 surfaced an issue that required a fix, commit it now with a descriptive message. Otherwise no commit is needed — the implementation is done.

---

## Done

All approved spec items are implemented:

- ✅ Domain accepts `WorkDir` in `UpdateWorkerParams`, validates non-empty, trims, writes through.
- ✅ Execution path auto-creates the directory before each run.
- ✅ CLI exposes `--work-dir` on `worker update`.
- ✅ Skill embedded docs reflect the new flag.
- ✅ Web edit sheet lets the user change the path.
- ✅ Old directory is left untouched; new directory is created lazily.
- ✅ Running executions are not blocked by an in-flight update.
