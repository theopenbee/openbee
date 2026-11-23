import { describe, expect, it } from "vitest"
import { testI18n } from "@/test/i18n"
import type { ExecutionDetail, ExecutionStep, ToolStep } from "../types"
import {
  diffLines,
  diffStats,
  formatDuration,
  handoffRunLabel,
  isToolKind,
  knownSteps,
  splitThinking,
  stepStats,
  switcherRuns,
  toolDuration,
  toolKind,
  toolTitle,
} from "../execution"

const en = testI18n("en").t
const zh = testI18n("zh").t

describe("formatDuration", () => {
  it("shows seconds under a minute and at least one second", () => {
    expect(formatDuration(41_000, en)).toBe("41s")
    expect(formatDuration(41_000, zh)).toBe("41 秒")
    expect(formatDuration(200, en)).toBe("1s")
    expect(formatDuration(-5_000, en)).toBe("1s")
  })

  it("shows minutes and seconds", () => {
    expect(formatDuration(65_000, zh)).toBe("1 分 5 秒")
    expect(formatDuration(75_000, en)).toBe("1m 15s")
  })

  it("shows minutes only on a whole minute", () => {
    expect(formatDuration(600_000, zh)).toBe("10 分钟")
    expect(formatDuration(600_000, en)).toBe("10m")
  })

  it("shows hours and minutes for long runs", () => {
    expect(formatDuration(3_900_000, en)).toBe("1h 5m")
    expect(formatDuration(7_200_000, zh)).toBe("2 小时")
  })
})

describe("toolDuration", () => {
  it("hides durations under a second", () => {
    expect(toolDuration(undefined, en)).toBeNull()
    expect(toolDuration(0, en)).toBeNull()
    expect(toolDuration(999, en)).toBeNull()
  })

  it("formats a second or more", () => {
    expect(toolDuration(1_000, en)).toBe("1s")
    expect(toolDuration(61_000, zh)).toBe("1 分 1 秒")
  })
})

describe("handoffRunLabel", () => {
  it("shows the status of a running run without a duration", () => {
    expect(handoffRunLabel({ id: "e1", status: "running", started_at: 1_000 }, false, en)).toBe("Running")
  })

  it("adds how long a finished run took", () => {
    const run = { id: "e1", status: "completed", started_at: 1_000, completed_at: 109_000 } as const
    expect(handoffRunLabel(run, false, en)).toBe("Completed · Took 1m 48s")
    expect(handoffRunLabel({ ...run, status: "failed", completed_at: 601_000 }, false, zh)).toBe("失败 · 用时 10 分钟")
  })

  it("prefixes the latest run of a scheduled task", () => {
    const run = { id: "e1", status: "completed", started_at: 1_000, completed_at: 109_000 } as const
    expect(handoffRunLabel(run, true, en)).toBe("Latest: Completed · Took 1m 48s")
    expect(handoffRunLabel(run, true, zh)).toBe("最近一次：已完成 · 用时 1 分 48 秒")
    expect(handoffRunLabel({ id: "e2", status: "running", started_at: 1_000 }, true, en)).toBe("Latest: Running")
  })

  it("leaves out the duration when the end time is missing", () => {
    expect(handoffRunLabel({ id: "e1", status: "failed", started_at: 1_000, completed_at: null }, false, en)).toBe(
      "Failed",
    )
  })
})

describe("stepStats", () => {
  it("counts steps and tool calls", () => {
    const steps: ExecutionStep[] = [
      { type: "thinking", text: "plan" },
      { type: "tool", kind: "command", name: "Bash", status: "done" },
      { type: "text", text: "done" },
      { type: "tool", kind: "read", name: "Read", status: "done" },
    ]
    expect(stepStats(steps)).toEqual({ steps: 4, tools: 2 })
    expect(stepStats([])).toEqual({ steps: 0, tools: 0 })
  })
})

describe("knownSteps", () => {
  it("drops step types this version does not know", () => {
    const text: ExecutionStep = { type: "text", text: "hi" }
    const future = { type: "image", text: "x" } as unknown as ExecutionStep
    expect(knownSteps([text, future])).toEqual([text])
  })
})

