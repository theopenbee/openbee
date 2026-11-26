import { describe, expect, it } from "vitest"
import { buildWorkerUpdate, followedEngine, profileEngine, type WorkerEditValues } from "../worker-edit"
import type { Worker } from "../types"

const worker = (over: Partial<Worker>): Worker => ({
  id: "w1",
  name: "sam",
  description: "writes code",
  constraints: "",
  work_dir: "/tmp/sam",
  engine: "pi",
  engine_args: {},
  engine_models: {},
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
  engineModels: w.engine_models ?? {},
  avatar: w.avatar ?? "",
  ...over,
})

describe("buildWorkerUpdate", () => {
  it("returns null when nothing changed", () => {
    const w = worker({})
    expect(buildWorkerUpdate(w, values(w))).toBeNull()
  })

  it("sends only a changed avatar", () => {
    const w = worker({ avatar: "mfers/00001" })
    expect(buildWorkerUpdate(w, values(w))).toBeNull()
    expect(buildWorkerUpdate(w, values(w, { avatar: "mfers/00002" }))).toMatchObject({ avatar: "mfers/00002" })
    expect(buildWorkerUpdate(w, values(w, { avatar: "" }))).toMatchObject({ avatar: "" })
    expect(buildWorkerUpdate(w, values(w, { description: "x" }))).not.toHaveProperty("avatar")
  })

  it("treats a missing avatar as none", () => {
    const w = worker({ avatar: undefined })
    expect(buildWorkerUpdate(w, values(w, { avatar: "" }))).toBeNull()
    expect(buildWorkerUpdate(w, values(w, { avatar: "mfers/00003" }))).toMatchObject({ avatar: "mfers/00003" })
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

describe("buildWorkerUpdate engine_models", () => {
  it("leaves engine_models out when no model changed", () => {
    const w = worker({ engine_models: { pi: "deepseek/deepseek-flash" } })
    expect(buildWorkerUpdate(w, values(w, { description: "edited" }))).not.toHaveProperty("engine_models")
  })

  it("treats a worker without engine_models like an empty map", () => {
    const w = worker({ engine_models: undefined })
    expect(buildWorkerUpdate(w, values(w))).toBeNull()
    expect(buildWorkerUpdate(w, values(w, { engineModels: { pi: "  " } }))).toBeNull()
  })

  it("sends only the engine whose model changed", () => {
    const w = worker({ engine_models: { pi: "a/b", claude: "opus" } })
    const data = buildWorkerUpdate(w, values(w, { engineModels: { pi: "a/c", claude: "opus" } }))
    expect(data).toMatchObject({ engine_models: { pi: "a/c" } })
    expect(data?.engine_models).not.toHaveProperty("claude")
  })

  it("sends an empty string to remove a cleared model", () => {
    const w = worker({ engine_models: { pi: "a/b" } })
    expect(buildWorkerUpdate(w, values(w, { engineModels: { pi: "" } }))).toMatchObject({ engine_models: { pi: "" } })
    expect(buildWorkerUpdate(w, values(w, { engineModels: { pi: "   " } }))).toMatchObject({ engine_models: { pi: "" } })
  })

  it("trims a typed model and ignores whitespace-only edits", () => {
    const w = worker({ engine_models: { pi: "a/b" } })
    expect(buildWorkerUpdate(w, values(w, { engineModels: { pi: " a/b " } }))).toBeNull()
    expect(buildWorkerUpdate(w, values(w, { engineModels: { pi: " a/c\t" } }))).toMatchObject({
      engine_models: { pi: "a/c" },
    })
  })

  it("does not revert models another client changed while the form was open", () => {
    const opened = { pi: "a/b", claude: "opus" }
    const refetched = worker({ engine_models: { pi: "a/b", claude: "sonnet" } })
    expect(buildWorkerUpdate(refetched, values(refetched, { engineModels: opened }), opened)).toBeNull()
    const data = buildWorkerUpdate(refetched, values(refetched, { engineModels: { ...opened, pi: "a/c" } }), opened)
    expect(data).toMatchObject({ engine_models: { pi: "a/c" } })
    expect(data?.engine_models).not.toHaveProperty("claude")
  })

  it("sends a model the user changed even when the live worker moved on", () => {
    const opened = { pi: "a/b" }
    const refetched = worker({ engine_models: { pi: "x/y" } })
    expect(buildWorkerUpdate(refetched, values(refetched, { engineModels: { pi: "a/c" } }), opened)).toMatchObject({
      engine_models: { pi: "a/c" },
    })
    expect(buildWorkerUpdate(refetched, values(refetched, { engineModels: { pi: "" } }), opened)).toMatchObject({
      engine_models: { pi: "" },
    })
  })

  it("adds a model for a new engine without touching the others", () => {
    const w = worker({ engine: "", engine_models: { claude: "opus" } })
    expect(buildWorkerUpdate(w, values(w, { engineModels: { claude: "opus", pi: "x/y" } }))).toEqual({
      description: "writes code",
      engine_args: {},
      engine_models: { pi: "x/y" },
    })
  })
})

describe("profileEngine", () => {
  it("shows the worker engine with its model", () => {
    expect(profileEngine(worker({ engine: "pi", engine_models: { pi: "a/b" } }), "claude", ["pi", "claude"])).toEqual({
      engine: "pi",
      model: "a/b",
    })
  })

  it("shows the worker engine without a model when none is set for it", () => {
    expect(profileEngine(worker({ engine: "pi", engine_models: { claude: "opus" } }), "claude", ["pi"])).toEqual({
      engine: "pi",
      model: "",
    })
  })

  it("shows the system default engine for an unset engine only when it has a model", () => {
    const followed = worker({ engine: "", engine_models: { claude: "opus" } })
    expect(profileEngine(followed, "claude", ["pi", "claude"])).toEqual({ engine: "claude", model: "opus" })
    expect(profileEngine(worker({ engine: "" }), "claude", ["pi", "claude"])).toBeNull()
    expect(profileEngine(followed, "codex", ["pi", "claude"])).toBeNull()
  })

  it("falls back to the default model when the worker has none for the engine", () => {
    const defaults = { pi: "deepseek/deepseek-flash", claude: "sonnet" }
    expect(profileEngine(worker({ engine: "pi" }), "claude", ["pi", "claude"], defaults)).toEqual({
      engine: "pi",
      model: "deepseek/deepseek-flash",
    })
    expect(profileEngine(worker({ engine: "pi", engine_models: { pi: "a/b" } }), "claude", ["pi"], defaults)).toEqual({
      engine: "pi",
      model: "a/b",
    })
    expect(profileEngine(worker({ engine: "" }), "claude", ["pi", "claude"], defaults)).toEqual({
      engine: "claude",
      model: "sonnet",
    })
    expect(profileEngine(worker({ engine: "" }), "codex", ["pi", "claude"], defaults)).toBeNull()
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
