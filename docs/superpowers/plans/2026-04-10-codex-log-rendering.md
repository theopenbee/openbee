# Codex Log Rendering Adaptation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adapt `LogViewer` to parse and render Codex NDJSON logs alongside the existing Claude Code format, using an abstract `StreamParser` interface for future engine extensibility.

**Architecture:** A new `log-viewer/` subdirectory holds four focused modules: shared types + helpers (`types.ts`), engine auto-detection (`detect-engine.ts`), and two parser implementations (`claude-parser.ts`, `codex-parser.ts`). `log-viewer.tsx` becomes an orchestrator that auto-detects the engine on first log line, creates the appropriate parser, and delegates all line parsing to it. Two new UI components (`CodexCommandEntry`, `CodexTurnEntry`) render Codex-specific entries.

**Tech Stack:** React 19, TypeScript, Vitest (unit tests), i18next (i18n), Tailwind CSS 4, `@base-ui/react`

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Create | `web/src/components/log-viewer/types.ts` | `ParsedEntry` union, `StreamParser` interface, `appendTextEntry`, `appendRawEntry` helpers |
| Create | `web/src/components/log-viewer/detect-engine.ts` | Auto-detect `"claude"` vs `"codex"` from first log line |
| Create | `web/src/components/log-viewer/claude-parser.ts` | Migrate Claude parsing logic from `log-viewer.tsx` |
| Create | `web/src/components/log-viewer/codex-parser.ts` | New Codex event parsing logic |
| Create | `web/src/components/log-viewer/__tests__/detect-engine.test.ts` | Unit tests for engine detection |
| Create | `web/src/components/log-viewer/__tests__/claude-parser.test.ts` | Smoke tests for Claude parser migration |
| Create | `web/src/components/log-viewer/__tests__/codex-parser.test.ts` | Unit tests for Codex parser |
| Modify | `web/src/components/log-viewer.tsx` | Wire parser abstraction; add `CodexCommandEntry`, `CodexTurnEntry`; update filter logic |
| Modify | `web/vite.config.ts` | Add Vitest configuration |
| Modify | `web/package.json` | Add `test` script and `vitest` devDependency |
| Modify | `web/src/locales/en.json` | New i18n keys for Codex entries |
| Modify | `web/src/locales/zh.json` | Chinese translations for new keys |

---

## Task 1: Set Up Vitest

**Files:**
- Modify: `web/package.json`
- Modify: `web/vite.config.ts`

- [ ] **Step 1: Install Vitest**

```bash
cd web && pnpm add -D vitest
```

Expected: `vitest` appears in `devDependencies` in `package.json`.

- [ ] **Step 2: Add test script to package.json**

In `web/package.json`, add `"test"` to `"scripts"`:
```json
"scripts": {
  "dev": "vite",
  "build": "tsc -b && vite build",
  "preview": "vite preview",
  "lint": "eslint .",
  "test": "vitest run"
}
```

- [ ] **Step 3: Configure Vitest in vite.config.ts**

Replace the entire `web/vite.config.ts` with:
```typescript
import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import path from "path"

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/mcp": "http://localhost:8080",
      "/internal": "http://localhost:8080",
    },
  },
  build: {
    target: "es2020",
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return;
          const pkgMatch = id.match(/.*\/node_modules\/((?:@[^/]+\/)?[^/]+)/);
          if (!pkgMatch) return;
          const pkg = pkgMatch[1];
          if (pkg === "react" || pkg === "react-dom") return "vendor-react";
          if (pkg === "react-router-dom" || pkg === "react-router") return "vendor-router";
          if (pkg === "@tanstack/react-query") return "vendor-query";
          if (pkg === "i18next" || pkg === "react-i18next") return "vendor-i18n";
          if (pkg === "@base-ui/react") return "vendor-ui";
          if (pkg === "lucide-react") return "vendor-icons";
        },
      },
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
})
```

- [ ] **Step 4: Verify Vitest works**

Create a temporary sanity-check test to confirm the setup works, then delete it:
```bash
echo 'import { test, expect } from "vitest"; test("sanity", () => expect(1+1).toBe(2))' > web/src/sanity.test.ts
cd web && pnpm test
```

Expected output: `1 passed`.

Then delete the file:
```bash
rm web/src/sanity.test.ts
```

- [ ] **Step 5: Commit**

```bash
cd web && git add package.json vite.config.ts && git commit -m "chore: add vitest for unit testing"
```

---

## Task 2: Create types.ts

**Files:**
- Create: `web/src/components/log-viewer/types.ts`

- [ ] **Step 1: Create the directory and types file**

