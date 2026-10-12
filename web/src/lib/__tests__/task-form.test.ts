import { describe, expect, it } from "vitest"
import { initialTaskFormValues, nextFullHour, toDateTimeLocalValue, toTaskInput, type TaskFormValues } from "../task-form"
import type { Task } from "../types"

const now = new Date(2026, 9, 12, 14, 25, 30).getTime()

function task(overrides: Partial<Task>): Task {
  return {
    id: "t1",
    worker_id: "w1",
    worker_name: "Ada",
    instruction: "report",
    type: "scheduled",
    status: "pending",
    scheduled_at: null,
    cron_expr: "",
    next_run_at: null,
    created_at: 0,
    updated_at: 0,
    ...overrides,
  }
}

const base: TaskFormValues = {
  workerId: "w1",
  instruction: " daily report ",
  type: "scheduled",
  cronExpr: " 0  9 * * 1-5 ",
  runAt: "",
}

describe("toDateTimeLocalValue", () => {
  it("formats in local time without seconds", () => {
    expect(toDateTimeLocalValue(new Date(2026, 0, 5, 7, 3, 59).getTime())).toBe("2026-01-05T07:03")
  })
})

describe("nextFullHour", () => {
  it("rounds up to the next hour", () => {
    expect(nextFullHour(now)).toBe(new Date(2026, 9, 12, 15, 0, 0).getTime())
  })
})

describe("initialTaskFormValues", () => {
  it("defaults a new task to a daily schedule for the given worker", () => {
    expect(initialTaskFormValues(null, "w9", now)).toEqual({
      workerId: "w9",
      instruction: "",
      type: "scheduled",
      cronExpr: "0 9 * * *",
      runAt: "2026-10-12T15:00",
    })
  })

  it("loads a scheduled task", () => {
    const values = initialTaskFormValues(task({ cron_expr: "*/5 * * * *" }), undefined, now)
    expect(values).toMatchObject({ workerId: "w1", instruction: "report", type: "scheduled", cronExpr: "*/5 * * * *" })
  })

  it("loads a countdown task", () => {
    const at = new Date(2026, 9, 20, 8, 30).getTime()
    const values = initialTaskFormValues(task({ type: "countdown", scheduled_at: at }), undefined, now)
    expect(values).toMatchObject({ type: "countdown", runAt: "2026-10-20T08:30", cronExpr: "0 9 * * *" })
  })
})

describe("toTaskInput", () => {
  it("builds a scheduled input with trimmed values", () => {
    expect(toTaskInput(base, now)).toEqual({
      input: { worker_id: "w1", instruction: "daily report", type: "scheduled", cron_expr: "0 9 * * 1-5" },
    })
  })

  it("builds a countdown input from the local date time", () => {
    const result = toTaskInput({ ...base, type: "countdown", runAt: "2026-10-12T15:00" }, now)
    expect(result).toEqual({
      input: {
        worker_id: "w1",
        instruction: "daily report",
        type: "countdown",
        scheduled_at: new Date(2026, 9, 12, 15, 0).getTime(),
      },
    })
  })

  it.each([
    ["worker", { workerId: "" }],
    ["instruction", { instruction: "   " }],
    ["cron", { cronExpr: "  " }],
    ["runAt", { type: "countdown" as const, runAt: "" }],
    ["runAt", { type: "countdown" as const, runAt: "2026-10-12T14:25" }],
  ])("reports %s errors", (error, overrides) => {
    expect(toTaskInput({ ...base, ...overrides }, now)).toEqual({ error })
  })
})
