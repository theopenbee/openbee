import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"
import type { HandoffInfo, Worker } from "@/lib/types"
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

describe("HandoffCard", () => {
  it("shows the worker avatar without a status dot", () => {
    const info: HandoffInfo = { worker_id: "w1", task_id: "t1", conversation_id: "worker:w1", task_type: "immediate" }
    const html = renderToStaticMarkup(
      createElement(HandoffCard, { info, instruction: "go", workers: new Map([["w1", worker]]), onOpen: () => {} }),
    )
    expect(html).toContain('aria-label="L I"')
    expect(html).not.toContain("ring-surface")
    expect(html).not.toContain("animate-ping")
  })
})