describe("isToolKind", () => {
  it("accepts only known tool kinds", () => {
    expect(["command", "read", "edit", "search", "web", "message", "other"].every(isToolKind)).toBe(true)
    expect(isToolKind("constructor")).toBe(false)
    expect(isToolKind("browser")).toBe(false)
  })
})

describe("switcherRuns", () => {
  const detail = (extra: Partial<ExecutionDetail> = {}): ExecutionDetail => ({
    id: "e1",
    task_id: "t1",
    worker_id: "w1",
    engine: "claude",
    status: "failed",
    instruction: "go",
    started_at: 1_000,
    completed_at: 9_000,
    error: "",
    steps: [],
    steps_error: "",
    ...extra,
  })

  it("is empty for a task that ran once", () => {
    expect(switcherRuns(detail())).toEqual([])
  })

  it("keeps the runs when they include the open execution", () => {
    const runs = [
      { id: "e2", status: "running" as const, started_at: 20_000 },
      { id: "e1", status: "failed" as const, started_at: 1_000, completed_at: 9_000 },
    ]
    expect(switcherRuns(detail({ runs }))).toEqual(runs)
  })

  it("appends an open execution older than the listed runs", () => {
    const runs = [
      { id: "e3", status: "completed" as const, started_at: 30_000, completed_at: 31_000 },
      { id: "e2", status: "completed" as const, started_at: 20_000, completed_at: 21_000 },
    ]
    expect(switcherRuns(detail({ runs }))).toEqual([
      ...runs,
      { id: "e1", status: "failed", started_at: 1_000, completed_at: 9_000 },
    ])
  })
})

describe("toolTitle", () => {
  const step = (kind: string, summary?: string, name = "Tool") =>
    ({ type: "tool", kind, name, summary, status: "done" }) as ToolStep

  it("joins the verb and the summary", () => {
    expect(toolTitle(step("read", "src/app.ts"), en)).toBe("Read src/app.ts")
    expect(toolTitle(step("search", "rate limit"), zh)).toBe("搜索 rate limit")
    expect(toolTitle(step("message", "On it"), zh)).toBe("发送消息：On it")
  })

  it("collapses whitespace in the summary", () => {
    expect(toolTitle(step("message", "  Done.\n\nSee   the PR "), en)).toBe("Sent a message: Done. See the PR")
  })

  it("uses the bare verb without a summary", () => {
    expect(toolTitle(step("command"), en)).toBe("Ran a command")
    expect(toolTitle(step("edit", "   "), zh)).toBe("修改文件")
  })

  it("names other and unknown kinds by the tool name", () => {
    expect(toolTitle(step("other", "3 todos", "todo_write"), en)).toBe("Called todo_write")
    expect(toolTitle(step("browser", "x", "navigate"), zh)).toBe("调用 navigate")
    expect(toolKind(step("browser"))).toBe("other")
  })
})

describe("splitThinking", () => {
  it("takes the first non-empty line as the title", () => {
    expect(splitThinking("\n  Plan the change \nCheck routes\n\nThen tests\n")).toEqual({
      title: "Plan the change",
      rest: "Check routes\n\nThen tests",
    })
  })

  it("drops bold markers around the title", () => {
    expect(splitThinking("**Planning**\n\nRead first").title).toBe("Planning")
    expect(splitThinking("__Planning__").title).toBe("Planning")
    expect(splitThinking("**a** and **b**").title).toBe("**a** and **b**")
  })

  it("has no rest for a single line", () => {
    expect(splitThinking("One line")).toEqual({ title: "One line", rest: "" })
    expect(splitThinking("")).toEqual({ title: "", rest: "" })
  })
})

describe("diffLines", () => {
  it("reads the marker of each line", () => {
    expect(diffLines(" keep\n-old\n+new\n+\n…\n@@ -1 +1 @@\n")).toEqual([
      { type: "context", text: "keep" },
      { type: "remove", text: "old" },
      { type: "add", text: "new" },
      { type: "add", text: "" },
      { type: "note", text: "…" },
      { type: "note", text: "@@ -1 +1 @@" },
    ])
  })

  it("counts added and removed lines", () => {
    expect(diffStats(diffLines("-a\n+b\n+c\n d"))).toEqual({ added: 2, removed: 1 })
    expect(diffStats([])).toEqual({ added: 0, removed: 0 })
  })
})
