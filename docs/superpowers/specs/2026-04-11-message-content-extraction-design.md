# Message Content Extraction Design

**Date:** 2026-04-11  
**Branch:** feat/engine-plugin  
**Status:** Approved

## Problem

Two pages display session message content (`trigger_input`), but currently show raw content without stripping metadata wrappers:

1. `/sessions/detail` — sidebar (partially handled) and detail view (unhandled)
2. `/workers/[id]` — session list (unhandled)

Messages come in two formats that need to be parsed to extract the actual human-readable content.

## Message Formats

### Old Format (Frontmatter)
```
---
message_id: xxx
---

{actual content}
```

### New Format (XML Tags)
```
<message_meta>{"from":"feishu","session_key":"...","message_id":"..."}</message_meta>
<message_content>
actual content
</message_content>
```

Or with `<task_content>` instead of `<message_content>` — the two never coexist in the same message.

## Design

### 1. New Utility Function in `web/src/lib/format.ts`

Add `extractMessageContent(input: string): string`:

- **New format:** Match `<message_content>...</message_content>` or `<task_content>...</task_content>` and return the trimmed inner text as-is (preserving any nested tags/content)
- **Old format:** Match `^---\n[\s\S]*?\n---\n\n?` frontmatter block and return the content after it
- **Fallback:** Return the original string unchanged

```typescript
export function extractMessageContent(input: string): string {
  if (!input) return input;

  // New format: extract <message_content> or <task_content>
  const newFormatMatch =
    input.match(/<message_content>\n?([\s\S]*?)\n?<\/message_content>/) ||
    input.match(/<task_content>\n?([\s\S]*?)\n?<\/task_content>/);
  if (newFormatMatch) return newFormatMatch[1].trim();

  // Old format: strip frontmatter ---...---
  const oldFormatMatch = input.match(/^---\n[\s\S]*?\n---\n\n?/);
  if (oldFormatMatch) return input.slice(oldFormatMatch[0].length);

  return input;
}
```

### 2. `web/src/pages/session-detail.tsx`

- Remove local `stripMetadataPrefix` function
- Import `extractMessageContent` from `../lib/format`
- Sidebar Turn list: replace `stripMetadataPrefix(exec.trigger_input)` with `extractMessageContent(exec.trigger_input)`
- Detail view: wrap `selectedExecution.trigger_input` in `extractMessageContent()` before rendering in `<pre>`

### 3. `web/src/pages/worker-detail.tsx`

- Import `extractMessageContent` from `../lib/format`
- Session list `trigger_input` display: wrap in `extractMessageContent()` before rendering

## Scope

- No changes to backend
- No changes to data types
- No changes to log viewer or other components
- All three display locations unified under one function

## Non-Goals

- Parsing nested XML inside `task_content` (display as-is per design decision)
- Supporting additional message formats beyond the two described
