# Pi Agent Log Rendering Design

**Date:** 2026-04-11
**Branch:** feat/engine-plugin

## Overview

Add support for rendering pi agent logs in the session detail page's log viewer. Pi agent is a third AI engine (alongside Claude and Codex) whose logs follow the pi-mono RPC event format streamed as NDJSON to stdout.

## Requirements

- Real-time streaming feel: tool execution shows in-progress state then final result
- Show thinking blocks (agent internal reasoning), collapsible
- Flat timeline layout (no turn grouping), consistent with Claude/Codex rendering
- Hybrid parsing: tool execution events handled in real-time; text/thinking rendered on `message_end`

## Architecture

### Engine Detection

Update `detect-engine.ts` to detect pi logs by checking the first line's event type:

| First line `type` | Engine |
|-------------------|--------|
| `assistant` | `"claude"` |
| `thread.started` | `"codex"` |
| `agent_start` | `"pi"` (new) |

Return type changes: `"claude" | "codex"` → `"claude" | "codex" | "pi"`

### New ParsedEntry Kind

Add to `types.ts`:

```typescript
| { kind: "pi-thinking"; id: string; thinking: string }
```

All other entry kinds are reused:
- `{ kind: "text" }` — assistant text output (extracted from `message_end`)
- `{ kind: "tool" }` — tool calls (extracted from `tool_execution_start` / `tool_execution_end`)
- `{ kind: "raw" }` — fallback for unknown/malformed events

### File Changes

| File | Change |
|------|--------|
| `web/src/components/log-viewer/types.ts` | Add `pi-thinking` ParsedEntry kind |
| `web/src/components/log-viewer/detect-engine.ts` | Add `agent_start` → `"pi"` detection branch |
| `web/src/components/log-viewer/pi-parser.ts` | New file implementing `StreamParser` interface |
| `web/src/components/log-viewer/__tests__/pi-parser.test.ts` | New file with unit tests |
| `web/src/components/log-viewer/__tests__/detect-engine.test.ts` | Add pi detection test cases |
| `web/src/components/log-viewer.tsx` | Register pi parser, add `PiThinkingEntry` component, update filter logic |

## PiParser Logic

Implements the `StreamParser` interface. Event handling strategy:

| Event | Action |
|-------|--------|
| `agent_start` / `agent_end` | Ignore (`agent_end` messages already covered by `message_end`) |
| `turn_start` / `turn_end` | Ignore (no turn grouping) |
| `queue_update` | Ignore |
| `compaction_start` / `compaction_end` | Ignore |
| `message_start` / `message_update` | Ignore (text waits for `message_end`) |
| `message_end` | Extract thinking blocks → `pi-thinking` entries; extract text blocks → `text` entries; ignore `tool_use` blocks (handled by tool_execution events) |
| `tool_execution_start` | Create `tool` entry with `inProgress=true`; store `toolCallId → index` in `itemMap` |
| `tool_execution_end` | Look up entry via `itemMap`, fill in result content and `isError` flag |
| `auto_retry_start` | Append raw entry: "Retrying (attempt N/max)..." |
| `auto_retry_end` (failed) | Append raw entry with `finalError` message |
| `extension_error` | `appendRawEntry` fallback |
| Unknown events | `appendRawEntry` fallback |

### Thinking Block ID Generation

`message_end` thinking blocks have no standalone ID. Generate stable keys using:
`"thinking-{messageIndex}-{blockIndex}"`

where `messageIndex` increments per `message_end` event and `blockIndex` is the position within the content array.

### message_end Content Extraction

The `message_end.message` follows Anthropic's message format with a `content` array:

```json
{
  "role": "assistant",
  "content": [
    { "type": "thinking", "thinking": "..." },
    { "type": "text", "text": "..." },
    { "type": "tool_use", "id": "...", "name": "...", "input": {} }
  ]
}
```

- `type: "thinking"` → create `pi-thinking` entry
- `type: "text"` → create `text` entry
- `type: "tool_use"` → skip (tool_execution events are the source of truth)

## UI Components

### PiThinkingEntry

New component for `pi-thinking` entries:

- Default state: collapsed, header shows "Thinking" label with a distinct icon/marker
- Expanded state: full thinking text rendered in monospace or markdown
- Visual style: muted/subtle color scheme to differentiate from regular assistant text
- Structure mirrors `ToolEntry` collapse pattern for consistency

### ToolEntry (reused)

No changes needed. `tool_execution_start` creates an entry with `inProgress=true` (spinner shown), `tool_execution_end` fills in the result. `isError=true` auto-expands the entry with red highlight — same behavior as Claude tool calls.

## Filter Logic

| Filter | Visible entry kinds |
|--------|---------------------|
| `all` | All |
| `text` | `text`, `pi-thinking` |
| `tool` | `tool` |
| `raw` | `raw` |
| Always visible | `result`, `codex-turn` |

`pi-thinking` is grouped under the `text` filter because it is part of the assistant's output.

## Testing Strategy

Unit tests in `pi-parser.test.ts` covering:

- `agent_start` / `agent_end` events are ignored
- `turn_start` / `turn_end` events are ignored
- `message_end` with thinking block → creates `pi-thinking` entry
- `message_end` with text block → creates `text` entry
- `message_end` with tool_use block → ignored (no duplicate entry)
- `tool_execution_start` → creates in-progress `tool` entry, stored in itemMap
- `tool_execution_end` → updates matching tool entry with result and isError
- `tool_execution_end` for unknown toolCallId → silently ignored
- `auto_retry_start` → raw entry with retry message
- `auto_retry_end` success → ignored
- `auto_retry_end` failure → raw entry with error
- `extension_error` → raw entry fallback
- Unknown event type → raw entry fallback
- Malformed JSON → raw entry fallback

Update `detect-engine.test.ts`:
- `agent_start` type → returns `"pi"`
- Non-pi/codex/claude type still defaults to `"claude"`
