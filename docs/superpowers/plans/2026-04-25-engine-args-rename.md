# engine_extra_args → engine_args Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the `engine_extra_args` identifier to `engine_args` across all codebase layers — database, Go backend, API, CLI, frontend, i18n, and docs — with no behavior change.

**Architecture:** Pure mechanical rename across 24 files. Dependency order: AI types first (others import them), then infra model, then infra store + migration, then domain, then API, then CLI, then frontend, then i18n, then docs.

**Tech Stack:** Go (backend), TypeScript/React (frontend), SQLite (database via migration), Cobra (CLI), i18next (i18n)

---

## File Map

| File | Change |
|------|--------|
| `internal/ai/extra_args.go` → `internal/ai/engine_args.go` | Rename file + type + functions |
| `internal/ai/extra_args_test.go` → `internal/ai/engine_args_test.go` | Rename file + test functions |
| `internal/infra/model/system_config.go` | Rename 2 constants |
| `internal/infra/model/worker.go` | Rename struct field + JSON/DB tags |
| `internal/infra/store/db.go` | Edit migration #44 |
| `internal/infra/store/worker_store.go` | Rename SQL column references + const strings |
| `internal/domain/worker/worker.go` | Rename field, JSON tag, error string, local var |
| `internal/domain/worker/manager.go` | Rename 4 methods + type refs + error strings |
| `internal/domain/worker/manager_test.go` | Rename test functions |
| `internal/domain/bee/bee_process.go` | Rename 2 methods + type refs + const refs |
| `internal/api/worker_handler.go` | Rename struct fields, JSON tags, helper func, error string |
| `internal/api/worker_handler_test.go` | Rename field refs |
| `internal/api/system_config_handler.go` | Rename constant refs |
| `internal/api/system_config_handler_test.go` | Rename constant refs |
| `cmd/openbee/ctl_worker.go` | Rename flag const, vars, func, payload key |
| `web/src/lib/types.ts` | Rename interface field |
| `web/src/lib/api.ts` | Rename field in create type |
| `web/src/hooks/use-workers.ts` | Rename field in mutation type |
| `web/src/components/engine-extra-args-section.tsx` → `engine-args-section.tsx` | Rename file + component + props |
| `web/src/components/edit-worker-info-sheet.tsx` | Update import path + payload field |
| `web/src/locales/en.json` | Rename 3 translation keys + update 2 display strings |
| `web/src/locales/zh.json` | Rename 3 translation keys + update 2 display strings |
| `internal/infra/skillinstall/skills/openbee-bee/references/cli-reference.md` | Update flag name in 3 places |

---

## Task 1: Rename AI package file and symbols

**Files:**
- Delete: `internal/ai/extra_args.go`
- Create: `internal/ai/engine_args.go`
- Delete: `internal/ai/extra_args_test.go`
- Create: `internal/ai/engine_args_test.go`

- [ ] **Step 1: Create `internal/ai/engine_args.go` with renamed type and functions**

