import type { Query } from "@tanstack/react-query"
import { describe, expect, it } from "vitest"
import { ApiError } from "@/lib/api"
import type { ExecutionDetail } from "@/lib/types"
import { executionGone, executionQuery } from "../use-execution"

const detail = (status: ExecutionDetail["status"]): ExecutionDetail => ({
  id: "e1",
  task_id: "t1",
  worker_id: "w1",
  engine: "claude",
  status,
  instruction: "go",
  started_at: 1_000,
  completed_at: status === "running" ? null : 2_000,
  error: "",
  steps: [],
  steps_error: "",
})

const gone = new ApiError(404, "execution not found", "execution_not_found")

const interval = (data: ExecutionDetail | undefined, error: Error | null) => {
  const { refetchInterval } = executionQuery("e1")
  if (typeof refetchInterval !== "function") throw new Error("refetchInterval is not a function")
  return refetchInterval({ state: { data, error } } as Query<ExecutionDetail, Error, ExecutionDetail, string[]>)
}

describe("executionQuery", () => {
  it("polls a running execution", () => {
    expect(interval(detail("running"), null)).toBe(2000)
    expect(interval(detail("completed"), null)).toBe(false)
    expect(interval(undefined, null)).toBe(false)
  })

  it("stops polling once the execution is gone, but not on other errors", () => {
    expect(interval(detail("running"), gone)).toBe(false)
    expect(interval(detail("running"), new ApiError(500, "boom"))).toBe(2000)
  })

  it("tells a missing execution apart from other errors", () => {
    expect(executionGone(gone)).toBe(true)
    expect(executionGone(new ApiError(500, "boom"))).toBe(false)
    expect(executionGone(null)).toBe(false)
  })
})
