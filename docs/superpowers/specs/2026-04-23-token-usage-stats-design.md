# Token Usage Statistics — Storage Design

**Date:** 2026-04-23  
**Scope:** Data collection and storage only (no query API / UI in this phase)  
**Supported agents:** Claude Code, Codex, Pi (Kimi excluded — no token output)

---

## Background

`bee_executions` records each message execution as a subset of an agent session. To track token consumption, we need session-level aggregation grouped by `(session_id, model)`.

Session ID mapping per agent:
- **Claude Code / Pi:** `bee_executions.session_id` = agent session ID directly
- **Codex:** `bee_executions.session_id` → read `~/.openbee/.codex/sessions/{session_id}` (plain text) → real Codex session ID

---

## Architecture

```
bee_executions (session_ids + engine via bee_workers)
        ↓ query distinct session_ids by date window
TokenStatsSyncer  ← background goroutine, every 10 minutes
        ↓ dispatch by engine
Claude Parser / Codex Parser / Pi Parser
        ↓ parse JSONL, aggregate by (session_id, model)
bee_token_stats   ← INSERT OR REPLACE upsert
```

**Engine resolution:** `bee_executions.worker_id` → `bee_workers.engine` → one of `claude` / `codex` / `pi`

---

## Database Schema

New migration **#41**:

```sql
CREATE TABLE IF NOT EXISTS bee_token_stats (
    id                    TEXT    PRIMARY KEY,
    session_id            TEXT    NOT NULL,
    agent_type            TEXT    NOT NULL,   -- 'claude' | 'codex' | 'pi'
    model                 TEXT    NOT NULL,
    input_tokens          INTEGER NOT NULL DEFAULT 0,
    output_tokens         INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
    synced_at             INTEGER NOT NULL  -- Unix milliseconds
);

CREATE UNIQUE INDEX idx_bee_token_stats_session_model
    ON bee_token_stats(session_id, model);

CREATE INDEX idx_bee_token_stats_session_id
    ON bee_token_stats(session_id);
```

### Go Model

```go
// internal/infra/model/token_stats.go
type TokenStats struct {
    ID                  string `json:"id" db:"id"`
    SessionID           string `json:"session_id" db:"session_id"`
    AgentType           string `json:"agent_type" db:"agent_type"`
    Model               string `json:"model" db:"model"`
    InputTokens         int64  `json:"input_tokens" db:"input_tokens"`
    OutputTokens        int64  `json:"output_tokens" db:"output_tokens"`
    CacheCreationTokens int64  `json:"cache_creation_tokens" db:"cache_creation_tokens"`
    CacheReadTokens     int64  `json:"cache_read_tokens" db:"cache_read_tokens"`
    SyncedAt            int64  `json:"synced_at" db:"synced_at"`
}
```

---

## Sync Job — TokenStatsSyncer

### File Layout

```
internal/tokenstat/
    syncer.go        # background sync job
    parser.go        # Parser interface + SessionTokenUsage type
    claude.go        # Claude Code JSONL parser
    codex.go         # Codex JSONL parser
    pi.go            # Pi Agent JSONL parser
internal/infra/
    model/token_stats.go
    store/token_stats_store.go
```

### Startup Behavior

```
On start:
  IF bee_token_stats is empty:
    full mode → collect ALL distinct session_ids from bee_executions
  ELSE:
    incremental mode → collect session_ids with completed_at > now - 30 days

Then enter tick loop (every 10 minutes):
  always use incremental mode
```

### Per-Session Sync

```
1. JOIN bee_executions + bee_workers to get engine for session_id
2. Select parser by engine
3. parser.Parse(sessionID) → []SessionTokenUsage
4. For each usage: INSERT OR REPLACE INTO bee_token_stats
```

### Common Interface

