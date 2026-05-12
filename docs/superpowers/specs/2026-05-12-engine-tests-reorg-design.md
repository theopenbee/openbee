# Engine Tests Reorganization Design

**Date**: 2026-05-12
**Scope**: `internal/ai/engine/{claude,codex,pi,kimi}`

## Background

The AI engine packages recently went through a refactor that collapsed the
`Invoker`, `Collector`, and `Extractor` abstractions into a single `Backend`
type (commits `1d1acad`, `dd6dcaf`). The production code was renamed
accordingly, but the test files still carry the old conceptual names
(`invoker_test.go`, `token_usage_test.go`) and a few obsolete patterns:

- `claude/adapter_test.go` actually tests `cleanupLegacyRules`, which lives
  in `backend.go`, not `adapter.go`.
- Backend-related tests are scattered across multiple files
  (`invoker_test.go`, `token_usage_test.go`, plus a stray group in
  `adapter_test.go` for claude).
- Test packages are mixed: most tests use the internal package
  (`package claude`), but `token_usage_test.go` files and
  `codex/adapter_test.go` use the external package (`package claude_test`).
- Two redundant test cases provide no additional coverage.

## Goal

1. Test file names map 1:1 to the source file under test.
2. Unify on internal test packages within `internal/ai/engine/*`.
3. Remove redundant test cases.

## File Changes

### claude/

| Action | From | To |
|---|---|---|
| Merge + repackage | `invoker_test.go` + `token_usage_test.go` + `cleanupLegacyRules*` tests from `adapter_test.go` | `backend_test.go` |
| Rename | `invoker_unix_test.go` | `backend_unix_test.go` (keep `//go:build !windows`) |
| Keep | `download_test.go` | unchanged |
| Keep | `provider_test.go` | unchanged |
| Delete | `adapter_test.go` | — (all cases relocated) |

### codex/

| Action | From | To |
|---|---|---|
| Merge + repackage | `invoker_test.go` + `token_usage_test.go` | `backend_test.go` |
| Keep | `session_store_test.go` | unchanged |
| Delete | `adapter_test.go` | compile-time assertion `var _ ai.EngineAdapter = (*Adapter)(nil)` is moved into `adapter.go` |

### pi/

| Action | From | To |
|---|---|---|
| Merge + repackage | `invoker_test.go` + `token_usage_test.go` | `backend_test.go` |
| Rename | `invoker_bench_test.go` | `backend_bench_test.go` |

### kimi/

| Action | From | To |
|---|---|---|
| Merge + repackage | `invoker_test.go` + `token_usage_test.go` | `backend_test.go` |

## Redundant Test Cases to Remove

1. **`claude/adapter_test.go::TestCleanupLegacyRules_Stub`** — semantically
   equivalent to `TestCleanupLegacyRules_NoopWhenFilesAbsent` in the same
   file (both call `cleanupLegacyRules` on an empty directory and assert no
   error). Drop the stub.
2. **`codex/adapter_test.go::TestAdapter_ExtraEnvInBaseEnv`** — the only
   meaningful assertion is the compile-time `var _ ai.EngineAdapter = a`
   line. Move that assertion into `adapter.go` as a package-level
   `var _ ai.EngineAdapter = (*Adapter)(nil)` and delete the runtime test.

## Implementation Notes

- The `token_usage_test.go` files currently sit in external packages and
  call `claude.NewBackend` / `pi.NewBackendAt` / etc. After repackaging,
  these calls become bare `NewBackend` / `NewBackendAt`.
- Helper functions across the soon-to-be-merged files have overlapping
  names: `writeClaudeTempFile`, `writePiTempFile`, `makeKimiSessionFile`,
  `writeTemp`. Pick a single helper per merged file (or unify into one) to
  avoid duplicate-symbol errors.
- `claude/invoker_test.go::TestMain` (subprocess helper triggered by
  `GO_TEST_EMIT_IS_ERROR=1`) must be preserved verbatim in the merged
  `backend_test.go` — `TestBackend_Run_IsErrorEmitsOutputError` depends on
  it.
- The `claude/adapter.go` source file currently has no dedicated test file
  after this reorg. That is acceptable: `adapter.go` is a 17-line passthrough
  exercised end-to-end via `backend_test.go`.

## Verification

After the reorg:

- `go build ./...` succeeds.
- `go test ./internal/ai/engine/...` is green.
- `go test -bench=. -benchtime=1x ./internal/ai/engine/pi/...` still runs
  the moved benchmarks.

## Out of Scope

- No production code changes beyond the single line moved into
  `codex/adapter.go`.
- No new test coverage. This reorg only relocates and deletes; existing
  assertions are preserved.
