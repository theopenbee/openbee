# Kimi Engine Support Design

**Date**: 2026-04-16
**Branch**: feat/worker-engine-selection

## Overview

Add Kimi (月之暗面) as a fourth supported AI engine alongside Claude, Codex, and Pi. Kimi is invoked via CLI, outputs stream-JSON in a role-based message format, and integrates into the existing per-worker engine selection system.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Session management | Pass `opts.SessionID` directly as `--session=<UUID>` | Kimi natively accepts UUID format; no mapping store needed |
| Configuration | `env` map (same as Pi) | Flexible env var injection for MOONSHOT_API_KEY etc.; consistent with Pi pattern |
| Backend result extraction | Last `role=assistant` message | Simple, reliable; handles both string and array content formats |
| Frontend log detection | Top-level `role` field without `type` field | Kimi is the only engine using role-based top-level format |

## Kimi CLI Invocation

```
echo "<prompt>" | kimi --session=<UUID> --yolo --output-format=stream-json --print
```

- Prompt is piped via stdin
- `--session=<UUID>` maps directly to `opts.SessionID`
- `--yolo` skips interactive approvals (analogous to `--dangerously-skip-permissions` for Claude)
- `--output-format=stream-json` + `--print` for non-interactive JSON stream output

## Kimi Output Format

Kimi emits one JSON object per line, each with a `role` field:

```json
{"role": "user", "content": "prompt text"}
{"role": "assistant", "content": "reply text"}
{"role": "assistant", "content": [{"type": "text", "text": "reply"}], "tool_calls": [{"type": "function", "id": "tc_1", "function": {"name": "Shell", "arguments": "{\"command\":\"ls\"}"}}]}
{"role": "tool", "tool_call_id": "tc_1", "content": "tool result"}
```

The `assistant` content field is either a plain string or an array of content blocks.

## Backend Implementation

### 1. Engine Constants (`internal/ai/engine.go`)

Add:
```go
EngineKimi = "kimi"
```

Add `"kimi"` to `AllEngines`.

### 2. New Package: `internal/ai/kimi/`

**`adapter.go`**
- Registers factory via `init()`
- Reads `path` and `env` from `cfg.Raw`
- `Prepare()` is a no-op (Kimi needs no workspace cleanup)
- Delegates `Run()` and `ExtractResult()` to Invoker

**`invoker.go`**
- `Invoker` struct: `binary string`, `baseEnv []string`
- Constructor takes `binary`, `openbeeURL`, `extraEnv map[string]string` (same pattern as Pi)
- `buildArgs(sessionID string) []string` returns `["--session=<UUID>", "--yolo", "--output-format=stream-json", "--print"]`
- `Run()`: pipes prompt via stdin, writes stdout+stderr to logFile, emits `OutputDone` or `OutputError` based on exit code
- `ExtractResultFromLog(logPath string) string`: scans log for last `role=assistant` line; if content is string return it; if array return first `type=text` block's text

### 3. Config (`internal/infra/config/config.go`)

Add:
```go
type KimiConfig struct {
    Path    string            `yaml:"path"`
    Timeout time.Duration     `yaml:"timeout"`
    Env     map[string]string `yaml:"env"`
}
```

Add `Kimi KimiConfig` to `BeeConfig`.

Update `WorkerTimeout()`: add `case "kimi": return b.Kimi.Timeout`.

Update `EngineConfigRawFor()`: add `case "kimi": return map[string]any{"path": b.Kimi.Path, "env": b.Kimi.Env}`.

### 4. App Bootstrap (`internal/app/app.go`)

Add blank import:
```go
_ "github.com/theopenbee/openbee/internal/ai/kimi"
```

### 5. Config Template (`internal/infra/config/config.yaml.tmpl`)

Add kimi section under `bee`:
```yaml
kimi:
  path: kimi
  timeout: 30m
  env:
    MOONSHOT_API_KEY: ""
```

## Frontend Implementation

### 6. Types (`web/src/lib/types.ts`)

Add `"kimi"` to `ENGINES` const array.

### 7. Engine Detection (`web/src/components/log-viewer/detect-engine.ts`)

Add Kimi detection before the default Claude fallback:
- Parse each line as JSON
- If a line has a top-level `role` field and no top-level `type` field → return `"kimi"`

This is safe because Claude/Codex/Pi embed `role` inside nested objects, never at the top level.

### 8. Kimi Parser (`web/src/components/log-viewer/kimi-parser.ts`)

Implements `StreamParser` interface:

| Input line | Action |
|-----------|--------|
| `role=user` | Skip |
| `role=assistant`, string content | Create `text` entry |
| `role=assistant`, array content | Create `text` entry from `type=text` blocks; create in-progress `tool` entry per `tool_calls` item |
| `role=tool` | Look up `tool_call_id` in `itemMap`; update matched `tool` entry with result |
| Non-JSON / unknown | Create `raw` entry |

No new `ParsedEntry` kinds needed; reuses existing `text`, `tool`, and `raw`.

Register the parser in `log-viewer.tsx` alongside the existing three parsers.

### 9. i18n

Add `workers.engines.kimi` translation key to all locale files (e.g., `"Kimi"` / `"Kimi"`).

## Error Handling

- Backend: relies on process exit code; non-zero exit → `OutputError`
- Frontend: malformed/non-JSON lines → `raw` entry (consistent with other parsers)

## Testing

- Backend: `kimi/invoker_test.go` — unit tests for `ExtractResultFromLog` covering string content, array content, tool_calls sequences, empty log
- Frontend: `log-viewer/__tests__/kimi-parser.test.ts` — unit tests covering all role variants and edge cases; `detect-engine.test.ts` — add kimi detection cases
