import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { I18nextProvider } from "react-i18next"
import { describe, expect, it } from "vitest"
import type { HandoffInfo, Worker } from "@/lib/types"
import { testI18n } from "@/test/i18n"
import { HandoffCard } from "./handoff-card"

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

const base: HandoffInfo = { worker_id: "w1", task_id: "t1", conversation_id: "worker:w1", task_type: "immediate" }

const i18n = testI18n("en")

const render = (info: HandoffInfo, onShowExecution?: (id: string) => void) =>
  renderToStaticMarkup(
    createElement(
      I18nextProvider,
      { i18n },
      createElement(HandoffCard, {
        info,
        instruction: "go",
        workers: new Map([["w1", worker]]),
        onOpen: () => {},
        onShowExecution,
      }),
    ),
  )

describe("HandoffCard", () => {
  it("shows the worker avatar without a status dot", () => {
    const html = render(base)
    expect(html).toContain('aria-label="L I"')
    expect(html).not.toContain("ring-surface")
    expect(html).not.toContain("animate-ping")
  })

  it("has no run row before the task runs", () => {
    const html = render(base, () => {})
    expect(html).not.toContain("View process")
  })

  it("shows a spinning run row while the task runs", () => {
    const html = render({ ...base, run: { id: "e1", status: "running", started_at: 1_000 } }, () => {})
    expect(html).toContain("Running")
    expect(html).toContain("animate-spin")
    expect(html).toContain("View process")
  })

  it("shows how long a finished run took", () => {
    const html = render(
      { ...base, run: { id: "e1", status: "failed", started_at: 1_000, completed_at: 601_000 } },
      () => {},
    )
    expect(html).toContain("Failed · Took 10m")
    expect(html).toContain("text-danger")
    expect(html).not.toContain("Latest: ")
  })

  it("marks the run of a scheduled task as the latest one", () => {
    const html = render(
      {
        ...base,
        task_type: "scheduled",
        cron_expr: "0 9 * * *",
        run: { id: "e1", status: "completed", started_at: 1_000, completed_at: 109_000 },
      },
      () => {},
    )
    expect(html).toContain("Latest: Completed · Took 1m 48s")
  })

  it("hides the run row without a handler", () => {
    const html = render({ ...base, run: { id: "e1", status: "running", started_at: 1_000 } })
    expect(html).not.toContain("View process")
  })
})
