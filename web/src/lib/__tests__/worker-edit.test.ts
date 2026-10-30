import { describe, expect, it } from "vitest"
import { buildWorkerUpdate, followedEngine, type WorkerEditValues } from "../worker-edit"
import type { Worker } from "../types"

const worker = (over: Partial<Worker>): Worker => ({
  id: "w1",
  name: "sam",
  description: "writes code",
  constraints: "",
  work_dir: "/tmp/sam",
  engine: "pi",
  engine_args: {},
  status: "idle",
  created_at: 0,
  updated_at: 0,
  ...over,
})

const values = (w: Worker, over: Partial<WorkerEditValues> = {}): WorkerEditValues => ({
  name: w.name,
  description: w.description,
  workDir: w.work_dir,
  engine: w.engine ?? "",
  engineArgs: w.engine_args ?? {},
  ...over,
})

describe("buildWorkerUpdate", () => {
  it("returns null when nothing changed", () => {
    const w = worker({})
    expect(buildWorkerUpdate(w, values(w))).toBeNull()
  })

  it("keeps an unset-engine worker on the system default when another field changes", () => {
    const w = worker({ engine: "" })
    const data = buildWorkerUpdate(w, values(w, { description: "reviews code" }))
    expect(data).toEqual({ description: "reviews code", engine_args: {} })
    expect(data).not.toHaveProperty("engine")
  })

  it("returns null for an unset-engine worker with no edits", () => {
    const w = worker({ engine: "" })
    expect(buildWorkerUpdate(w, values(w))).toBeNull()
  })

  it("treats a missing engine as unset", () => {
    const w = worker({ engine: undefined as unknown as string })
    const data = buildWorkerUpdate(w, values(w, { description: "x" }))
    expect(data).not.toHaveProperty("engine")
  })

  it("sends the engine when the user picks one for an unset-engine worker", () => {
    const w = worker({ engine: "" })
    expect(buildWorkerUpdate(w, values(w, { engine: "claude" }))).toEqual({ description: "writes code", engine: "claude", engine_args: {} })
  })

  it("sends the engine when the user switches it", () => {
    const w = worker({ engine: "pi" })
    expect(buildWorkerUpdate(w, values(w, { engine: "claude" }))).toMatchObject({ engine: "claude" })
  })

  it("does not resend a stored engine that is no longer installed", () => {
    const w = worker({ engine: "codex", engine_args: { codex: "--model o3" } })
    const data = buildWorkerUpdate(w, values(w, { description: "edited" }))
    expect(data).toEqual({ description: "edited", engine_args: { codex: "--model o3" } })
  })

  it("sends name and work_dir only when they changed", () => {
    const w = worker({})
    expect(buildWorkerUpdate(w, values(w, { name: "kim" }))).toMatchObject({ name: "kim" })
    expect(buildWorkerUpdate(w, values(w, { name: "kim" }))).not.toHaveProperty("work_dir")
    const moved = buildWorkerUpdate(w, values(w, { workDir: "  /srv/sam  " }))
    expect(moved).toMatchObject({ work_dir: "/srv/sam" })
    expect(moved).not.toHaveProperty("name")
  })

  it("detects engine argument edits and ignores blank ones", () => {
    const w = worker({ engine_args: { pi: "--model a" } })
    expect(buildWorkerUpdate(w, values(w, { engineArgs: { pi: "--model b" } }))).toMatchObject({
      engine_args: { pi: "--model b" },
    })
    expect(buildWorkerUpdate(w, values(w, { engineArgs: { pi: "--model a", claude: "  " } }))).toBeNull()
  })
})

describe("followedEngine", () => {
  it("returns the selected engine when one is set", () => {
    expect(followedEngine("claude", "pi", ["claude", "pi"])).toBe("claude")
    expect(followedEngine("codex", "pi", ["pi"])).toBe("codex")
  })

  it("follows the system default for an unset engine when it is installed", () => {
    expect(followedEngine("", "pi", ["claude", "pi"])).toBe("pi")
  })

  it("is empty when the system default is unset or not installed", () => {
    expect(followedEngine("", undefined, ["claude", "pi"])).toBe("")
    expect(followedEngine("", "", ["claude", "pi"])).toBe("")
    expect(followedEngine("", "codex", ["claude", "pi"])).toBe("")
    expect(followedEngine("", "pi", [])).toBe("")
  })
})
