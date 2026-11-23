import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { I18nextProvider } from "react-i18next"
import { describe, expect, it } from "vitest"
import type { ExecutionDetail, ExecutionStep, Worker } from "@/lib/types"
import { testI18n } from "@/test/i18n"
import { ExecutionView } from "./execution-panel"

const worker: Worker = {
  id: "w1",
  name: "Lin",
  description: "",
  constraints: "",
  work_dir: "",
  engine: "claude",
  status: "working",
  created_at: 0,
  updated_at: 0,
}

const execution = (extra: Partial<ExecutionDetail> = {}): ExecutionDetail => ({
  id: "e1",
  task_id: "t1",
  worker_id: "w1",
  engine: "claude",
  status: "completed",
  instruction: "Update `README`",
  started_at: 1_000,
  completed_at: 193_000,
  error: "",
  steps: [],
  steps_error: "",
  ...extra,
})

const runningTool: ExecutionStep = { type: "tool", kind: "command", name: "Bash", summary: "pnpm test", status: "running" }

const render = (detail: ExecutionDetail, lng: "en" | "zh" = "en") =>
  renderToStaticMarkup(
    createElement(
      I18nextProvider,
      { i18n: testI18n(lng) },
      createElement(ExecutionView, {
        execution: detail,
        worker,
        engine: { id: "claude", name: "Claude Code" },
        onSelectRun: () => {},
      }),
    ),
  )

describe("ExecutionView", () => {
  it("shows the summary of a finished execution", () => {
    const html = render(execution())
    expect(html).toContain("Lin")
    expect(html).toContain("Completed")
    expect(html).toContain("Update README")
    expect(html).toContain("Claude Code")
    expect(html).toContain("Took 3m 12s")
  })

  it("explains an engine plugin without transcript support", () => {
    const html = render(execution({ steps_error: "transcript_unsupported" }))
    expect(html).toContain("This engine plugin can&#x27;t show the process yet. Update it in Settings → Plugins.")
    expect(html).not.toContain("No steps were recorded.")
    expect(render(execution({ steps_error: "transcript_unsupported" }), "zh")).toContain(
      "当前引擎插件版本不支持查看过程，请在设置 → 插件中升级",
    )
  })

  it.each([
    ["engine_unavailable", "The engine plugin isn&#x27;t installed or is turned off, so the process can&#x27;t be shown."],
    ["transcript_failed", "Couldn&#x27;t read the process. Try again later."],
  ] as const)("explains %s", (stepsError, text) => {
    expect(render(execution({ steps_error: stepsError }))).toContain(text)
  })

  it("waits for the first step while running", () => {
    const html = render(execution({ status: "running", completed_at: null }))
    expect(html).toContain("Waiting for the first step…")
    expect(html).toContain("Updating live")
  })

  it("says no steps were recorded after the run", () => {
    const html = render(execution())
    expect(html).toContain("No steps were recorded.")
    expect(html).not.toContain("steps,")
  })

  it("spins a running tool while the execution runs", () => {
    const html = render(execution({ status: "running", completed_at: null, steps: [runningTool] }))
    expect(html).toContain("animate-spin")
    expect(html).not.toContain("Stopped")
  })

  it("marks a running tool as stopped once the execution ended", () => {
    const html = render(execution({ status: "failed", error: "timed out", steps: [runningTool] }))
    expect(html).not.toContain("animate-spin")
    expect(html).toContain("Stopped")
    expect(html).toContain("Run failed")
    expect(html).toContain("timed out")
  })

  it("renders the steps and counts them in the footer", () => {
    const html = render(
      execution({
        steps: [
          { type: "thinking", text: "Find the install section" },
          { type: "tool", kind: "edit", name: "Edit", summary: "README.md", output: "+npm i -g openbee", status: "done", duration_ms: 41_000 },
          { type: "tool", kind: "message", name: "send_message", summary: "On it", status: "done" },
          { type: "tool", kind: "other", name: "todo_write", status: "error" },
          { type: "error", text: "rate limited" },
          { type: "text", text: "All done" },
        ],
      }),
    )
    expect(html).toContain("Find the install section")
    expect(html).toContain("Edited a file")
    expect(html).toContain("README.md")
    expect(html).toContain("41s")
    expect(html).toContain('aria-expanded="false"')
    expect(html).toContain("Sent a message")
    expect(html).toContain("On it")
    expect(html).toContain("Called todo_write")
    expect(html).toContain("rate limited")
    expect(html).toContain("All done")
    expect(html).toContain("6 steps, 3 tool calls")
  })

  it("shows the run switcher only with several runs", () => {
    const runs = [
      { id: "e2", status: "running" as const, started_at: 5_000 },
      { id: "e1", status: "completed" as const, started_at: 1_000, completed_at: 193_000 },
    ]
    expect(render(execution({ runs }))).toContain('role="combobox"')
    expect(render(execution({ runs: [runs[1]] }))).not.toContain('role="combobox"')
    expect(render(execution())).not.toContain('role="combobox"')
  })
})