```go
package ai

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

type EngineArgsMap map[string][]string

// ParseEngineArgs tokenizes raw CLI strings per engine while preserving
// order, duplicates, and quoted values.
func ParseEngineArgs(raw map[string]string) (EngineArgsMap, error) {
	result := make(EngineArgsMap, len(raw))
	for engine, s := range raw {
		args, err := splitCLIArgs(s)
		if err != nil {
			return nil, fmt.Errorf("engine %q: %w", engine, err)
		}
		result[engine] = args
	}
	return result, nil
}

func splitCLIArgs(s string) ([]string, error) {
	var (
		args      []string
		buf       strings.Builder
		inSingle  bool
		inDouble  bool
		escaped   bool
		tokenOpen bool
	)

	flush := func() {
		if !tokenOpen {
			return
		}
		args = append(args, buf.String())
		buf.Reset()
		tokenOpen = false
	}

	for _, r := range s {
		switch {
		case escaped:
			buf.WriteRune(r)
			escaped = false
			tokenOpen = true

		case inSingle:
			if r == '\'' {
				inSingle = false
			} else {
				buf.WriteRune(r)
			}
			tokenOpen = true

		case inDouble:
			switch r {
			case '"':
				inDouble = false
			case '\\':
				escaped = true
				tokenOpen = true
			default:
				buf.WriteRune(r)
				tokenOpen = true
			}

		default:
			switch {
			case unicode.IsSpace(r):
				flush()
			case r == '\'':
				inSingle = true
				tokenOpen = true
			case r == '"':
				inDouble = true
				tokenOpen = true
			case r == '\\':
				escaped = true
				tokenOpen = true
			default:
				buf.WriteRune(r)
				tokenOpen = true
			}
		}
	}

	if escaped {
		return nil, fmt.Errorf("unterminated escape sequence")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quoted string")
	}
	flush()
	return args, nil
}

// MergeEngineArgs merges base and override by appending override args
// after base args, so later flags can override earlier ones while preserving
// the original CLI ordering.
func MergeEngineArgs(base, override EngineArgsMap) EngineArgsMap {
	result := make(EngineArgsMap, len(base)+len(override))
	for engine, args := range base {
		result[engine] = slices.Clone(args)
	}
	for engine, overrideArgs := range override {
		result[engine] = append(result[engine], overrideArgs...)
	}
	return result
}

// ParseEngineArgsJSON returns nil for empty/unset values.
func ParseEngineArgsJSON(value string) EngineArgsMap {
	if value == "" || value == "{}" {
		return nil
	}
	var raw map[string]string
	if json.Unmarshal([]byte(value), &raw) != nil {
		return nil
	}
	parsed, _ := ParseEngineArgs(raw)
	return parsed
}
```

- [ ] **Step 2: Delete `internal/ai/extra_args.go`**

```bash
git rm internal/ai/extra_args.go
```

- [ ] **Step 3: Create `internal/ai/engine_args_test.go` (rename test file)**

Read the existing `extra_args_test.go`, then create `engine_args_test.go` with all `EngineExtraArgs` → `EngineArgs`, `ParseEngineExtraArgs` → `ParseEngineArgs`, `MergeEngineExtraArgs` → `MergeEngineArgs` substituted throughout. Keep all test cases identical — only names change.

```bash
# Read the old test file, create the new one with renames applied:
sed 's/EngineExtraArgsMap/EngineArgsMap/g; s/ParseEngineExtraArgs/ParseEngineArgs/g; s/MergeEngineExtraArgs/MergeEngineArgs/g; s/ParseEngineExtraArgsJSON/ParseEngineArgsJSON/g; s/TestParseEngineExtraArgs/TestParseEngineArgs/g; s/TestMergeEngineExtraArgs/TestMergeEngineArgs/g' \
  internal/ai/extra_args_test.go > internal/ai/engine_args_test.go
git rm internal/ai/extra_args_test.go
```

- [ ] **Step 4: Verify the AI package compiles**

```bash
go build ./internal/ai/...
```

Expected: no output (clean build)

- [ ] **Step 5: Run AI package tests**

```bash
go test ./internal/ai/... -v
```

Expected: all tests PASS (same tests as before, just renamed)

- [ ] **Step 6: Commit**

```bash
git add internal/ai/engine_args.go internal/ai/engine_args_test.go
git commit -m "refactor: rename EngineExtraArgsMap/ParseEngineExtraArgs → EngineArgsMap/ParseEngineArgs in ai package"
```

---

## Task 2: Rename infra model constants and struct field

**Files:**
- Modify: `internal/infra/model/system_config.go`
- Modify: `internal/infra/model/worker.go`

- [ ] **Step 1: Update `internal/infra/model/system_config.go`**

Replace the two constant definitions:

```go
// SystemConfigKeyEngineArgsGlobal is the key for global engine args (applied to all workers).
const SystemConfigKeyEngineArgsGlobal = "engine_args_global"

// SystemConfigKeyEngineArgsBee is the key for bee-level engine args.
const SystemConfigKeyEngineArgsBee = "engine_args_bee"
```

(Remove `SystemConfigKeyEngineExtraArgsGlobal` and `SystemConfigKeyEngineExtraArgsBee` entirely.)

- [ ] **Step 2: Update `internal/infra/model/worker.go` struct field**

In the `Worker` struct, change:
```go
EngineExtraArgs     string       `json:"engine_extra_args" db:"engine_extra_args"`
```
to:
```go
EngineArgs          string       `json:"engine_args" db:"engine_args"`
```

