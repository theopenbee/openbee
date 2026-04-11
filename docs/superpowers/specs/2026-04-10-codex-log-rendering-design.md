# Codex Log Rendering Adaptation — Design Spec

**Date:** 2026-04-10
**Branch:** feat/engine-plugin
**Author:** 毛毛

---

## Background

The session detail page (`/sessions/detail?session_id=...`) uses a `LogViewer` component to render execution logs. Currently, only Claude Code's event format is supported. With Codex now integrated as an AI engine, its NDJSON output format needs to be parsed and rendered.

### Claude Code Event Format (current)
```
{"type":"assistant","message":{"content":[{"type":"text","text":"..."},{"type":"tool_use","id":"...","name":"...","input":{}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"...","content":"...","is_error":false}]}}
{"type":"result","result":"...","subtype":"..."}
{"type":"system",...}
```

### Codex Event Format (new)
```
{"type":"thread.started","thread_id":"0199a213-81c0-7800-8aa1-bbab2a035a53"}
{"type":"turn.started"}
{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"bash -lc ls","status":"in_progress"}}
{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":"Repo contains docs, sdk, and examples directories."}}
{"type":"turn.completed","usage":{"input_tokens":24763,"cached_input_tokens":24448,"output_tokens":122}}
```

---

## Decisions

| Question | Decision |
|---|---|
| Engine detection | Runtime auto-detection: check if first line is `thread.started` → Codex, otherwise → Claude |
| `turn.started` / `turn.completed` | `turn.completed` shows token usage chip; `turn.started` is ignored |
| `item.started` command_execution | Render immediately as in-progress card; update when `item.completed` arrives |
| Unknown item types | Fall through to `raw` entry |
| Implementation approach | Abstract `StreamParser` interface; Claude and Codex each implement it |

---

## Architecture

### File Structure

```
web/src/components/
  log-viewer.tsx              ← Orchestrator; delegates parsing to parser instance
  log-viewer/
    types.ts                  ← ParsedEntry union type + StreamParser interface
    claude-parser.ts          ← Existing Claude parsing logic (migrated)
    codex-parser.ts           ← New Codex parsing logic
    detect-engine.ts          ← Engine auto-detection (inspects first log line)
```

### StreamParser Interface

```typescript
// types.ts
interface StreamParser {
  parseLine(
    line: string,
    logType: string,
    entries: ParsedEntry[],
    itemMap: Map<string, number>
  ): void
}
```

### Engine Detection

`detect-engine.ts` exports a single function:

```typescript
function detectEngine(firstLine: string): "claude" | "codex"
```

Logic: parse `firstLine` as JSON; if `type === "thread.started"` → `"codex"`; otherwise → `"claude"`. On parse failure → `"claude"` (safe fallback).

### log-viewer Integration

- Add `parserRef = useRef<StreamParser | null>(null)` 
- Add `engineDetectedRef = useRef(false)`
- In `appendLines`, before calling `parseLine`, check if engine has been detected; if not, call `detectEngine(lines[0])`, create parser, set `engineDetectedRef`
- Reset both refs in the `executionId` change effect

---

## Data Types

### Full ParsedEntry Union

```typescript
type ParsedEntry =
  // Existing (unchanged)
  | { kind: "text"; text: string }
  | { kind: "tool"; id: string; name: string; input: unknown; result?: string; isError?: boolean }
  | { kind: "result"; text: string; subtype: string }
  | { kind: "raw"; content: string; logType: string }
  // New — Codex
  | {
      kind: "codex-command"
      id: string
      command: string
      output?: string      // populated from item.text or item.output on item.completed; exact field TBD empirically
      inProgress: boolean
    }
  | {
      kind: "codex-turn"
      inputTokens: number
      cachedInputTokens: number
      outputTokens: number
    }
```

### Codex Event → Entry Mapping

| Codex event type | item.type | Result |
|---|---|---|
| `thread.started` | — | Ignored |
| `turn.started` | — | Ignored |
| `item.started` | `command_execution` | New `codex-command` entry (`inProgress: true`) |
| `item.completed` | `command_execution` | Update matching `codex-command` (`inProgress: false`, fill output from `item.text` or `item.output` — verify empirically) |
| `item.completed` | `agent_message` | New `text` entry (reuses AssistantEntry) |
| `item.completed` | other | New `raw` entry |
| `turn.completed` | — | New `codex-turn` entry with token counts |

### itemMap Usage

`CodexParser` reuses the existing `itemMap: Map<string, number>` passed by log-viewer.
- Key: Codex `item.id`
- Value: index into `entries` array
- Used by `item.completed` to locate and update the matching in-progress entry

---

## UI Components

### CodexCommandEntry

Renders a `codex-command` entry. Structurally mirrors `ToolEntry`.

- **Timeline marker color:**
  - `inProgress: true` → `bg-primary/55` (blue pulsing)
  - completed, no error → `bg-status-idle` (green)
  - error → `bg-destructive` (red)
- **Label chip:** `SH` (consistent with Claude's Bash tool)
- **Header:** "Command" label + truncated command string
- **Expanded body (two-column):**
  - Left: full command text
  - Right: output (shows `t("logViewer.running")` while `inProgress`)
- Starts collapsed; auto-expands if there's an error

### CodexTurnEntry

Renders a `codex-turn` entry. Lightweight row, no collapsing.

- **Timeline marker:** `bg-muted-foreground/40` (gray, low visual weight)
- **Content:** Three `MetricChip` instances for input / cached / output tokens
- **Label:** "Turn" prefix before chips
- Always visible regardless of active filter (same behavior as `result`)

### Filter Behavior

| Entry kind | Counted in filter | Filter key |
|---|---|---|
| `text` | yes | `text` |
| `tool` | yes | `tool` |
| `codex-command` | yes | `tool` (grouped with tool calls) |
| `raw` | yes | `raw` |
| `result` | always shown | — |
| `codex-turn` | always shown | — |

---

## Error Handling

| Scenario | Behavior |
|---|---|
| JSON parse failure on any line | `appendRawEntry` with original line content |
| `item.completed` with unknown `id` | Skip silently |
| Engine detection failure (bad JSON) | Default to `ClaudeParser` |
| Missing `usage` on `turn.completed` | Skip codex-turn entry creation |

---

## i18n Additions

Add to `web/src/locales/en.json` under `logViewer`:

```json
"commandExecution": "Command",
"running": "Running...",
"turnUsage": "Turn",
"inputTokens": "input",
"cachedTokens": "cached",
"outputTokens": "output"
```

Add corresponding entries to `zh.json` (and any other locale files present).

---

## Testing Strategy

- **`detect-engine.ts`:** Unit test with `thread.started` line, non-Codex line, and malformed JSON
- **`codex-parser.ts`:** Unit tests for each event type (thread.started, item.started, item.completed agent_message, item.completed command_execution, item.completed unknown, turn.completed)
- **`claude-parser.ts`:** Existing behavior preserved; smoke-test the migration didn't break anything
- **Manual:** Run a Codex execution, open session detail, verify rendering of all event types
