# Message Content Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract actual human-readable content from trigger_input messages in two formats (old frontmatter, new XML tags) and display it consistently across session-detail and worker-detail pages.

**Architecture:** Add a single `extractMessageContent` utility to `format.ts`, remove the local `stripMetadataPrefix` from `session-detail.tsx`, and apply the new function to all three display locations across both pages.

**Tech Stack:** TypeScript, React, Vitest

---

## File Map

| Action | File | Change |
|--------|------|--------|
| Modify | `web/src/lib/format.ts` | Add `extractMessageContent` function |
| Create | `web/src/lib/__tests__/format.test.ts` | Unit tests for `extractMessageContent` |
| Modify | `web/src/pages/session-detail.tsx` | Remove local function, import + use `extractMessageContent` in sidebar (line 256) and detail view (line 316) |
| Modify | `web/src/pages/worker-detail.tsx` | Import + use `extractMessageContent` in session list (line 305) |

---

### Task 1: Add `extractMessageContent` to format.ts with tests (TDD)

**Files:**
- Create: `web/src/lib/__tests__/format.test.ts`
- Modify: `web/src/lib/format.ts`

- [ ] **Step 1: Create the test file with failing tests**

Create `web/src/lib/__tests__/format.test.ts`:

```typescript
import { describe, expect, it } from "vitest"
import { extractMessageContent } from "../format"

describe("extractMessageContent", () => {
  it("returns input unchanged when no known format is detected", () => {
    expect(extractMessageContent("hello world")).toBe("hello world")
  })

  it("returns empty string unchanged", () => {
    expect(extractMessageContent("")).toBe("")
  })

  it("strips old frontmatter format", () => {
    const input = "---\nmessage_id: abc123\n---\n\nThis is the content"
    expect(extractMessageContent(input)).toBe("This is the content")
  })

  it("strips old frontmatter format without trailing blank line", () => {
    const input = "---\nmessage_id: abc123\n---\nThis is the content"
    expect(extractMessageContent(input)).toBe("This is the content")
  })

  it("extracts content from new format with message_content tag", () => {
    const input = `<message_meta>{"from":"feishu","message_id":"abc"}</message_meta>\n<message_content>\nhello world\n</message_content>`
    expect(extractMessageContent(input)).toBe("hello world")
  })

  it("extracts content from new format with task_content tag", () => {
    const input = `<message_meta>{"from":"feishu","message_id":"abc"}</message_meta>\n<task_content>\ndo something\n</task_content>`
    expect(extractMessageContent(input)).toBe("do something")
  })

  it("preserves nested content inside task_content as-is", () => {
    const input = `<message_meta>{}</message_meta>\n<task_content>\n<worker_persona>X</worker_persona>\nsome task\n</task_content>`
    expect(extractMessageContent(input)).toBe("<worker_persona>X</worker_persona>\nsome task")
  })
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/web
npx vitest run src/lib/__tests__/format.test.ts
```

Expected: FAIL — `extractMessageContent` is not exported from format.ts

- [ ] **Step 3: Add `extractMessageContent` to `web/src/lib/format.ts`**

Append to the end of `web/src/lib/format.ts`:

```typescript
export function extractMessageContent(input: string): string {
  if (!input) return input

  // New format: extract <message_content> or <task_content> inner text
  const newFormatMatch =
    input.match(/<message_content>\n?([\s\S]*?)\n?<\/message_content>/) ||
    input.match(/<task_content>\n?([\s\S]*?)\n?<\/task_content>/)
  if (newFormatMatch) return newFormatMatch[1].trim()

  // Old format: strip frontmatter ---...---
  const oldFormatMatch = input.match(/^---\n[\s\S]*?\n---\n\n?/)
  if (oldFormatMatch) return input.slice(oldFormatMatch[0].length)

  return input
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/web
npx vitest run src/lib/__tests__/format.test.ts
```

Expected: All 7 tests PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/format.ts web/src/lib/__tests__/format.test.ts
git commit -m "feat: add extractMessageContent utility for parsing message formats"
```

---

### Task 2: Update session-detail.tsx to use `extractMessageContent`

**Files:**
- Modify: `web/src/pages/session-detail.tsx`

- [ ] **Step 1: Update import line (line 15) to include `extractMessageContent`**

In `web/src/pages/session-detail.tsx`, replace line 15:

```typescript
import { formatTimestamp, formatCompactTimestamp, formatDuration, statusTone, isActiveStatus } from "@/lib/format"
```

with:

```typescript
import { formatTimestamp, formatCompactTimestamp, formatDuration, statusTone, isActiveStatus, extractMessageContent } from "@/lib/format"
```

- [ ] **Step 2: Delete the local `stripMetadataPrefix` function (lines 17–20)**

Remove these lines entirely:

```typescript
function stripMetadataPrefix(input: string): string {
  const match = input.match(/^---\n[\s\S]*?\n---\n\n?/)
  return match ? input.slice(match[0].length) : input
}
```

- [ ] **Step 3: Update sidebar usage (was line 256, now line 252 after deletion)**

Replace:
```tsx
{stripMetadataPrefix(exec.trigger_input) || t("sessionDetail.noTriggerInput")}
```

with:
```tsx
{extractMessageContent(exec.trigger_input) || t("sessionDetail.noTriggerInput")}
```

- [ ] **Step 4: Update detail view usage (was line 316, now ~line 312 after deletion)**

Replace:
```tsx
<pre className="whitespace-pre-wrap break-words text-sm leading-6 text-foreground">
  {selectedExecution.trigger_input}
</pre>
```

with:
```tsx
<pre className="whitespace-pre-wrap break-words text-sm leading-6 text-foreground">
  {extractMessageContent(selectedExecution.trigger_input)}
</pre>
```

- [ ] **Step 5: Verify the app builds without errors**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/web
npx tsc --noEmit
```

Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/session-detail.tsx
git commit -m "feat: use extractMessageContent in session-detail page"
```

---

### Task 3: Update worker-detail.tsx to use `extractMessageContent`

**Files:**
- Modify: `web/src/pages/worker-detail.tsx`

- [ ] **Step 1: Add `extractMessageContent` to the format import**

In `web/src/pages/worker-detail.tsx`, find the existing format import line (it imports from `@/lib/format`) and add `extractMessageContent` to it.

Find the line that looks like:
```typescript
import { ... } from "@/lib/format"
```

Add `extractMessageContent` to the named imports. For example if the line currently is:
```typescript
import { formatTimestamp, formatDuration, groupExecutionsBySession, isActiveStatus, statusTone, STATUS_ROW_BORDER } from "@/lib/format"
```

Change it to:
```typescript
import { formatTimestamp, formatDuration, groupExecutionsBySession, isActiveStatus, statusTone, STATUS_ROW_BORDER, extractMessageContent } from "@/lib/format"
```

- [ ] **Step 2: Update session list trigger_input display (line ~305)**

Replace:
```tsx
{oldest.trigger_input ? (
  <span>{oldest.trigger_input}</span>
) : null}
```

with:
```tsx
{oldest.trigger_input ? (
  <span>{extractMessageContent(oldest.trigger_input)}</span>
) : null}
```

- [ ] **Step 3: Verify the app builds without errors**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/web
npx tsc --noEmit
```

Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/worker-detail.tsx
git commit -m "feat: use extractMessageContent in worker-detail session list"
```