- [ ] **Step 3: Verify infra model compiles (will fail until store is fixed — skip if so, continue)**

```bash
go build ./internal/infra/model/...
```

Expected: clean (model package has no imports of itself that use the old names)

- [ ] **Step 4: Commit**

```bash
git add internal/infra/model/system_config.go internal/infra/model/worker.go
git commit -m "refactor: rename engine_extra_args field and constants in infra model"
```

---

## Task 3: Update DB migration and store SQL

**Files:**
- Modify: `internal/infra/store/db.go`
- Modify: `internal/infra/store/worker_store.go`

- [ ] **Step 1: Edit migration #44 in `internal/infra/store/db.go`**

Find (around line 404–408):
```go
{
    version: 44,
    name:    "add_engine_extra_args_to_workers",
    sql:     `ALTER TABLE bee_workers ADD COLUMN engine_extra_args TEXT NOT NULL DEFAULT '{}'`,
},
```

Replace with:
```go
{
    version: 44,
    name:    "add_engine_args_to_workers",
    sql:     `ALTER TABLE bee_workers ADD COLUMN engine_args TEXT NOT NULL DEFAULT '{}'`,
},
```

- [ ] **Step 2: Update column references in `internal/infra/store/worker_store.go`**

Change the two `const` strings (lines 44–46):
```go
const (
    workerColumns        = `id, name, description, constraints, work_dir, engine, engine_args, status, permission_scopes, created_at, updated_at`
    workerColumnsAliased = `w.id, w.name, w.description, w.constraints, w.work_dir, w.engine, w.engine_args, w.status, w.permission_scopes, w.created_at, w.updated_at`
)
```

Change the `Create` INSERT SQL (line 33):
```go
`INSERT INTO bee_workers (id, name, description, constraints, work_dir, engine, engine_args, status, permission_scopes, created_at, updated_at)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
```

Change the `Create` guard (line 29):
```go
if w.EngineArgs == "" {
    w.EngineArgs = "{}"
}
```

Change the `Update` SQL (line 223):
```go
`UPDATE bee_workers SET name=?, description=?, constraints=?, work_dir=?, engine=?, engine_args=?, status=?, permission_scopes=?, updated_at=?
 WHERE id=?`,
w.Name, w.Description, w.Constraints, w.WorkDir, w.Engine,
w.EngineArgs, w.Status, w.PermissionScopes, w.UpdatedAt, w.ID,
```

Change the `scanWorker` scan (line 52):
```go
err := scanner.Scan(
    &w.ID, &w.Name, &w.Description, &w.Constraints,
    &w.WorkDir, &w.Engine, &w.EngineArgs, &w.Status, &w.PermissionScopes, &w.CreatedAt, &w.UpdatedAt,
)
```

- [ ] **Step 3: Verify infra store compiles**

```bash
go build ./internal/infra/...
```

Expected: clean build

- [ ] **Step 4: Commit**

```bash
git add internal/infra/store/db.go internal/infra/store/worker_store.go
git commit -m "refactor: rename engine_extra_args column to engine_args in migration and store"
```

---

## Task 4: Update domain worker package

**Files:**
- Modify: `internal/domain/worker/worker.go`
- Modify: `internal/domain/worker/manager.go`
- Modify: `internal/domain/worker/manager_test.go`

- [ ] **Step 1: Update `internal/domain/worker/worker.go`**

In `CreateWorkerParams` struct, rename field and comment:
```go
EngineArgs  string // JSON: map[engine]rawCLIString
```

In `UpdateWorkerParams` struct, rename field and JSON tag:
```go
EngineArgs  map[string]string `json:"engine_args"` // engine -> raw CLI string; nil = no change; empty map clears all
```

In `HasChanges()`:
```go
return p.Name != nil || p.Description != nil || p.Constraints != nil ||
    p.PermissionScopes != nil || p.Engine != nil || p.EngineArgs != nil