```bash
mkdir -p web/src/components/log-viewer/__tests__
```

Create `web/src/components/log-viewer/types.ts`:
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
  | { kind: "raw"; content: string; logType: string }
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

export interface StreamParser {
  parseLine(
    line: string,
    logType: string,
    entries: ParsedEntry[],
    itemMap: Map<string, number>
  ): void
}

export function appendTextEntry(text: string, entries: ParsedEntry[]): void {
  const last = entries[entries.length - 1]
  if (last?.kind === "text") {
    last.text = `${last.text}\n\n${text}`
    return
  }
  entries.push({ kind: "text", text })
}

export function appendRawEntry(content: string, logType: string, entries: ParsedEntry[]): void {
  const last = entries[entries.length - 1]
  if (last?.kind === "raw" && last.logType === logType) {
    last.content = `${last.content}\n${content}`
    return
  }
  entries.push({ kind: "raw", content, logType })
}
```

- [ ] **Step 2: Commit**

```bash
cd web && git add src/components/log-viewer/types.ts && git commit -m "feat: add ParsedEntry types and StreamParser interface"
```

---

## Task 3: Create detect-engine.ts (TDD)

**Files:**
- Create: `web/src/components/log-viewer/detect-engine.ts`
- Create: `web/src/components/log-viewer/__tests__/detect-engine.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/components/log-viewer/__tests__/detect-engine.test.ts`:
```typescript
import { describe, expect, it } from "vitest"
import { detectEngine } from "../detect-engine"

