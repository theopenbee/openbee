# Pi Agent Log Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add pi agent log rendering to the session detail log viewer, supporting text, thinking blocks, and tool calls with real-time tool execution state.

**Architecture:** Add a `PiParser` implementing the existing `StreamParser` interface; detect pi logs via the `agent_start` first-line event; render thinking blocks with a new `PiThinkingEntry` component in `log-viewer.tsx`. Tool calls reuse the existing `ToolEntry` component via the existing `tool` entry kind.

**Tech Stack:** TypeScript, React, Vitest, i18next, Tailwind CSS

---

## File Map

| File | Change |
|------|--------|
| `web/src/components/log-viewer/types.ts` | Add `pi-thinking` ParsedEntry kind |
| `web/src/components/log-viewer/detect-engine.ts` | Add `"pi"` return branch for `agent_start` |
| `web/src/components/log-viewer/__tests__/detect-engine.test.ts` | Add pi detection test cases |
| `web/src/components/log-viewer/pi-parser.ts` | New file: PiParser class |
| `web/src/components/log-viewer/__tests__/pi-parser.test.ts` | New file: PiParser unit tests |
| `web/src/components/log-viewer.tsx` | Import PiParser, add PiThinkingEntry component, update ensureParser, filter logic, render switch |
| `web/src/locales/en.json` | Add `logViewer.thinking` key |
| `web/src/locales/zh.json` | Add `logViewer.thinking` key |

---

### Task 1: Add `pi-thinking` ParsedEntry kind

**Files:**
- Modify: `web/src/components/log-viewer/types.ts`

- [ ] **Step 1: Add the new kind to the union**

In `types.ts`, append the `pi-thinking` variant to the `ParsedEntry` union after the `codex-turn` line:

```typescript
export type ParsedEntry =
  | { kind: "text"; text: string }
  | {
      kind: "tool"
      id: string
      name: string
      input: unknown
      result?: string
      isError?: boolean
    }
  | { kind: "result"; text: string; subtype: string }
  | { kind: "raw"; content: string; logType: string; lineCount: number }
  | {
      kind: "codex-command"
      id: string
      command: string
      output?: string
      inProgress: boolean
    }
  | {
      kind: "codex-turn"
      inputTokens: number
      cachedInputTokens: number
      outputTokens: number
    }
  | { kind: "pi-thinking"; id: string; thinking: string }
```

All other exports in the file remain unchanged.

- [ ] **Step 2: Commit**

```bash
git add web/src/components/log-viewer/types.ts
git commit -m "feat: add pi-thinking ParsedEntry kind"
```

---

### Task 2: Extend engine detection to support pi

**Files:**
- Modify: `web/src/components/log-viewer/detect-engine.ts`
- Modify: `web/src/components/log-viewer/__tests__/detect-engine.test.ts`

- [ ] **Step 1: Write failing tests**

Replace the contents of `detect-engine.test.ts` with:

```typescript
import { describe, expect, it } from "vitest"
import { detectEngine } from "../detect-engine"

describe("detectEngine", () => {
  it("returns 'codex' when first line is thread.started", () => {
    const line = JSON.stringify({ type: "thread.started", thread_id: "abc-123" })
    expect(detectEngine(line)).toBe("codex")
  })

  it("returns 'pi' when first line is agent_start", () => {
    const line = JSON.stringify({ type: "agent_start" })
    expect(detectEngine(line)).toBe("pi")
  })

  it("returns 'claude' for a Claude assistant event", () => {
    const line = JSON.stringify({ type: "assistant", message: { content: [] } })
    expect(detectEngine(line)).toBe("claude")
  })

  it("returns 'claude' for any non-Codex non-pi type", () => {
    const line = JSON.stringify({ type: "system" })
    expect(detectEngine(line)).toBe("claude")
  })

  it("returns 'claude' when JSON is malformed", () => {
    expect(detectEngine("not json at all")).toBe("claude")
  })

  it("returns 'claude' for an empty string", () => {
    expect(detectEngine("")).toBe("claude")
  })
})
```

- [ ] **Step 2: Run test to verify pi test fails**

