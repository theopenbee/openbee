# Kimi Token Usage Statistics — Design Spec

Date: 2026-04-23

## Overview

Add token usage tracking for Kimi agent sessions, consistent with existing claude/codex/pi parsers. No schema changes required; the existing `bee_token_stats` table covers all needed fields.

## Data Source

- File location: `~/.kimi/sessions/*/{session_id}/wire.jsonl`
  - One fixed intermediate directory level between `sessions/` and the session directory
  - Discovered via `filepath.Glob(kimiHome + "/sessions/*/" + sessionID + "/wire.jsonl")`
  - `kimiHome` defaults to `~/.kimi`; no environment variable override

- Record format (relevant subset):
  ```json
  {
    "message": {
      "type": "StatusUpdate",
      "payload": {
        "token_usage": {
          "input_other": 446,
          "output": 70,
          "input_cache_read": 16384,
          "input_cache_creation": 0
        }
      }
    }
  }
  ```

- Each `StatusUpdate` records **cumulative** token usage up to that point in the session. The last `StatusUpdate` in the file is the authoritative total.

## Field Mapping

| Kimi field | `bee_token_stats` column |
|---|---|
| `token_usage.input_other` | `input_tokens` |
| `token_usage.output` | `output_tokens` |
| `token_usage.input_cache_read` | `cache_read_tokens` |
| `token_usage.input_cache_creation` | `cache_creation_tokens` |
| _(not available)_ | `model` = `"kimi"` (fixed) |
| `agent_type` | `"kimi"` (fixed) |

## Implementation

### New file: `internal/tokenstat/kimi.go`

- `kimiParser` struct with no fields (home dir resolved at construction via `os.UserHomeDir`)
- `NewKimiParser() Parser`
- `Parse(sessionID string)` algorithm:
  1. Glob `~/.kimi/sessions/*/{sessionID}/wire.jsonl`; if no match → `ErrSessionDataNotFound`
  2. Read all lines of the first match using `scanJSONLFile`
  3. Collect bytes of every line where `message.type == "StatusUpdate"` into a slice
  4. Iterate the slice in reverse; unmarshal the first valid `token_usage` entry
  5. If no `StatusUpdate` found → `ErrSessionDataNotFound`
  6. Return single-element `[]SessionTokenUsage` with fixed model `"kimi"`

### Modified: `internal/tokenstat/syncer.go`

1. Register `KimiParser` in `NewSyncer` parsers map under `ai.EngineKimi`
2. Add `ai.EngineKimi` as a fourth bind parameter in the `collectSessions` SQL query

`defaultParserOrder` (used only for legacy sessions with empty engine) is **not** changed — Kimi sessions always carry an explicit engine hint.

## Error Handling

- `ErrSessionDataNotFound`: no file found, or file has no `StatusUpdate` — logged at debug, non-fatal, consistent with other parsers
- Glob/file read errors: returned as-is, logged at warn by syncer

## Testing

- `internal/tokenstat/kimi_test.go` covering:
  - Happy path: parses last `StatusUpdate` from a multi-record fixture
  - No file → `ErrSessionDataNotFound`
  - File with no `StatusUpdate` → `ErrSessionDataNotFound`
  - Zero-value token fields (all zeros) handled gracefully
