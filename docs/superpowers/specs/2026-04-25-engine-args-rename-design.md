# Rename `engine_extra_args` → `engine_args`

**Date:** 2026-04-25  
**Branch:** feat/engine-extra-args  
**Status:** Approved

## Overview

Rename the `engine_extra_args` identifier to `engine_args` across all layers of the codebase. The feature has not been released, so no backward-compatibility shims are needed.

## Scope

~25 files, ~159 occurrences. Pure rename — no behavior changes, no new files, no new interfaces.

## Database

Edit migration #44 directly (no new migration needed):

- Column rename: `engine_extra_args` → `engine_args` in `bee_workers` table

## System Config Keys

| Old Key | New Key |
|---------|---------|
| `engine_extra_args_global` | `engine_args_global` |
| `engine_extra_args_bee` | `engine_args_bee` |

Go constants:

| Old | New |
|-----|-----|
| `SystemConfigKeyEngineExtraArgsGlobal` | `SystemConfigKeyEngineArgsGlobal` |
| `SystemConfigKeyEngineExtraArgsBee` | `SystemConfigKeyEngineArgsBee` |

## Go Backend

### Struct Fields & JSON/DB Tags

| Old | New |
|-----|-----|
| `EngineExtraArgs` (field) | `EngineArgs` |
| `json:"engine_extra_args"` | `json:"engine_args"` |
| `db:"engine_extra_args"` | `db:"engine_args"` |

Affected structs: `Worker` (infra model), `UpdateWorkerParams` (domain), `createWorkerRequest` (API), `workerResponse` (API).

### Types

| Old | New |
|-----|-----|
| `EngineExtraArgsMap` | `EngineArgsMap` |

### Functions

| Old | New |
|-----|-----|
| `ParseEngineExtraArgs()` | `ParseEngineArgs()` |
| `MergeEngineExtraArgs()` | `MergeEngineArgs()` |
| `ParseEngineExtraArgsJSON()` | `ParseEngineArgsJSON()` |
| `ValidateEngineExtraArgs()` | `ValidateEngineArgs()` |
| `parseWorkerEngineExtraArgs()` | `parseWorkerEngineArgs()` |
| `loadExtraArgs()` (manager.go, bee_process.go) | `loadEngineArgs()` |
| `resolveExtraArgs()` (manager.go, bee_process.go) | `resolveEngineArgs()` |

### Error Messages & String Literals

All occurrences of `"engine_extra_args"` in error messages and log strings renamed to `"engine_args"`.

### Test Functions

| Old | New |
|-----|-----|
| `TestManager_ValidateEngineExtraArgs_*` | `TestManager_ValidateEngineArgs_*` |
| `TestParseEngineExtraArgs_*` | `TestParseEngineArgs_*` |
| `TestMergeEngineExtraArgs_*` | `TestMergeEngineArgs_*` |

## CLI

| Old | New |
|-----|-----|
| `--engine-extra-args` (flag) | `--engine-args` |
| `engineExtraArgsFlagName` (constant) | `engineArgsFlagName` |
| `parseEngineExtraArgsFlag()` | `parseEngineArgsFlag()` |
| API payload key `"engine_extra_args"` | `"engine_args"` |

## Frontend

### TypeScript Types (`lib/types.ts`, `lib/api.ts`, `hooks/use-workers.ts`)

| Old | New |
|-----|-----|
| `engine_extra_args?: Record<string, string>` | `engine_args?: Record<string, string>` |

### React Component

| Old | New |
|-----|-----|
| File `engine-extra-args-section.tsx` | `engine-args-section.tsx` |
| `EngineExtraArgsSection` | `EngineArgsSection` |
| `EngineExtraArgsSectionProps` | `EngineArgsSectionProps` |
| Variable `engineExtraArgs` | `engineArgs` |

### Imports in `edit-worker-info-sheet.tsx`

Update import path and component reference.

## Internationalization

### Translation Keys

| Old Key | New Key |
|---------|---------|
| `workers.form.engineExtraArgs` | `workers.form.engineArgs` |
| `workers.form.engineExtraArgsPlaceholder` | `workers.form.engineArgsPlaceholder` |
| `workers.form.engineExtraArgsHelper` | `workers.form.engineArgsHelper` |

### Display Strings

| Locale | Old | New |
|--------|-----|-----|
| `en.json` | "Engine Extra Args" | "Engine Args" |
| `zh.json` | "引擎额外参数" | "引擎参数" |

Helper text in `en.json` and `zh.json` updated to remove "extra".

## Documentation

- `skills/openbee-bee/references/cli-reference.md`: Replace all `--engine-extra-args` with `--engine-args`.

## Files Affected

| File | Change Type |
|------|-------------|
| `internal/infra/store/db.go` | Edit migration #44 column name |
| `internal/infra/model/system_config.go` | Rename constants |
| `internal/infra/model/worker.go` | Rename field, JSON/DB tags |
| `internal/infra/store/worker_store.go` | Rename SQL column references |
| `internal/domain/worker/worker.go` | Rename field, JSON tag |
| `internal/domain/worker/manager.go` | Rename functions, type refs, strings |
| `internal/domain/worker/manager_test.go` | Rename test functions |
| `internal/domain/bee/bee_process.go` | Rename type refs, strings |
| `internal/ai/extra_args.go` → `internal/ai/engine_args.go` | Rename file + type + functions |
| `internal/ai/extra_args_test.go` → `internal/ai/engine_args_test.go` | Rename file + test functions |
| `internal/api/worker_handler.go` | Rename field, JSON tag, helper function |
| `internal/api/worker_handler_test.go` | Rename field refs |
| `internal/api/system_config_handler.go` | Rename constant refs |
| `internal/api/system_config_handler_test.go` | Rename constant refs |
| `cmd/openbee/ctl_worker.go` | Rename flag, constant, function, payload key |
| `web/src/lib/types.ts` | Rename interface field |
| `web/src/lib/api.ts` | Rename interface field |
| `web/src/hooks/use-workers.ts` | Rename field |
| `web/src/components/engine-extra-args-section.tsx` | Rename file + component + props |
| `web/src/components/edit-worker-info-sheet.tsx` | Update import + field refs |
| `web/src/locales/en.json` | Rename keys + update display strings |
| `web/src/locales/zh.json` | Rename keys + update display strings |
| `internal/infra/skillinstall/skills/openbee-bee/references/cli-reference.md` | Update flag name |

## Success Criteria

- All Go tests pass (`go test ./...`)
- Frontend builds without type errors (`npm run build`)
- CLI `--engine-args` flag works end-to-end
- No references to `engine_extra_args` remain in the codebase (except git history)