```bash
cd web && pnpm test run src/components/log-viewer/__tests__/detect-engine.test.ts
```

Expected: FAIL — `"pi"` test fails because current implementation returns `"claude"`.

- [ ] **Step 3: Update detect-engine.ts**

Replace the contents of `detect-engine.ts` with:

```typescript
import { parseJsonEvent } from "./types"

export function detectEngine(firstLine: string): "claude" | "codex" | "pi" {
  const event = parseJsonEvent<{ type: string }>(firstLine)
  if (event?.type === "thread.started") return "codex"
  if (event?.type === "agent_start") return "pi"
  return "claude"
}
```

- [ ] **Step 4: Run tests to verify all pass**

```bash
cd web && pnpm test run src/components/log-viewer/__tests__/detect-engine.test.ts
```

Expected: PASS — all 6 tests green.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/log-viewer/detect-engine.ts web/src/components/log-viewer/__tests__/detect-engine.test.ts
git commit -m "feat: detect pi engine via agent_start event"
```

---

### Task 3: Implement PiParser with tests (TDD)

**Files:**
- Create: `web/src/components/log-viewer/pi-parser.ts`
- Create: `web/src/components/log-viewer/__tests__/pi-parser.test.ts`

- [ ] **Step 1: Write the full test file**

Create `web/src/components/log-viewer/__tests__/pi-parser.test.ts`:

```typescript
import { describe, expect, it } from "vitest"
import { PiParser } from "../pi-parser"
import type { ParsedEntry } from "../types"

function run(lines: string[], logType = "stdout"): ParsedEntry[] {
  const parser = new PiParser()
  const entries: ParsedEntry[] = []
  const itemMap = new Map<string, number>()
  for (const line of lines) parser.parseLine(line, logType, entries, itemMap)
  return entries
}