```

In `Validate()`:
```go
if p.EngineArgs != nil {
    if err := m.ValidateEngineArgs(p.EngineArgs); err != nil {
        return err
    }
}
```

In `ApplyTo()`, rename all `p.EngineExtraArgs` → `p.EngineArgs` and `w.EngineExtraArgs` → `w.EngineArgs`, and fix the error string:
```go
if p.EngineArgs != nil {
    if len(p.EngineArgs) == 0 {
        w.EngineArgs = "{}"
    } else {
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
    }
}
```

In `CreateWorker()`, rename local variable and struct field:
```go
engineArgs := p.EngineArgs
if engineArgs == "" {
    engineArgs = "{}"
}
workerModel := model.Worker{
    // ...
    EngineArgs:       engineArgs,
    // ...
}
```

- [ ] **Step 2: Update `internal/domain/worker/manager.go`**

Rename `loadExtraArgs` → `loadEngineArgs`:
```go
func (m *Manager) loadEngineArgs(ctx context.Context, key string) ai.EngineArgsMap {
    if m.sysConfigStore == nil {
        return nil
    }
    cfg, found, err := m.sysConfigStore.Get(ctx, key)
    if err != nil || !found {
        return nil
    }
    return ai.ParseEngineArgsJSON(cfg.Value)
}
```

Rename `resolveExtraArgs` → `resolveEngineArgs`, update const refs and type refs:
```go
func (m *Manager) resolveEngineArgs(ctx context.Context, worker model.Worker, engineName string) []string {
    globalMap := m.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsGlobal)
    workerMap := ai.ParseEngineArgsJSON(worker.EngineArgs)
    merged := ai.MergeEngineArgs(globalMap, workerMap)
    return merged[engineName]
}
```

Rename `ValidateEngineExtraArgs` → `ValidateEngineArgs`, update all internal refs:
```go
func (m *Manager) ValidateEngineArgs(raw map[string]string) error {
    if len(raw) == 0 {
        return nil
    }
    for engine := range raw {
        if engine == "" {
            return fmt.Errorf("engine_args contains an empty engine name: %w", ErrValidation)
        }
        if err := m.ValidateEngine(engine); err != nil {
            return fmt.Errorf("engine_args[%q]: %w", engine, err)
        }
    }
    if _, err := ai.ParseEngineArgs(raw); err != nil {
        return fmt.Errorf("invalid engine_args: %w", err)
    }
    return nil
}
```

- [ ] **Step 3: Update `internal/domain/worker/manager_test.go`**

Rename test functions:
- `TestManager_ValidateEngineExtraArgs_RejectsUnknownEngine` → `TestManager_ValidateEngineArgs_RejectsUnknownEngine`
- `TestManager_ValidateEngineExtraArgs_RejectsInvalidArgs` → `TestManager_ValidateEngineArgs_RejectsInvalidArgs`

Update any call sites: `m.ValidateEngineExtraArgs(...)` → `m.ValidateEngineArgs(...)`

- [ ] **Step 4: Verify domain worker package compiles**

```bash
go build ./internal/domain/worker/...
```

Expected: clean build

- [ ] **Step 5: Run domain worker tests**

```bash
go test ./internal/domain/worker/... -v
```

Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/worker/worker.go internal/domain/worker/manager.go internal/domain/worker/manager_test.go
git commit -m "refactor: rename engine_extra_args → engine_args in domain/worker package"
```

---

## Task 5: Update domain bee package

**Files:**
- Modify: `internal/domain/bee/bee_process.go`

- [ ] **Step 1: Update `internal/domain/bee/bee_process.go`**

Rename `resolveExtraArgs` → `resolveEngineArgs`, update const refs and type refs:
```go
func (p *BeeProcess) resolveEngineArgs(ctx context.Context) []string {
    engineName := p.engineCfg.Get()
    globalMap := p.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsGlobal)
    beeMap := p.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsBee)
    merged := ai.MergeEngineArgs(globalMap, beeMap)
    return merged[engineName]
}
```

Rename `loadExtraArgs` → `loadEngineArgs`, update type ref:
```go
func (p *BeeProcess) loadEngineArgs(ctx context.Context, key string) ai.EngineArgsMap {
    if p.sysConfigStore == nil {
        return nil
    }
    cfg, found, err := p.sysConfigStore.Get(ctx, key)
    if err != nil || !found {
        return nil
    }
    return ai.ParseEngineArgsJSON(cfg.Value)
}
```

Update the call site in `Run()`:
```go
opts.ExtraArgs = p.resolveEngineArgs(ctx)
```

- [ ] **Step 2: Verify bee package compiles**

