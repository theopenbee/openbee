# Tokenstat Engine Cohesion Design

**Date:** 2026-04-26
**Status:** Draft (pending review)
**Authors:** 貂蝉 (worker), in collaboration with the project owner

## Background

`internal/tokenstat` is currently a central module that polls the database every 10 minutes for completed sessions, then dispatches each session to one of four per-engine parsers (`claude.go`, `codex.go`, `pi.go`, `kimi.go`). Each parser walks an engine-specific session directory (e.g. `~/.claude/projects/...`, `~/.codex/sessions/...`), parses JSONL, and emits aggregated `SessionTokenUsage` rows that the syncer writes to `bee_token_stats`.

This layout creates several sources of friction:

- Adding a new engine requires touching both `internal/ai/<engine>/` and `internal/tokenstat/` (parser registration, fallback order, file walking, field mapping).
- Engine-specific knowledge (where session JSONL lives, how it's named, how field maps onto `TokenUsage`) is duplicated between the engine package and the parser. Codex's `SessionStore` already lives in `internal/ai/codex/`, but the parser in `tokenstat` reads it via filesystem layout knowledge — an implicit cross-package contract.
- The syncer's "preferred parser then fallback chain" logic costs unnecessary file walks for sessions whose engine is already known.

## Goal

Move the per-engine token-usage extraction logic into each engine's package. `tokenstat` becomes a thin orchestrator that knows about scheduling, DB I/O, retry budget, and tombstones — but knows nothing about engine-specific file layouts or formats.

User-visible behavior (sync cadence, retry semantics, tombstone behavior, DB schema) does not change.

## Non-Goals

- Changing the polling cadence or switching to push/event-driven collection. (Considered and rejected: external agents flush JSONL asynchronously; polling stays the simplest correct mechanism.)
- Modifying the `bee_token_stats` schema or the `bee_executions` schema.
- Refactoring engine adapters' `Prepare`/`Run` paths.
- Removing the legacy fallback chain. Older `bee_executions` rows have an empty `engine` field and still need it.

## Architecture

```
internal/
  ai/
    engine.go               (modified) EngineAdapter gains CollectTokenUsage
    types.go                (new)      TokenUsage struct, ErrSessionDataNotFound
    sessionfile/            (new)      Shared JSONL walk/scan helpers
    claude/
      adapter.go            (existing)
      token_usage.go        (new)      Claude collector (migrated from tokenstat/claude.go)
      token_usage_test.go   (new)
    codex/
      adapter.go            (existing)
      session_store.go      (existing) Now used in-package by the collector
      token_usage.go        (new)
      token_usage_test.go   (new)
    pi/
      adapter.go            (existing)
      token_usage.go        (new)
      token_usage_test.go   (new)
    kimi/
      adapter.go            (existing)
      token_usage.go        (new)
      token_usage_test.go   (new)
  tokenstat/
    syncer.go               (modified) Dispatch + fallback + retry + tombstone only
    syncer_test.go          (modified) Uses fake collectors via the EngineAdapter mock
    (claude.go / codex.go / pi.go / kimi.go / parser.go / session_files.go — deleted)
```

Dependency direction: `tokenstat` → `internal/ai` (single-direction). Engine packages do not depend on each other and do not depend on `tokenstat`.

## Interface Contract

### `internal/ai/types.go` (new)

```go
package ai

import (
    "errors"
    "time"
)

// TokenUsage represents one model invocation's token consumption,
// emitted by an engine's CollectTokenUsage method. Field shape mirrors
// the previous tokenstat.SessionTokenUsage so the bee_token_stats schema
// and DB layer are unchanged.
type TokenUsage struct {
    Model            string
    InputTokens      int64
    OutputTokens     int64
    CacheReadTokens  int64
    CacheWriteTokens int64
    Timestamp        time.Time // optional; engine fills if available
}

// ErrSessionDataNotFound signals that the engine could not yet locate
// session data (file not flushed, mapping missing, etc.). The syncer
// treats this as "retry within budget, then tombstone".
var ErrSessionDataNotFound = errors.New("ai: session data not found")
```

### `internal/ai/engine.go` (modified)

```go
type EngineAdapter interface {
    Prepare(workDir string, opts PrepareOptions) error
    Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (RunResult, error)

    // CollectTokenUsage extracts token usage for a completed session.
    // Returning (nil, ErrSessionDataNotFound) tells the syncer the data
    // is not yet available and to retry later (or tombstone after budget
    // exhaustion). Returning ([], nil) means "session exists and
    // verifiably has no usage" — the syncer tombstones it immediately.
    CollectTokenUsage(ctx context.Context, sessionID string) ([]TokenUsage, error)
}
```

The method is mandatory. All four current engines implement it; the compiler enforces this.

### Error semantics

- `ErrSessionDataNotFound` → data not yet ready; syncer applies the existing retry budget and may eventually tombstone.
- Any other non-nil error → handled identically to the current syncer's behavior for non-sentinel errors (preserved verbatim — this design does not introduce new error policy).
- `([], nil)` → session located but verifiably empty; syncer tombstones immediately. This matches the 6c0e0cd fix (write a tombstone for sessions whose usages list is empty, preventing infinite re-sync).

### `internal/ai/sessionfile/` (new)

Shared, optional helpers. Not all engines must use them — codex/kimi have idiosyncratic discovery — but `WalkJSONL` and `ScanJSONLLines` cover the common case for claude and pi, and the line-scanning helper is useful to all four.

```go
package sessionfile

func WalkJSONL(root string, match func(name string) bool) ([]string, error)
func ScanJSONLLines(path string, fn func(line []byte) error) error
```

## Syncer Behavior

```go
type Syncer struct {
    db         *sql.DB
    collectors map[string]ai.EngineAdapter // engine name → adapter
    fallback   []ai.EngineAdapter          // legacy fallback chain, fixed order
    interval   time.Duration               // default 10m
    // retry budget / tombstone configuration
}

func (s *Syncer) syncSession(ctx context.Context, sess pendingSession) {
    var collector ai.EngineAdapter
    if sess.Engine != "" {
        collector = s.collectors[sess.Engine]
    }

    if collector != nil {
        s.tryOne(ctx, sess, collector)
        return
    }

    // Legacy fallback: bee_executions rows with empty Engine field
    // (older versions did not record engine). Walk the chain, advancing
    // only when the collector reports ErrSessionDataNotFound; any other
    // outcome (success, empty result, hard error) is terminal for this
    // session in this round.
    for _, c := range s.fallback {
        err := s.tryOne(ctx, sess, c)
        if !errors.Is(err, ai.ErrSessionDataNotFound) {
            return
        }
    }
    s.tombstoneIfBudgetExhausted(sess)
}
```

Behavior summary:

- Engine known → exactly one collector call (no more 4× walk-all).
- Engine empty → legacy fallback chain, ordered identically to the current implementation. The order is preserved verbatim so historic sessions classify the same way.
- Retry budget and tombstone logic are unchanged from today.

## Wiring

At server startup (in the entrypoint that constructs `EngineAdapter`s), inject the adapters into the syncer:

```go
adapters := map[string]ai.EngineAdapter{
    "claude": claudeAdapter,
    "codex":  codexAdapter,
    "pi":     piAdapter,
    "kimi":   kimiAdapter,
}
syncer := tokenstat.NewSyncer(db, adapters,
    tokenstat.WithFallbackOrder("claude", "codex", "pi", "kimi"))
```

If the existing wiring constructs adapters lazily, minor adjustment may be needed to ensure they are ready before the syncer starts. No new lifecycle changes beyond that.

## Testing

- The four existing parser unit-test files migrate to their engine packages with the same test cases. Imports change from `tokenstat.SessionTokenUsage` / `tokenstat.ErrSessionDataNotFound` to `ai.TokenUsage` / `ai.ErrSessionDataNotFound`. Behavior assertions are unchanged.
- `syncer_test.go` is rewritten against a fake `EngineAdapter` (a mock that returns scripted `[]TokenUsage` / errors). It focuses on dispatch correctness, fallback chain, retry budget, and tombstone behavior.
- New integration tests cover two paths explicitly: "engine known → direct dispatch" and "engine empty → fallback chain".

## Migration

Single PR. The repository must compile cleanly at every step:

1. Add `internal/ai/types.go`, `internal/ai/sessionfile/`, and the `CollectTokenUsage` method on `EngineAdapter`. Provide a temporary stub on each adapter so the build still passes.
2. Implement `token_usage.go` in each of the four engine packages, migrating logic from the matching `tokenstat/<engine>.go`. Translate `tokenstat.SessionTokenUsage` → `ai.TokenUsage` and `tokenstat.ErrSessionDataNotFound` → `ai.ErrSessionDataNotFound`. The codex collector calls the in-package `SessionStore` directly.
3. Migrate the four parser test files alongside the implementations.
4. Refactor `syncer.go` to dispatch via the injected adapter map plus the fallback slice. Delete the parser registry.
5. Update the wiring code at the server entrypoint to inject adapters into the syncer.
6. Delete `tokenstat/claude.go`, `tokenstat/codex.go`, `tokenstat/pi.go`, `tokenstat/kimi.go`, `tokenstat/parser.go`, `tokenstat/session_files.go`, and their tests.
7. Run `make test` and smoke-test each engine's token sync end to end.

## Risks and Mitigations

- **Fallback order regression for legacy sessions.** Mitigation: the new `WithFallbackOrder` parameter is wired with the exact order that the current syncer iterates today; verified against `syncer.go` before deletion.
- **Codex SessionStore access pattern change.** The current parser reads codex sessions via filesystem layout knowledge of `~/.codex/sessions/...`; after the refactor, the codex collector lives in the same package as `SessionStore` and uses it directly. This removes an implicit cross-package dependency.
- **Adding a method to `EngineAdapter` is a breaking change for the interface.** The interface is internal and has exactly four implementations, all in this repository, so the compiler surfaces any missing implementation at build time. No external consumers.
- **Behavioral parity.** The DB schema, sync cadence, retry budget defaults, and tombstone semantics are unchanged. Existing token-stats data continues to work without migration.