```go
// internal/tokenstat/parser.go

type SessionTokenUsage struct {
    SessionID           string
    AgentType           string
    Model               string
    InputTokens         int64
    OutputTokens        int64
    CacheCreationTokens int64
    CacheReadTokens     int64
}

type Parser interface {
    Parse(sessionID string) ([]SessionTokenUsage, error)
}
```

### Error Handling

| Scenario | Action |
|----------|--------|
| JSONL file not found | warn log, skip session |
| Single line parse failure | debug log, skip line, continue |
| Codex mapping file not found | warn log, skip session |
| DB upsert failure | error log, continue to next session |
| Full batch failure | no retry, wait for next tick |

---

## Parser Details

### Claude Code Parser

**File location:**
1. Check env `CLAUDE_CONFIG_DIR` (comma-separated paths)
2. Fallback: `~/.claude/projects/` then `~/.config/claude/projects/`
3. File path: `{base}/projects/{session_id}.jsonl` (session ID is path relative to `projects/`)

**Parse logic:**
- Skip lines without `message.usage`
- `model` = `message.model`; if `message.speed == "fast"` append `-fast`
- Accumulate: `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`
- Aggregate by model

**Field mapping:**

| Target field | Source |
|---|---|
| inputTokens | `message.usage.input_tokens` |
| outputTokens | `message.usage.output_tokens` |
| cacheCreationTokens | `message.usage.cache_creation_input_tokens` |
| cacheReadTokens | `message.usage.cache_read_input_tokens` |

---

### Codex Parser

**File location (two-step):**
1. Read `~/.openbee/.codex/sessions/{openbee_session_id}` → real Codex session ID (plain text)
2. Check env `CODEX_HOME`, fallback `~/.codex/sessions/`
3. File path: `{base}/sessions/{codex_session_id}.jsonl`

**Parse logic:**
- Maintain current `model` (updated by `turn_context` events)
- Maintain `prevTotalUsage` for delta calculation
- On `type == "turn_context"`: update current model from `payload.model`
- On `type == "event_msg"` with `info.last_token_usage`: use directly
- On `type == "event_msg"` with `info.total_token_usage` but no `last_token_usage`: delta = current total − prev total; update prev total
- `cacheCreationTokens` always 0
- `cached_input_tokens` → `cacheReadTokens`
- Aggregate by model

**Field mapping:**

| Target field | Source |
|---|---|
| inputTokens | `info.last_token_usage.input_tokens` |
| outputTokens | `info.last_token_usage.output_tokens` (includes reasoning tokens) |
| cacheCreationTokens | always 0 |
| cacheReadTokens | `info.last_token_usage.cached_input_tokens` |

---

### Pi Agent Parser

**File location:**
1. Check env `PI_AGENT_DIR`, fallback `~/.pi/agent/sessions/`
2. Session ID appears after the first `_` in the filename → glob `*.jsonl`, filter by `_{session_id}` in filename

**Parse logic:**
- Skip lines where `type != "message"`
- Skip lines where `message.role != "assistant"`
- Skip lines without `message.usage`
- `model` = `"[pi]" + message.model`
- Accumulate: `usage.input`, `usage.output`, `usage.cacheWrite`, `usage.cacheRead`
- Aggregate by model

**Field mapping:**

| Target field | Source |
|---|---|
| inputTokens | `message.usage.input` |
| outputTokens | `message.usage.output` |
| cacheCreationTokens | `message.usage.cacheWrite` |
| cacheReadTokens | `message.usage.cacheRead` |

---

## Parser Comparison

| | Claude Code | Codex | Pi |
|--|--|--|--|
| File location | direct path | two-step mapping | glob match |
| Model source | per-line `message.model` | `turn_context` event | per-line `message.model` |
| cacheCreation | ✅ | ❌ always 0 | ✅ |
| Special handling | `-fast` suffix | delta calculation | `[pi]` prefix |

---

## Out of Scope (This Phase)

- Query API / REST endpoints for token stats
- Frontend display / dashboards
- Cost calculation (USD)
- Kimi agent (no token output supported)
- Per-execution granularity (session-level only)