```bash
go build ./internal/domain/bee/...
```

Expected: clean build

- [ ] **Step 3: Run all Go tests so far**

```bash
go test ./internal/...
```

Expected: all tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/domain/bee/bee_process.go
git commit -m "refactor: rename engine_extra_args → engine_args in domain/bee package"
```

---

## Task 6: Update API layer

**Files:**
- Modify: `internal/api/worker_handler.go`
- Modify: `internal/api/worker_handler_test.go`
- Modify: `internal/api/system_config_handler.go`
- Modify: `internal/api/system_config_handler_test.go`

- [ ] **Step 1: Update `internal/api/worker_handler.go`**

In `createWorkerRequest` struct:
```go
EngineArgs  map[string]string `json:"engine_args"` // engine -> raw CLI string
```

In `workerResponse` struct:
```go
EngineArgs  map[string]string  `json:"engine_args"`
```

Rename function `parseWorkerEngineExtraArgs` → `parseWorkerEngineArgs` (keep body identical):
```go
func parseWorkerEngineArgs(raw string) (map[string]string, error) {
    if raw == "" || raw == "{}" {
        return map[string]string{}, nil
    }
    var parsed map[string]string
    if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
        return nil, err
    }
    return parsed, nil
}
```

In `toWorkerResponse()`, update call site and field:
```go
func toWorkerResponse(w model.Worker) (workerResponse, error) {
    engineArgs, err := parseWorkerEngineArgs(w.EngineArgs)
    if err != nil {
        return workerResponse{}, fmt.Errorf("parse worker %s engine_args: %w", w.ID, err)
    }
    return workerResponse{
        // ...
        EngineArgs:       engineArgs,
        // ...
    }, nil
}
```

In `Create()` handler, rename local var and struct field:
```go
if err := h.manager.ValidateEngineArgs(req.EngineArgs); err != nil { ... }

var engineArgsJSON string
if len(req.EngineArgs) > 0 {
    b, _ := json.Marshal(req.EngineArgs)
    engineArgsJSON = string(b)
} else {
    engineArgsJSON = "{}"
}

w, err := h.manager.CreateWorker(worker.CreateWorkerParams{
    // ...
    EngineArgs:  engineArgsJSON,
})
```

- [ ] **Step 2: Update `internal/api/system_config_handler.go`**

Find all references to `model.SystemConfigKeyEngineExtraArgsGlobal` and `model.SystemConfigKeyEngineExtraArgsBee`, replace with `model.SystemConfigKeyEngineArgsGlobal` and `model.SystemConfigKeyEngineArgsBee` respectively.

- [ ] **Step 3: Update `internal/api/worker_handler_test.go` and `system_config_handler_test.go`**

In test files, update any JSON field references from `"engine_extra_args"` to `"engine_args"` and any struct field references from `EngineExtraArgs` to `EngineArgs`.

- [ ] **Step 4: Verify API package compiles**

```bash
go build ./internal/api/...
```

Expected: clean build

- [ ] **Step 5: Run API tests**

```bash
go test ./internal/api/... -v
```

Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api/worker_handler.go internal/api/worker_handler_test.go \
        internal/api/system_config_handler.go internal/api/system_config_handler_test.go
git commit -m "refactor: rename engine_extra_args → engine_args in API layer"
```

---

## Task 7: Update CLI

**Files:**
- Modify: `cmd/openbee/ctl_worker.go`

- [ ] **Step 1: Update `cmd/openbee/ctl_worker.go`**

Rename constant:
```go
const engineArgsFlagName = "engine-args"
```

Rename variables:
```go
var (
    // in create vars block:
    workerCreateEngineArgs []string
)
var (
    // in update vars block:
    workerUpdateEngineArgs []string
)
```

Rename function `parseEngineExtraArgsFlag` → `parseEngineArgsFlag` (body unchanged):
```go
func parseEngineArgsFlag(entries []string) map[string]string {
    result := make(map[string]string, len(entries))
    for _, entry := range entries {
        engine, args, ok := strings.Cut(entry, "=")
        if !ok {
            continue
        }
        result[engine] = args
    }
    return result
}
```