describe("detectEngine", () => {
  it("returns 'codex' when first line is thread.started", () => {
    const line = JSON.stringify({ type: "thread.started", thread_id: "abc-123" })
    expect(detectEngine(line)).toBe("codex")
  })

  it("returns 'claude' for a Claude assistant event", () => {
    const line = JSON.stringify({ type: "assistant", message: { content: [] } })
    expect(detectEngine(line)).toBe("claude")
  })

  it("returns 'claude' for any non-Codex type", () => {
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

- [ ] **Step 2: Run test to verify it fails**

```bash
cd web && pnpm test -- --reporter=verbose
```

Expected: FAIL — `Cannot find module '../detect-engine'`

- [ ] **Step 3: Implement detect-engine.ts**

Create `web/src/components/log-viewer/detect-engine.ts`:
```typescript
export function detectEngine(firstLine: string): "claude" | "codex" {
  try {
    const obj = JSON.parse(firstLine)
    if (obj && obj.type === "thread.started") return "codex"
  } catch {
    // fall through
  }
  return "claude"
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd web && pnpm test -- --reporter=verbose
```

Expected: `5 passed` (all detect-engine tests)

- [ ] **Step 5: Commit**

```bash
cd web && git add src/components/log-viewer/detect-engine.ts src/components/log-viewer/__tests__/detect-engine.test.ts && git commit -m "feat: add engine auto-detection for log parser"
```

---

## Task 4: Create claude-parser.ts (migrate + smoke tests)

**Files:**
- Create: `web/src/components/log-viewer/claude-parser.ts`
- Create: `web/src/components/log-viewer/__tests__/claude-parser.test.ts`

- [ ] **Step 1: Write smoke tests**

Create `web/src/components/log-viewer/__tests__/claude-parser.test.ts`:
```typescript
import { describe, expect, it } from "vitest"
import { ClaudeParser } from "../claude-parser"
import type { ParsedEntry } from "../types"

function run(lines: string[]): ParsedEntry[] {
  const parser = new ClaudeParser()
  const entries: ParsedEntry[] = []
  const itemMap = new Map<string, number>()
  for (const line of lines) parser.parseLine(line, "stdout", entries, itemMap)
  return entries
}

describe("ClaudeParser", () => {
  it("parses assistant text block", () => {
    const line = JSON.stringify({
      type: "assistant",
      message: { content: [{ type: "text", text: "Hello world" }] },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "text", text: "Hello world" })
  })

  it("parses tool_use block", () => {
    const line = JSON.stringify({
      type: "assistant",
      message: {
        content: [{ type: "tool_use", id: "t1", name: "Bash", input: { command: "ls" } }],
      },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "tool", id: "t1", name: "Bash" })
  })

  it("fills tool result via user event", () => {
    const assistantLine = JSON.stringify({
      type: "assistant",
      message: { content: [{ type: "tool_use", id: "t1", name: "Bash", input: {} }] },
    })
    const userLine = JSON.stringify({
      type: "user",
      message: {
        content: [{ type: "tool_result", tool_use_id: "t1", content: "output text" }],
      },
    })
    const entries = run([assistantLine, userLine])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "tool", id: "t1", result: "output text" })
  })

  it("parses result event", () => {
    const line = JSON.stringify({ type: "result", result: "done", subtype: "success" })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "result", text: "done", subtype: "success" })
  })

  it("ignores system events", () => {
    const line = JSON.stringify({ type: "system", data: "init" })
    const entries = run([line])
    expect(entries).toHaveLength(0)
  })

  it("ignores rate_limit_event", () => {
    const line = JSON.stringify({ type: "rate_limit_event" })
    const entries = run([line])
    expect(entries).toHaveLength(0)
  })

  it("falls back to raw for unknown events", () => {
    const line = JSON.stringify({ type: "unknown_type" })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stdout" })
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd web && pnpm test -- --reporter=verbose
```

Expected: FAIL — `Cannot find module '../claude-parser'`

- [ ] **Step 3: Implement claude-parser.ts**

Create `web/src/components/log-viewer/claude-parser.ts`:
```typescript
import type { ParsedEntry, StreamParser } from "./types"
import { appendTextEntry, appendRawEntry } from "./types"

interface ClaudeStreamEvent {
  type: string
  subtype?: string
  message?: {
    content: Array<{
      type: string
      text?: string
      id?: string
      name?: string
      input?: unknown
      tool_use_id?: string
      content?: string | unknown
      is_error?: boolean
    }>
  }
  result?: string
}

function parseStreamLine(line: string): ClaudeStreamEvent | null {
  try {
    const obj = JSON.parse(line)
    if (obj && typeof obj.type === "string") return obj as ClaudeStreamEvent
    return null
  } catch {
    return null
  }
}

export function truncate(value: string, max = 96): string {
  return value.length > max ? `${value.slice(0, max)}…` : value
}

export function stringify(value: unknown): string {
  if (typeof value === "string") return value
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

export function getToolMeta(name: string): {
  label: string
  summary: (input: unknown) => string
} {
  switch (name) {
    case "Bash":
      return {
        label: "SH",
        summary: (input: unknown) =>
          truncate(
            (input as { command?: string; cmd?: string })?.command ??
              (input as { command?: string; cmd?: string })?.cmd ??
              stringify(input)
          ),
      }
    case "Read":
    case "Write":
    case "Edit":
    case "Glob":
    case "Grep":
      return {
        label: "FS",
        summary: (input: unknown) => {
          const record = input as Record<string, string>
          return truncate(
            record?.file_path ??
              record?.path ??
              record?.pattern ??
              record?.glob ??
              stringify(input)
          )
        },
      }
    case "WebSearch":
    case "WebFetch":
      return {
        label: "WEB",
        summary: (input: unknown) => {
          const record = input as Record<string, string>
          return truncate(record?.query ?? record?.url ?? stringify(input))
        },
      }
    default:
      return {
        label: "TOOL",
        summary: (input: unknown) => truncate(stringify(input)),
      }
  }
}

export function extractToolResultText(content: unknown): string {
  if (typeof content === "string") return content
  if (Array.isArray(content)) {
    const texts = content
      .filter((chunk): chunk is { type: string; text: string } => {
        return (
          typeof chunk === "object" &&
          chunk !== null &&
          "text" in chunk &&
          typeof (chunk as Record<string, unknown>).text === "string"
        )
      })
      .map((chunk) => chunk.text)
    if (texts.length > 0) return texts.join("\n")
  }
  return stringify(content)
}

export class ClaudeParser implements StreamParser {
  parseLine(
    line: string,
    logType: string,
    entries: ParsedEntry[],
    itemMap: Map<string, number>
  ): void {
    if (logType === "stdout") {
      const event = parseStreamLine(line)
      if (event) {
        if (event.type === "assistant" && event.message?.content) {
          for (const block of event.message.content) {
            if (block.type === "text" && block.text) {
              appendTextEntry(block.text, entries)
            } else if (block.type === "tool_use" && block.id && block.name) {
              itemMap.set(block.id, entries.length)
              entries.push({ kind: "tool", id: block.id, name: block.name, input: block.input })
            }
          }
          return
        }

        if (event.type === "user" && event.message?.content) {
          for (const block of event.message.content) {
            if (block.type === "tool_result" && block.tool_use_id) {
              const idx = itemMap.get(block.tool_use_id)
              if (idx === undefined) continue
              const existing = entries[idx]
              if (existing?.kind === "tool") {
                entries[idx] = {
                  ...existing,
                  result: extractToolResultText(block.content),
                  isError: block.is_error,
                }
              }
            }
          }
          return
        }

        if (event.type === "result") {
          entries.push({ kind: "result", text: event.result ?? "", subtype: event.subtype ?? "" })
          return
        }

        if (event.type === "system" || event.type === "rate_limit_event") {
          return
        }
      }
    }

    appendRawEntry(line, logType, entries)
  }
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd web && pnpm test -- --reporter=verbose
```

Expected: all 7 claude-parser tests pass (plus all prior detect-engine tests)

- [ ] **Step 5: Commit**

```bash
cd web && git add src/components/log-viewer/claude-parser.ts src/components/log-viewer/__tests__/claude-parser.test.ts && git commit -m "feat: add ClaudeParser (migrated from log-viewer.tsx)"
```

---

## Task 5: Create codex-parser.ts (TDD)

**Files:**
- Create: `web/src/components/log-viewer/codex-parser.ts`
- Create: `web/src/components/log-viewer/__tests__/codex-parser.test.ts`

- [ ] **Step 1: Write failing tests**

Create `web/src/components/log-viewer/__tests__/codex-parser.test.ts`:
```typescript
import { describe, expect, it } from "vitest"
import { CodexParser } from "../codex-parser"
import type { ParsedEntry } from "../types"

function run(lines: string[]): ParsedEntry[] {
  const parser = new CodexParser()
  const entries: ParsedEntry[] = []
  const itemMap = new Map<string, number>()
  for (const line of lines) parser.parseLine(line, "stdout", entries, itemMap)
  return entries
}

describe("CodexParser", () => {
  it("ignores thread.started", () => {
    const line = JSON.stringify({ type: "thread.started", thread_id: "abc" })
    expect(run([line])).toHaveLength(0)
  })

  it("ignores turn.started", () => {
    const line = JSON.stringify({ type: "turn.started" })
    expect(run([line])).toHaveLength(0)
  })

  it("creates in-progress codex-command entry for item.started command_execution", () => {
    const line = JSON.stringify({
      type: "item.started",
      item: { id: "item_1", type: "command_execution", command: "ls -la", status: "in_progress" },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      kind: "codex-command",
      id: "item_1",
      command: "ls -la",
      inProgress: true,
    })
  })

  it("updates codex-command entry when item.completed command_execution arrives", () => {
    const startLine = JSON.stringify({
      type: "item.started",
      item: { id: "item_1", type: "command_execution", command: "ls -la", status: "in_progress" },
    })
    const completeLine = JSON.stringify({
      type: "item.completed",
      item: { id: "item_1", type: "command_execution", text: "file1\nfile2" },
    })
    const entries = run([startLine, completeLine])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      kind: "codex-command",
      id: "item_1",
      command: "ls -la",
      inProgress: false,
      output: "file1\nfile2",
    })
  })

  it("creates text entry for item.completed agent_message", () => {
    const line = JSON.stringify({
      type: "item.completed",
      item: { id: "item_3", type: "agent_message", text: "Task done." },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "text", text: "Task done." })
  })

  it("falls back to raw for item.completed with unknown item type", () => {
    const line = JSON.stringify({
      type: "item.completed",
      item: { id: "item_5", type: "function_call", text: "result" },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stdout" })
  })

  it("creates codex-turn entry for turn.completed with usage", () => {
    const line = JSON.stringify({
      type: "turn.completed",
      usage: { input_tokens: 100, cached_input_tokens: 50, output_tokens: 20 },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      kind: "codex-turn",
      inputTokens: 100,
      cachedInputTokens: 50,
      outputTokens: 20,
    })
  })

  it("skips codex-turn when turn.completed has no usage", () => {
    const line = JSON.stringify({ type: "turn.completed" })
    expect(run([line])).toHaveLength(0)
  })

  it("falls back to raw for completely unknown event type", () => {
    const line = JSON.stringify({ type: "weird.event" })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stdout" })
  })

  it("falls back to raw when JSON parse fails", () => {
    const entries = run(["not valid json"])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", content: "not valid json" })
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd web && pnpm test -- --reporter=verbose
```

Expected: FAIL — `Cannot find module '../codex-parser'`

- [ ] **Step 3: Implement codex-parser.ts**

Create `web/src/components/log-viewer/codex-parser.ts`:
```typescript
import type { ParsedEntry, StreamParser } from "./types"
import { appendTextEntry, appendRawEntry } from "./types"

interface CodexEvent {
  type: string
  thread_id?: string
  item?: {
    id?: string
    type?: string
    command?: string
    text?: string
    status?: string
  }
  usage?: {
    input_tokens?: number
    cached_input_tokens?: number
    output_tokens?: number
  }
}

function parseCodexLine(line: string): CodexEvent | null {
  try {
    const obj = JSON.parse(line)
    if (obj && typeof obj.type === "string") return obj as CodexEvent
    return null
  } catch {
    return null
  }
}

export class CodexParser implements StreamParser {
  parseLine(
    line: string,
    logType: string,
    entries: ParsedEntry[],
    itemMap: Map<string, number>
  ): void {
    const event = parseCodexLine(line)

    if (!event) {
      appendRawEntry(line, logType, entries)
      return
    }

    if (event.type === "thread.started" || event.type === "turn.started") {
      return
    }

    if (event.type === "item.started") {
      const item = event.item
      if (item?.type === "command_execution" && item.id && item.command) {
        itemMap.set(item.id, entries.length)
        entries.push({
          kind: "codex-command",
          id: item.id,
          command: item.command,
          inProgress: true,
        })
      }
      return
    }

    if (event.type === "item.completed") {
      const item = event.item
      if (!item) {
        appendRawEntry(line, logType, entries)
        return
      }

      if (item.type === "agent_message" && item.text) {
        appendTextEntry(item.text, entries)
        return
      }

      if (item.type === "command_execution" && item.id) {
        const idx = itemMap.get(item.id)
        if (idx !== undefined) {
          const existing = entries[idx]
          if (existing?.kind === "codex-command") {
            entries[idx] = { ...existing, inProgress: false, output: item.text ?? "" }
            return
          }
        }
        // id not found in map — fall through to raw
      }

      appendRawEntry(line, logType, entries)
      return
    }

    if (event.type === "turn.completed") {
      const usage = event.usage
      if (
        usage &&
        typeof usage.input_tokens === "number" &&
        typeof usage.cached_input_tokens === "number" &&
        typeof usage.output_tokens === "number"
      ) {
        entries.push({
          kind: "codex-turn",
          inputTokens: usage.input_tokens,
          cachedInputTokens: usage.cached_input_tokens,
          outputTokens: usage.output_tokens,
        })
      }
      return
    }

    appendRawEntry(line, logType, entries)
  }
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd web && pnpm test -- --reporter=verbose
```

Expected: all 9 codex-parser tests pass, plus all prior tests (total: 21 tests)

- [ ] **Step 5: Commit**

```bash
cd web && git add src/components/log-viewer/codex-parser.ts src/components/log-viewer/__tests__/codex-parser.test.ts && git commit -m "feat: add CodexParser for Codex NDJSON log format"
```

---

## Task 6: Wire Parser Abstraction in log-viewer.tsx

**Files:**
- Modify: `web/src/components/log-viewer.tsx`

Replace the existing parsing infrastructure with the new parser abstraction. The UI render components remain unchanged for now.

- [ ] **Step 1: Add imports and remove old parsing code**

At the top of `web/src/components/log-viewer.tsx`, replace the existing imports section and the Claude-specific code (lines 1–216) with:

```typescript
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { ChevronDown, ChevronUp } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Streamdown } from "streamdown"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import type { ExecutionStatus } from "@/lib/types"
import { isActiveStatus } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { ParsedEntry, StreamParser } from "./log-viewer/types"
import { detectEngine } from "./log-viewer/detect-engine"
import { ClaudeParser } from "./log-viewer/claude-parser"
import { CodexParser } from "./log-viewer/codex-parser"

// Re-export helpers still used by UI components in this file
export { getToolMeta, stringify } from "./log-viewer/claude-parser"

type LogFilter = "all" | "text" | "tool" | "raw"
type LogViewerVariant = "standalone" | "embedded"

interface LogViewerProps {
  executionId: string
  status: ExecutionStatus
  onComplete?: () => void
  autoScroll?: boolean
  variant?: LogViewerVariant
}
```

- [ ] **Step 2: Update LogViewer component refs and appendLines logic**

Inside `export function LogViewer(...)`, add two new refs after the existing refs and update `appendLines` / `rebuildEntries`:

Find the ref declarations block (currently after `useState` calls):
```typescript
const toolMapRef = useRef<Map<string, number>>(new Map())
const viewportRef = useRef<HTMLDivElement>(null)
const parsedLengthRef = useRef(0)
const pendingLineRef = useRef("")
const prevStatusRef = useRef<ExecutionStatus>(status)
```

Replace with:
```typescript
const toolMapRef = useRef<Map<string, number>>(new Map())
const parserRef = useRef<StreamParser | null>(null)
const viewportRef = useRef<HTMLDivElement>(null)
const parsedLengthRef = useRef(0)
const pendingLineRef = useRef("")
const prevStatusRef = useRef<ExecutionStatus>(status)
```

- [ ] **Step 3: Update the executionId reset effect**

Find:
```typescript
useEffect(() => {
  setEntries([])
  toolMapRef.current = new Map()
  parsedLengthRef.current = 0
  pendingLineRef.current = ""
  setFollowLive(autoScroll)
}, [executionId, autoScroll])
```

Replace with:
```typescript
useEffect(() => {
  setEntries([])
  toolMapRef.current = new Map()
  parserRef.current = null
  parsedLengthRef.current = 0
  pendingLineRef.current = ""
  setFollowLive(autoScroll)
}, [executionId, autoScroll])
```

- [ ] **Step 4: Replace appendLines helper inside the fetch useEffect**

Find the `appendLines` function inside the fetch `useEffect`:
```typescript
const appendLines = (lines: string[]) => {
  if (lines.length === 0) return
  setEntries((previous) => {
    const next = [...previous]
    lines.forEach((line) => appendEntry(line, "stdout", next, toolMapRef.current))
    return next
  })
}
```

Replace with:
```typescript
const ensureParser = (firstLine: string): StreamParser => {
  if (!parserRef.current) {
    parserRef.current =
      detectEngine(firstLine) === "codex" ? new CodexParser() : new ClaudeParser()
  }
  return parserRef.current
}

const appendLines = (lines: string[]) => {
  if (lines.length === 0) return
  setEntries((previous) => {
    const parser = ensureParser(lines[0])
    const next = [...previous]
    lines.forEach((line) => parser.parseLine(line, "stdout", next, toolMapRef.current))
    return next
  })
}
```

- [ ] **Step 5: Replace rebuildEntries helper**

Find:
```typescript
const rebuildEntries = (content: string, flushTail: boolean) => {
  const segments = content.split("\n")
  if (content.endsWith("\n")) {
    segments.pop()
    pendingLineRef.current = ""
  } else if (!flushTail) {
    pendingLineRef.current = segments.pop() ?? ""
  } else {
    pendingLineRef.current = ""
  }

  const nextEntries: ParsedEntry[] = []
  const nextToolMap = new Map<string, number>()
  segments.filter(Boolean).forEach((line) => appendEntry(line, "stdout", nextEntries, nextToolMap))
  toolMapRef.current = nextToolMap
  setEntries(nextEntries)
}
```

Replace with:
```typescript
const rebuildEntries = (content: string, flushTail: boolean) => {
  const segments = content.split("\n")
  if (content.endsWith("\n")) {
    segments.pop()
    pendingLineRef.current = ""
  } else if (!flushTail) {
    pendingLineRef.current = segments.pop() ?? ""
  } else {
    pendingLineRef.current = ""
  }

  const lines = segments.filter(Boolean)
  const nextEntries: ParsedEntry[] = []
  const nextToolMap = new Map<string, number>()
  // Reset parser so engine is re-detected from the full stream
  parserRef.current = null
  if (lines.length > 0) {
    const parser = ensureParser(lines[0])
    lines.forEach((line) => parser.parseLine(line, "stdout", nextEntries, nextToolMap))
  }
  toolMapRef.current = nextToolMap
  setEntries(nextEntries)
}
```

- [ ] **Step 6: Verify TypeScript compiles**

```bash
cd web && pnpm build 2>&1 | head -40
```

Expected: build succeeds with no TypeScript errors. Fix any type errors before proceeding.

- [ ] **Step 7: Commit**

```bash
cd web && git add src/components/log-viewer.tsx && git commit -m "refactor: wire StreamParser abstraction in LogViewer"
```

---

## Task 7: Add CodexCommandEntry and CodexTurnEntry Components

**Files:**
- Modify: `web/src/components/log-viewer.tsx`

- [ ] **Step 1: Add CodexCommandEntry component**

In `log-viewer.tsx`, after the `ResultEntry` component (after line ~373) and before `RawEntry`, insert:

```typescript
function CodexCommandEntry({
  entry,
}: {
  entry: Extract<ParsedEntry, { kind: "codex-command" }>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(entry.inProgress || Boolean(!entry.output))

  const markerClass = entry.inProgress
    ? "bg-primary/55"
    : entry.output !== undefined
      ? "bg-status-idle"
      : "bg-muted-foreground/55"

  return (
    <TimelineRow markerClassName={markerClass}>
      <article className="overflow-hidden rounded-2xl border border-border/70 bg-background/80">
        <button
          type="button"
          aria-expanded={open}
          aria-label={
            open
              ? t("logViewer.collapse", { name: t("logViewer.commandExecution") })
              : t("logViewer.expand", { name: t("logViewer.commandExecution") })
          }
          onClick={() => setOpen((c) => !c)}
          className="flex w-full items-start gap-3 px-4 py-4 text-left transition-colors hover:bg-muted/25"
        >
          <span className="inline-flex h-7 shrink-0 items-center rounded-full border border-border/70 bg-background px-2.5 font-mono text-[11px] tracking-[0.18em] text-muted-foreground">
            SH
          </span>

          <div className="min-w-0 flex-1">
            <p className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
              {t("logViewer.commandExecution")}
            </p>
            <div className="mt-1 flex flex-wrap items-baseline gap-2">
              <p className="min-w-0 flex-1 truncate font-mono text-sm text-foreground">
                {entry.command}
              </p>
              {entry.inProgress && (
                <span className="text-[11px] text-status-working">{t("logViewer.running")}</span>
              )}
            </div>
          </div>

          <span className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden="true">
            {open ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
          </span>
        </button>

        {open && (
          <div className="grid gap-3 border-t border-border/70 px-4 pb-4 pt-3 md:grid-cols-2 animate-fade-in">
            <section className="space-y-2">
              <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                {t("logViewer.input")}
              </p>
              <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-xl border border-border/70 bg-muted/35 p-3 font-mono text-[12px] leading-6 text-foreground">
                {entry.command}
              </pre>
            </section>

            <section className="space-y-2">
              <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                {t("logViewer.output")}
              </p>
              <div className="rounded-xl border border-border/70 bg-muted/35 p-3">
                {entry.inProgress ? (
                  <p className="text-sm text-muted-foreground">{t("logViewer.running")}</p>
                ) : (
                  <pre className="overflow-x-auto whitespace-pre-wrap break-words font-mono text-[12px] leading-6 text-foreground">
                    {entry.output || "—"}
                  </pre>
                )}
              </div>
            </section>
          </div>
        )}
      </article>
    </TimelineRow>
  )
}

function CodexTurnEntry({
  entry,
}: {
  entry: Extract<ParsedEntry, { kind: "codex-turn" }>
}) {
  const { t } = useTranslation()

  return (
    <TimelineRow markerClassName="bg-muted-foreground/40">
      <div className="flex flex-wrap items-center gap-2 py-2">
        <span className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
          {t("logViewer.turnUsage")}
        </span>
        <MetricChip label={t("logViewer.inputTokens")} value={entry.inputTokens} />
        <MetricChip label={t("logViewer.cachedTokens")} value={entry.cachedInputTokens} />
        <MetricChip label={t("logViewer.outputTokens")} value={entry.outputTokens} />
      </div>
    </TimelineRow>
  )
}
```

- [ ] **Step 2: Update the entry render switch in the JSX**

Find the section in the `LogViewer` JSX that renders entries. Look for the `visibleEntries.map(...)` block. It currently renders entries by checking `entry.kind`. Add cases for the two new kinds:

Locate the map call that renders entries — it looks approximately like:
```tsx
{visibleEntries.map((entry, i) => {
  if (entry.kind === "text") return <AssistantEntry key={i} text={entry.text} />
  if (entry.kind === "tool") return <ToolEntry key={i} entry={entry} />
  if (entry.kind === "result") return <ResultEntry key={i} entry={entry} />
  return <RawEntry key={i} entry={entry} />
})}
```

Replace with:
```tsx
{visibleEntries.map((entry, i) => {
  if (entry.kind === "text") return <AssistantEntry key={i} text={entry.text} />
  if (entry.kind === "tool") return <ToolEntry key={i} entry={entry} />
  if (entry.kind === "result") return <ResultEntry key={i} entry={entry} />
  if (entry.kind === "codex-command") return <CodexCommandEntry key={i} entry={entry} />
  if (entry.kind === "codex-turn") return <CodexTurnEntry key={i} entry={entry} />
  return <RawEntry key={i} entry={entry} />
})}
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd web && pnpm build 2>&1 | head -40
```

Expected: no errors. Fix any type issues before continuing.

- [ ] **Step 4: Commit**

```bash
cd web && git add src/components/log-viewer.tsx && git commit -m "feat: add CodexCommandEntry and CodexTurnEntry UI components"
```

---

## Task 8: Update Filter Logic for Codex Entries

**Files:**
- Modify: `web/src/components/log-viewer.tsx`

- [ ] **Step 1: Update entry counting in useMemo**

Find the `useMemo` that computes `filterOptions` and `visibleEntries`. The counting loop currently is:
```typescript
for (const entry of entries) {
  if (entry.kind === "text") narrativeCount += 1
  if (entry.kind === "tool") toolCount += 1
  if (entry.kind === "raw") rawCount += 1
}
```

Replace with:
```typescript
for (const entry of entries) {
  if (entry.kind === "text") narrativeCount += 1
  if (entry.kind === "tool" || entry.kind === "codex-command") toolCount += 1
  if (entry.kind === "raw") rawCount += 1
}
```

- [ ] **Step 2: Update visibleEntries filter**

Find:
```typescript
const visibleEntries = entries.filter((entry) => {
  if (entry.kind === "result") return true
  if (filter === "all") return true
  return entry.kind === filter
})
```

Replace with:
```typescript
const visibleEntries = entries.filter((entry) => {
  if (entry.kind === "result" || entry.kind === "codex-turn") return true
  if (filter === "all") return true
  if (entry.kind === "codex-command") return filter === "tool"
  return entry.kind === filter
})
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd web && pnpm build 2>&1 | head -40
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
cd web && git add src/components/log-viewer.tsx && git commit -m "feat: include codex-command in tool filter, always show codex-turn"
```

---

## Task 9: Add i18n Strings

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add English strings to en.json**

In `web/src/locales/en.json`, find the `"logViewer"` object and add after the last existing key (`"failed"`):

```json
"commandExecution": "Command",
"running": "Running...",
"turnUsage": "Turn",
"inputTokens": "input",
"cachedTokens": "cached",
"outputTokens": "output"
```

The full `logViewer` block should end as:
```json
"logViewer": {
  "all": "All",
  "entries": "Events",
  "narrative": "Narrative",
  "tools": "Tool calls",
  "raw": "Raw stream",
  "waiting": "Waiting for output...",
  "noLogs": "No logs recorded.",
  "noMatches": "Current filters hide every event.",
  "input": "Input",
  "output": "Output",
  "expand": "Expand {{name}}",
  "collapse": "Collapse {{name}}",
  "result": "Result",
  "followLive": "Following live",
  "jumpToLatest": "Jump to latest",
  "toolCall": "Tool call",
  "assistant": "Assistant",
  "rawOutput": "Raw output",
  "pending": "Pending",
  "returned": "Returned",
  "failed": "Failed",
  "commandExecution": "Command",
  "running": "Running...",
  "turnUsage": "Turn",
  "inputTokens": "input",
  "cachedTokens": "cached",
  "outputTokens": "output"
}
```

- [ ] **Step 2: Add Chinese strings to zh.json**

In `web/src/locales/zh.json`, find the `"logViewer"` object and add after `"failed"`:

```json
"commandExecution": "命令",
"running": "执行中...",
"turnUsage": "本轮用量",
"inputTokens": "输入",
"cachedTokens": "缓存",
"outputTokens": "输出"
```

- [ ] **Step 3: Final build verification**

```bash
cd web && pnpm build 2>&1 | tail -20
```

Expected: build completes successfully with no errors.

- [ ] **Step 4: Commit**

```bash
cd web && git add src/locales/en.json src/locales/zh.json && git commit -m "feat: add i18n strings for Codex log rendering"
```

---

## Task 10: Manual Verification

- [ ] **Step 1: Start the dev server**

```bash
cd web && pnpm dev
```

- [ ] **Step 2: Run a Codex execution and open the session detail page**

Navigate to `/sessions/detail?session_id=<a-codex-session-id>` and verify:
- `agent_message` items render as narrative text (blue marker, markdown rendered)
- `command_execution` items render as `CodexCommandEntry` with "Running..." state during execution, updated with output once complete
- `turn.completed` shows a token usage row with input/cached/output chips
- Filter buttons correctly count and show/hide entries (Tool calls filter shows commands)
- Claude sessions still render correctly (regression check: open a Claude session)

- [ ] **Step 3: Check that `item.completed` output field resolves correctly**

If `command_execution` items show empty output after completion, check the actual Codex log file to find the correct field name. The log file for a given execution is at the path logged by the backend. If the output is in a field other than `text`, update `codex-parser.ts` line:

```typescript
entries[idx] = { ...existing, inProgress: false, output: item.text ?? "" }
```

to use the correct field name, then re-run `pnpm test` to update the test for `item.completed command_execution`.

---

## Self-Review Notes

- All 9 tasks map directly to a spec section
- Types in Task 2 (`ParsedEntry`, `StreamParser`) are consistently used across Tasks 3–8
- `codex-command` entries map to `"tool"` filter key throughout (Task 5 parser + Task 8 filter logic)
- `appendRawEntry` and `appendTextEntry` exported from `types.ts` — both parsers import from there
- `getToolMeta` and `stringify` re-exported from `claude-parser.ts` for use by `ToolEntry` in `log-viewer.tsx`
- The `command_execution` output field (`item.text`) is noted as empirically unverified — Task 10 Step 3 covers the fix path
