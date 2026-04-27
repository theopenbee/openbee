# ai Package Reorganization — Design Spec

**Date:** 2026-04-26  
**Branch:** feat/tokenstat-engine-cohesion  
**Scope:** `internal/ai/` root package + `internal/ai/claude/` subpackage

---

## Problem

The `internal/ai` root package contains several files with naming or scoping issues that create unnecessary cognitive friction:

| File | Issue |
|------|-------|
| `engine.go` | Name implies an engine implementation; actual content is the core plugin contracts (interfaces + types) |
| `types.go` | Only 3 items (`TokenUsage`, `DrainUsageMap`, `ErrSessionDataNotFound`); split from the closely-related `engine.go` for no clear reason |
| `scan.go` | Single-function file (`ScanJSONLines`); feels orphaned |
| `rules.go` | Name implies business-rule validation; actual content is prompt string builders |
| `claude/claudemd.go` | Single private function (`removeImportLine`) called only from `adapter.go`; unnecessary indirection |

---

## Chosen Approach: Option B — Rename for Semantic Clarity

Make every file name immediately answer "what's in here?" through targeted renames, merges, and one inline. No logic changes.

---

## Design

### `internal/ai/` root package

#### 1. `engine.go` → `contracts.go` (rename only)

`contracts.go` signals "the plugin contract that all engine adapters must satisfy." The file already contains exactly that — the `EngineAdapter` and `Process` interfaces, plus all the types and constants that compose those interfaces (`PrepareOptions`, `RunOptions`, `Output`, `RunResult`, `Role`, engine name constants, `AllEngines()`).

No content changes. Pure rename.

#### 2. `types.go` → merged into `contracts.go`, then deleted

`types.go` currently holds `TokenUsage`, `DrainUsageMap`, and `ErrSessionDataNotFound`. These belong logically with the contract: `EngineAdapter.CollectTokenUsage` returns `[]TokenUsage`, making the split across two files an arbitrary accident. Merge and delete `types.go`.

Placement within `contracts.go`: append at the end, after the `EngineAdapter` interface block.

#### 3. `scan.go` → merged into `process.go`, then deleted

`ScanJSONLines` is a low-level line-scanning utility used when parsing engine process output. It fits naturally alongside `CmdProcess`, `BuildRunEnv`, and `BuildBaseEnv` in `process.go`. Eliminates an orphan single-function file.

Placement within `process.go`: append at the end, after the env-builder functions.

#### 4. `rules.go` → `prompt.go` (rename only)

`WorkerPersona()` and `SkillHintPrefix()` are functions that construct prompt strings injected into AI sessions. "Rules" implies validation or business logic. "Prompt" accurately describes the content.

No content changes. Pure rename. The corresponding test file `rules_test.go` → `prompt_test.go`.

---

### `internal/ai/claude/` subpackage

#### 5. `claudemd.go` → inlined into `adapter.go`, then deleted

`removeImportLine()` is a single private function called exactly once, from `claudeAdapter.Prepare()`. Having it in its own file adds a navigation hop with no benefit. Inlining makes `Prepare()`'s complete logic visible in one place.

The function body and its imports (`bytes`, `errors`, `fmt`, `io/fs`, `os`, `path/filepath`) move into `adapter.go`.

---

### No changes

The following are already well-structured and remain untouched:

- `registry.go` — single responsibility, accurate name
- `dynamic.go` — single responsibility, accurate name
- `engine_args.go` — single responsibility, accurate name
- `codex/`, `kimi/`, `pi/` subpackages — consistent 3-file pattern (adapter + invoker + token_usage) is the right model

---

## File Count Delta

| Location | Before | After | Change |
|----------|--------|-------|--------|
| `internal/ai/` (non-test) | 8 | 6 | −2 (`types.go`, `scan.go` deleted; `engine.go`→`contracts.go`, `rules.go`→`prompt.go`) |
| `internal/ai/claude/` (non-test) | 6 | 5 | −1 (`claudemd.go` inlined into `adapter.go`) |

---

## Implementation Steps

1. Create `contracts.go` with content of `engine.go` + appended content of `types.go`; delete `engine.go` and `types.go`
2. Append `ScanJSONLines` to `process.go`; delete `scan.go`
3. Rename `rules.go` → `prompt.go`; rename `rules_test.go` → `prompt_test.go`
4. Inline `removeImportLine` (and its imports) from `claudemd.go` into `claude/adapter.go`; delete `claudemd.go`
5. Run `go build ./...` and `go test ./...` to verify no regressions

---

## Risk

All changes are renames, merges, and one inline. Zero logic changes. Go's package system is file-agnostic: callers reference `ai.TokenUsage`, not `ai/types.TokenUsage`, so no import paths change anywhere. Risk is minimal.