In the create command `RunE`, update variable names and API payload key:
```go
if len(workerCreateEngineArgs) > 0 {
    parsed := parseEngineArgsFlag(workerCreateEngineArgs)
    a["engine_args"] = parsed
}
```

In the update command `RunE`, update variable names, flag check, and API payload key:
```go
if cmd.Flags().Changed(engineArgsFlagName) {
    a["engine_args"] = parseEngineArgsFlag(workerUpdateEngineArgs)
}
```

In `init()`, update `StringArrayVar` registrations:
```go
ctlWorkerCreateCmd.Flags().StringArrayVar(&workerCreateEngineArgs, engineArgsFlagName, nil, "Extra CLI args per engine, e.g. \"claude=--model claude-sonnet-4-5 --effort high\" (repeatable)")
ctlWorkerUpdateCmd.Flags().StringArrayVar(&workerUpdateEngineArgs, engineArgsFlagName, nil, "Extra CLI args per engine, e.g. \"claude=--model claude-opus-4-7\" (repeatable); pass \"claude=\" to clear")
```

- [ ] **Step 2: Verify CLI compiles**

```bash
go build ./cmd/openbee/...
```

Expected: clean build

- [ ] **Step 3: Run full Go test suite**

```bash
go test ./...
```

Expected: all tests PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/openbee/ctl_worker.go
git commit -m "refactor: rename --engine-extra-args to --engine-args in CLI"
```

---

## Task 8: Update frontend TypeScript types

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/hooks/use-workers.ts`

- [ ] **Step 1: Update `web/src/lib/types.ts`**

In the `Worker` interface, rename field:
```typescript
engine_args?: Record<string, string>
```

- [ ] **Step 2: Update `web/src/lib/api.ts`**

Find the create worker function type parameter (around line 70–73), rename field:
```typescript
engine_args?: Record<string, string>
```

- [ ] **Step 3: Update `web/src/hooks/use-workers.ts`**

In the `useCreateWorker` mutation type, rename field:
```typescript
engine_args?: Record<string, string>
```

- [ ] **Step 4: Verify TypeScript types (quick check via tsc)**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

Expected: no errors (or only pre-existing unrelated errors)

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts web/src/hooks/use-workers.ts
git commit -m "refactor: rename engine_extra_args → engine_args in frontend TypeScript types"
```

---

## Task 9: Rename frontend React component

**Files:**
- Delete: `web/src/components/engine-extra-args-section.tsx`
- Create: `web/src/components/engine-args-section.tsx`
- Modify: `web/src/components/edit-worker-info-sheet.tsx`

- [ ] **Step 1: Create `web/src/components/engine-args-section.tsx`**

```tsx
import { useTranslation } from "react-i18next"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import type { Engine } from "@/lib/types"

interface EngineArgsSectionProps {
  engines: Engine[]
  value: Record<string, string>
  onChange: (args: Record<string, string>) => void
}

