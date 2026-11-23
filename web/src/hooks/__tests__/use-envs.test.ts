import { QueryClient } from "@tanstack/react-query"
import { describe, expect, it } from "vitest"
import { ENGINE_MODELS_QUERY_KEY } from "../use-engine-models"
import { invalidateEnvs } from "../use-envs"

const invalidated = (client: QueryClient, key: unknown[]) => client.getQueryState(key)?.isInvalidated ?? false

describe("invalidateEnvs", () => {
  it("marks every cached model list stale along with the changed env scope", () => {
    const client = new QueryClient()
    const models = [ENGINE_MODELS_QUERY_KEY, "opencode", ""]
    const workerModels = [ENGINE_MODELS_QUERY_KEY, "pi", "w1"]
    const scoped = ["envs", "worker", "w1"]
    const other = ["envs", "global", null]
    for (const key of [models, workerModels, scoped, other]) client.setQueryData(key, [])

    invalidateEnvs(client, "worker", "w1")

    expect(invalidated(client, models)).toBe(true)
    expect(invalidated(client, workerModels)).toBe(true)
    expect(invalidated(client, scoped)).toBe(true)
    expect(invalidated(client, other)).toBe(false)
  })
})