describe("PiParser", () => {
  // --- ignored events ---

  it("ignores agent_start", () => {
    expect(run([JSON.stringify({ type: "agent_start" })])).toHaveLength(0)
  })

  it("ignores agent_end", () => {
    expect(run([JSON.stringify({ type: "agent_end", messages: [] })])).toHaveLength(0)
  })

  it("ignores turn_start", () => {
    expect(run([JSON.stringify({ type: "turn_start" })])).toHaveLength(0)
  })

  it("ignores turn_end", () => {
    expect(run([JSON.stringify({ type: "turn_end", message: {}, toolResults: [] })])).toHaveLength(0)
  })

  it("ignores message_start", () => {
    expect(run([JSON.stringify({ type: "message_start", message: {} })])).toHaveLength(0)
  })

  it("ignores message_update", () => {
    expect(run([JSON.stringify({ type: "message_update", message: {}, assistantMessageEvent: { type: "text_delta", delta: "hi" } })])).toHaveLength(0)
  })

  it("ignores queue_update", () => {
    expect(run([JSON.stringify({ type: "queue_update", steering: [], followUp: [] })])).toHaveLength(0)
  })

  it("ignores compaction_start", () => {
    expect(run([JSON.stringify({ type: "compaction_start", reason: "threshold" })])).toHaveLength(0)
  })

  it("ignores compaction_end", () => {
    expect(run([JSON.stringify({ type: "compaction_end", reason: "threshold", aborted: false, willRetry: false })])).toHaveLength(0)
  })

  // --- message_end ---

  it("extracts text block from message_end as text entry", () => {
    const line = JSON.stringify({
      type: "message_end",
      message: {
        role: "assistant",
        content: [{ type: "text", text: "Hello world" }],
      },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "text", text: "Hello world" })
  })

  it("extracts thinking block from message_end as pi-thinking entry", () => {
    const line = JSON.stringify({
      type: "message_end",
      message: {
        role: "assistant",
        content: [{ type: "thinking", thinking: "let me reason..." }],
      },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "pi-thinking", thinking: "let me reason..." })
    expect((entries[0] as Extract<typeof entries[0], { kind: "pi-thinking" }>).id).toBe("thinking-0-0")
  })

  it("extracts both thinking and text blocks from the same message_end in order", () => {
    const line = JSON.stringify({
      type: "message_end",
      message: {
        role: "assistant",
        content: [
          { type: "thinking", thinking: "step by step..." },
          { type: "text", text: "Done." },
        ],
      },
    })
    const entries = run([line])
    expect(entries).toHaveLength(2)
    expect(entries[0]).toMatchObject({ kind: "pi-thinking", thinking: "step by step..." })
    expect(entries[1]).toMatchObject({ kind: "text", text: "Done." })
  })

  it("ignores tool_use blocks in message_end (handled by tool_execution events)", () => {
    const line = JSON.stringify({
      type: "message_end",
      message: {
        role: "assistant",
        content: [
          { type: "tool_use", id: "call_1", name: "bash", input: { command: "ls" } },
        ],
      },
    })
    expect(run([line])).toHaveLength(0)
  })

  it("assigns sequential thinking IDs across multiple message_end events", () => {
    const msg0 = JSON.stringify({
      type: "message_end",
      message: { role: "assistant", content: [{ type: "thinking", thinking: "first" }] },
    })
    const msg1 = JSON.stringify({
      type: "message_end",
      message: { role: "assistant", content: [{ type: "thinking", thinking: "second" }] },
    })
    const entries = run([msg0, msg1])
    expect(entries).toHaveLength(2)
    const ids = entries.map((e) => (e as Extract<typeof e, { kind: "pi-thinking" }>).id)
    expect(ids[0]).toBe("thinking-0-0")
    expect(ids[1]).toBe("thinking-1-0")
  })

  it("skips message_end when message has no content array", () => {
    const line = JSON.stringify({ type: "message_end", message: { role: "assistant" } })
    expect(run([line])).toHaveLength(0)
  })

  // --- tool_execution_start ---

  it("creates in-progress tool entry for tool_execution_start", () => {
    const line = JSON.stringify({
      type: "tool_execution_start",
      toolCallId: "call_abc",
      toolName: "bash",
      args: { command: "ls -la" },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      kind: "tool",
      id: "call_abc",
      name: "bash",
      input: { command: "ls -la" },
      result: undefined,
    })
  })

  // --- tool_execution_end ---

  it("updates tool entry with result on tool_execution_end", () => {
    const startLine = JSON.stringify({
      type: "tool_execution_start",
      toolCallId: "call_abc",
      toolName: "bash",
      args: { command: "ls" },
    })
    const endLine = JSON.stringify({
      type: "tool_execution_end",
      toolCallId: "call_abc",
      toolName: "bash",
      result: { content: [{ type: "text", text: "file1\nfile2" }] },
      isError: false,
    })
    const entries = run([startLine, endLine])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      kind: "tool",
      id: "call_abc",
      result: "file1\nfile2",
      isError: false,
    })
  })

  it("marks tool entry as error when isError is true", () => {
    const startLine = JSON.stringify({
      type: "tool_execution_start",
      toolCallId: "call_err",
      toolName: "bash",
      args: { command: "bad" },
    })
    const endLine = JSON.stringify({
      type: "tool_execution_end",
      toolCallId: "call_err",
      toolName: "bash",
      result: { content: [{ type: "text", text: "error output" }] },
      isError: true,
    })
    const entries = run([startLine, endLine])
    expect(entries[0]).toMatchObject({ kind: "tool", isError: true, result: "error output" })
  })

  it("silently ignores tool_execution_end for unknown toolCallId", () => {
    const line = JSON.stringify({
      type: "tool_execution_end",
      toolCallId: "call_unknown",
      toolName: "bash",
      result: { content: [{ type: "text", text: "out" }] },
      isError: false,
    })
    expect(run([line])).toHaveLength(0)
  })

  it("falls back to empty string result when content array is empty", () => {
    const startLine = JSON.stringify({
      type: "tool_execution_start",
      toolCallId: "call_empty",
      toolName: "bash",
      args: {},
    })
    const endLine = JSON.stringify({
      type: "tool_execution_end",
      toolCallId: "call_empty",
      toolName: "bash",
      result: { content: [] },
      isError: false,
    })
    const entries = run([startLine, endLine])
    expect(entries[0]).toMatchObject({ kind: "tool", result: "" })
  })

  // --- auto_retry ---

  it("creates raw entry for auto_retry_start", () => {
    const line = JSON.stringify({
      type: "auto_retry_start",
      attempt: 1,
      maxAttempts: 3,
      delayMs: 2000,
      errorMessage: "overloaded",
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw" })
    expect((entries[0] as Extract<typeof entries[0], { kind: "raw" }>).content).toContain("Retrying")
    expect((entries[0] as Extract<typeof entries[0], { kind: "raw" }>).content).toContain("1/3")
  })

  it("ignores auto_retry_end when success is true", () => {
    const line = JSON.stringify({ type: "auto_retry_end", success: true, attempt: 2 })
    expect(run([line])).toHaveLength(0)
  })

  it("creates raw entry for auto_retry_end when success is false", () => {
    const line = JSON.stringify({
      type: "auto_retry_end",
      success: false,
      attempt: 3,
      finalError: "overloaded_error: Overloaded",
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw" })
    expect((entries[0] as Extract<typeof entries[0], { kind: "raw" }>).content).toContain("overloaded_error")
  })

  // --- extension_error ---

  it("creates raw entry for extension_error", () => {
    const line = JSON.stringify({
      type: "extension_error",
      extensionPath: "/path/ext.ts",
      event: "tool_call",
      error: "boom",
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw" })
  })

  // --- fallbacks ---

  it("creates raw entry for unknown event type", () => {
    const line = JSON.stringify({ type: "unknown.event" })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stdout" })
  })

  it("creates raw entry for malformed JSON", () => {
    const entries = run(["not valid json"])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", content: "not valid json" })
  })

  it("passes non-stdout lines through as raw", () => {
    const entries = run([JSON.stringify({ type: "agent_start" })], "stderr")
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stderr" })
  })
})
```

- [ ] **Step 2: Run tests to verify they all fail**

```bash
cd web && pnpm test run src/components/log-viewer/__tests__/pi-parser.test.ts
```

Expected: FAIL — module `../pi-parser` not found.

- [ ] **Step 3: Create pi-parser.ts**

Create `web/src/components/log-viewer/pi-parser.ts`:

```typescript
import type { ParsedEntry, StreamParser } from "./types"
import { appendRawEntry, parseJsonEvent } from "./types"

interface PiContentBlock {
  type: string
  text?: string
  thinking?: string
  id?: string
  name?: string
  input?: unknown
}

interface PiMessage {
  role?: string
  content?: PiContentBlock[]
}

interface PiToolResult {
  content?: Array<{ type: string; text?: string }>
}

interface PiEvent {
  type: string
  message?: PiMessage
  toolCallId?: string
  toolName?: string
  args?: unknown
  result?: PiToolResult
  isError?: boolean
  attempt?: number
  maxAttempts?: number
  finalError?: string
  success?: boolean
}

export class PiParser implements StreamParser {
  private messageCount = 0

  parseLine(
    line: string,
    logType: string,
    entries: ParsedEntry[],
    itemMap: Map<string, number>
  ): void {
    if (logType !== "stdout") {
      appendRawEntry(line, logType, entries)
      return
    }

    const event = parseJsonEvent<PiEvent>(line)

    if (!event) {
      appendRawEntry(line, logType, entries)
      return
    }

    switch (event.type) {
      case "agent_start":
      case "agent_end":
      case "turn_start":
      case "turn_end":
      case "message_start":
      case "message_update":
      case "queue_update":
      case "compaction_start":
      case "compaction_end":
        return

      case "message_end": {
        const messageIndex = this.messageCount++
        const content = event.message?.content
        if (!Array.isArray(content)) return
        content.forEach((block, blockIndex) => {
          if (block.type === "thinking" && block.thinking) {
            entries.push({
              kind: "pi-thinking",
              id: `thinking-${messageIndex}-${blockIndex}`,
              thinking: block.thinking,
            })
          } else if (block.type === "text" && block.text) {
            entries.push({ kind: "text", text: block.text })
          }
          // tool_use blocks are ignored — handled by tool_execution events
        })
        return
      }

      case "tool_execution_start": {
        const { toolCallId, toolName, args } = event
        if (!toolCallId || !toolName) {
          appendRawEntry(line, logType, entries)
          return
        }
        itemMap.set(toolCallId, entries.length)
        entries.push({ kind: "tool", id: toolCallId, name: toolName, input: args ?? {} })
        return
      }

      case "tool_execution_end": {
        const { toolCallId, result, isError } = event
        if (!toolCallId) {
          appendRawEntry(line, logType, entries)
          return
        }
        const idx = itemMap.get(toolCallId)
        if (idx === undefined) return
        const existing = entries[idx]
        if (existing?.kind !== "tool") return
        const textBlock = result?.content?.find((b) => b.type === "text")
        entries[idx] = {
          ...existing,
          result: textBlock?.text ?? "",
          isError: Boolean(isError),
        }
        itemMap.delete(toolCallId)
        return
      }

      case "auto_retry_start": {
        const attempt = event.attempt ?? "?"
        const max = event.maxAttempts ?? "?"
        appendRawEntry(`Retrying (attempt ${attempt}/${max})...`, logType, entries)
        return
      }

      case "auto_retry_end": {
        if (!event.success && event.finalError) {
          appendRawEntry(`Retry failed: ${event.finalError}`, logType, entries)
        }
        return
      }

      default:
        appendRawEntry(line, logType, entries)
    }
  }
}
```

- [ ] **Step 4: Run tests to verify all pass**

```bash
cd web && pnpm test run src/components/log-viewer/__tests__/pi-parser.test.ts
```

Expected: PASS — all tests green.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/log-viewer/pi-parser.ts web/src/components/log-viewer/__tests__/pi-parser.test.ts
git commit -m "feat: add PiParser for pi agent log rendering"
```

---

### Task 4: Add i18n key for thinking label

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add key to en.json**

In `web/src/locales/en.json`, inside the `"logViewer"` object, add after `"outputTokens": "output"`:

```json
"thinking": "Thinking"
```

So the `logViewer` section ends:
```json
    "outputTokens": "output",
    "thinking": "Thinking"
```

- [ ] **Step 2: Add key to zh.json**

In `web/src/locales/zh.json`, inside the `"logViewer"` object, add after `"outputTokens": "输出"`:

```json
"thinking": "思考过程"
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat: add logViewer.thinking i18n key"
```

---

### Task 5: Wire PiParser and add PiThinkingEntry to log-viewer.tsx

**Files:**
- Modify: `web/src/components/log-viewer.tsx`

- [ ] **Step 1: Add PiParser import**

At line 13, after the `CodexParser` import line:

```typescript
import { CodexParser } from "./log-viewer/codex-parser"
```

Add:

```typescript
import { PiParser } from "./log-viewer/pi-parser"
```

- [ ] **Step 2: Add PiThinkingEntry component**

After the `CodexTurnEntry` component (which ends at line 297) and before `RawEntry` (line 299), insert the new component:

```typescript
function PiThinkingEntry({
  entry,
}: {
  entry: Extract<ParsedEntry, { kind: "pi-thinking" }>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <TimelineRow markerClassName="bg-muted-foreground/35">
      <article className="overflow-hidden rounded-2xl border border-border/50 bg-muted/15">
        <button
          type="button"
          aria-expanded={open}
          aria-label={open ? t("logViewer.collapse", { name: t("logViewer.thinking") }) : t("logViewer.expand", { name: t("logViewer.thinking") })}
          onClick={() => setOpen((c) => !c)}
          className="flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/25"
        >
          <div className="min-w-0 flex-1">
            <p className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground/70">
              {t("logViewer.thinking")}
            </p>
          </div>
          <span className="mt-0.5 shrink-0 text-muted-foreground/60" aria-hidden="true">
            {open ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
          </span>
        </button>

        {open && (
          <div className="border-t border-border/50 px-4 pb-4 pt-3">
            <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-xl bg-muted/25 p-3 font-mono text-[12px] leading-6 text-muted-foreground">
              {entry.thinking}
            </pre>
          </div>
        )}
      </article>
    </TimelineRow>
  )
}
```

- [ ] **Step 3: Update ensureParser to handle pi engine**

Find this block in `log-viewer.tsx` (around line 364–368):

```typescript
    const ensureParser = (firstLine: string): StreamParser => {
      if (!parserRef.current) {
        parserRef.current =
          detectEngine(firstLine) === "codex" ? new CodexParser() : new ClaudeParser()
      }
      return parserRef.current
    }
```

Replace it with:

```typescript
    const ensureParser = (firstLine: string): StreamParser => {
      if (!parserRef.current) {
        const engine = detectEngine(firstLine)
        parserRef.current =
          engine === "codex"
            ? new CodexParser()
            : engine === "pi"
              ? new PiParser()
              : new ClaudeParser()
      }
      return parserRef.current
    }
```

- [ ] **Step 4: Update filter logic to count pi-thinking as narrative**

Find this block (around line 483–486):

```typescript
      if (entry.kind === "text") narrativeCount += 1
      else if (entry.kind === "tool" || entry.kind === "codex-command") toolCount += 1
      else if (entry.kind === "raw") rawCount += 1
```

Replace with:

```typescript
      if (entry.kind === "text" || entry.kind === "pi-thinking") narrativeCount += 1
      else if (entry.kind === "tool" || entry.kind === "codex-command") toolCount += 1
      else if (entry.kind === "raw") rawCount += 1
```

- [ ] **Step 5: Update visibility logic to show pi-thinking in text filter**

Find this block (around line 487–494):

```typescript
      const visible =
        entry.kind === "result" || entry.kind === "codex-turn"
          ? true
          : filter === "all"
            ? true
            : entry.kind === "codex-command"
              ? filter === "tool"
              : entry.kind === filter
```

Replace with:

```typescript
      const visible =
        entry.kind === "result" || entry.kind === "codex-turn"
          ? true
          : filter === "all"
            ? true
            : entry.kind === "codex-command"
              ? filter === "tool"
              : entry.kind === "pi-thinking"
                ? filter === "text"
                : entry.kind === filter
```

- [ ] **Step 6: Add pi-thinking to the render switch**

Find this line (around line 594):

```typescript
                if (entry.kind === "text") return <AssistantEntry key={`text-${k}`} text={entry.text} />
```

Add the pi-thinking case immediately before it:

```typescript
                if (entry.kind === "pi-thinking") return <PiThinkingEntry key={entry.id} entry={entry} />
                if (entry.kind === "text") return <AssistantEntry key={`text-${k}`} text={entry.text} />
```

- [ ] **Step 7: Build to verify no TypeScript errors**

```bash
cd web && pnpm build 2>&1 | tail -20
```

Expected: build completes with no type errors.

- [ ] **Step 8: Run all log-viewer tests**

```bash
cd web && pnpm test run src/components/log-viewer/
```

Expected: all tests pass.

- [ ] **Step 9: Commit**

```bash
git add web/src/components/log-viewer.tsx
git commit -m "feat: wire PiParser and add PiThinkingEntry component to log viewer"
```

---

### Task 6: Manual smoke test

**No files modified — verification only.**

- [ ] **Step 1: Start dev server**

```bash
cd web && pnpm dev
```

- [ ] **Step 2: Open the target session**

Navigate to: `http://localhost:5173/sessions/detail?session_id=0a9f644c-7ee7-4c0b-8c6b-9103e34e198c`

- [ ] **Step 3: Verify pi agent logs render correctly**

Check that:
- Text entries from `message_end` appear as narrative blocks
- Thinking blocks appear as collapsed "Thinking" entries that expand on click
- Tool calls show "Running..." while in-progress and fill in output when done
- Filter tabs (Narrative / Tool calls / Raw) work correctly
- No console errors

- [ ] **Step 4: Run full test suite**

```bash
cd web && pnpm test run
```

Expected: all tests pass.