export function EngineArgsSection({ engines, value, onChange }: EngineArgsSectionProps) {
  const { t } = useTranslation()

  if (engines.length === 0) return null

  return (
    <div className="space-y-2">
      <Label>{t("workers.form.engineArgs")}</Label>
      <div className="space-y-2">
        {engines.map((eng) => (
          <div key={eng} className="space-y-1">
            <span className="text-xs font-medium text-muted-foreground capitalize">{eng}</span>
            <Input
              value={value[eng] ?? ""}
              onChange={(e) =>
                onChange({ ...value, [eng]: e.target.value })
              }
              placeholder={t("workers.form.engineArgsPlaceholder")}
              className="font-mono text-xs"
            />
          </div>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">{t("workers.form.engineArgsHelper")}</p>
    </div>
  )
}
```

- [ ] **Step 2: Delete old component file**

```bash
git rm web/src/components/engine-extra-args-section.tsx
```

- [ ] **Step 3: Update `web/src/components/edit-worker-info-sheet.tsx`**

Change import (line 26):
```typescript
import { EngineArgsSection } from "@/components/engine-args-section"
```

Change state variable name:
```typescript
const [engineArgs, setEngineArgs] = useState<Record<string, string>>({})
```

In `useEffect`, update both references:
```typescript
setEngineArgs(worker.engine_args ?? {})
```

In `handleSubmit`, update the comparison variable and payload:
```typescript
const originalEngineArgs = worker.engine_args ?? {}
const engineArgsChanged =
    Object.keys(engineArgs).length !== Object.keys(originalEngineArgs).length ||
    Object.entries(engineArgs).some(([k, v]) => originalEngineArgs[k] !== v)
const workerChanged =
    description !== (worker.description ?? "") ||
    engine !== pickDefaultEngine(worker.engine, enabledEngines) ||
    engineArgsChanged

// ...
ops.push(updateWorker.mutateAsync({ id: worker.id, data: { description, engine, engine_args: engineArgs } }))
```

Replace `<EngineExtraArgsSection>` usage with `<EngineArgsSection>`:
```tsx
<EngineArgsSection
  engines={enabledEngines}
  value={engineArgs}
  onChange={setEngineArgs}
/>
```

- [ ] **Step 4: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add web/src/components/engine-args-section.tsx web/src/components/edit-worker-info-sheet.tsx
git commit -m "refactor: rename EngineExtraArgsSection → EngineArgsSection component"
```

---

## Task 10: Update i18n translation keys

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Update `web/src/locales/en.json`**

In `workers.form`, replace:
```json
"engineArgs": "Engine Args",
"engineArgsPlaceholder": "--model claude-sonnet-4-5 --effort high",
"engineArgsHelper": "CLI flags passed to the engine for this worker.",
```

(Remove `engineExtraArgs`, `engineExtraArgsPlaceholder`, `engineExtraArgsHelper` keys.)

- [ ] **Step 2: Update `web/src/locales/zh.json`**

In `workers.form`, replace:
```json
"engineArgs": "引擎参数",
"engineArgsPlaceholder": "--model claude-sonnet-4-5 --effort high",
"engineArgsHelper": "传递给该 Worker 引擎的 CLI 参数。",
```

(Remove `engineExtraArgs`, `engineExtraArgsPlaceholder`, `engineExtraArgsHelper` keys.)

- [ ] **Step 3: Run frontend build to verify no broken i18n references**

```bash
cd web && npm run build 2>&1 | tail -20
```

Expected: build succeeds with no errors

- [ ] **Step 4: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "refactor: rename engineExtraArgs i18n keys to engineArgs"
```

---

## Task 11: Update docs

**Files:**
- Modify: `internal/infra/skillinstall/skills/openbee-bee/references/cli-reference.md`

- [ ] **Step 1: Update CLI reference documentation**

Find all 3 occurrences of `--engine-extra-args` and replace with `--engine-args`.

Line 11 (create command):
```
openbee ctl worker create --name <name> [...] [--engine-args <engine=args>]
```

Line 12 (update command):
```
openbee ctl worker update <id> [...] [--engine-args <engine=args>]
```

Line 22 (flag description):
```
- `--engine-args` (create/update): extra CLI flags for a specific engine, in `engine=<flags>` format (repeatable); e.g. `--engine-args "claude=--model claude-sonnet-4-6 --effort high"`; for update, pass `engine=` (empty value) to clear args for that engine
```

- [ ] **Step 2: Commit**

```bash
git add internal/infra/skillinstall/skills/openbee-bee/references/cli-reference.md
git commit -m "docs: rename --engine-extra-args to --engine-args in CLI reference"
```

---

## Task 12: Final verification

- [ ] **Step 1: Grep for any remaining `engine_extra_args` occurrences**

```bash
grep -r "engine_extra_args\|engineExtraArgs\|engine-extra-args\|EngineExtraArgs" \
  --include="*.go" --include="*.ts" --include="*.tsx" --include="*.json" --include="*.md" \
  . 2>/dev/null | grep -v ".git/"
```

Expected: no output (zero remaining occurrences)

- [ ] **Step 2: Run full Go test suite**

```bash
go test ./... -count=1
```

Expected: all tests PASS

- [ ] **Step 3: Run frontend production build**

```bash
cd web && npm run build
```

Expected: build succeeds, no TypeScript errors

- [ ] **Step 4: Verify CLI binary compiles and flag is registered**

```bash
go run ./cmd/openbee ctl worker create --help | grep engine-args
```

Expected: `--engine-args` appears in the help output

- [ ] **Step 5: Final commit if any stray files were missed**

```bash
git status
```

If clean: done. If any files were missed, make the changes and commit.
