import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import type { ModelListStatus } from "@/lib/engine-models"
import type { Engine, EngineModel } from "@/lib/types"

export const ENGINE_MODELS_QUERY_KEY = "engine-models"

const MODELS_STALE_MS = 5 * 60_000
const NO_MODELS: EngineModel[] = []

export function useEngineModels(engine: Engine, workerId: string | undefined, enabled: boolean) {
  const query = useQuery({
    queryKey: [ENGINE_MODELS_QUERY_KEY, engine, workerId ?? ""],
    queryFn: () => api.engines.models(engine, workerId),
    enabled: enabled && engine !== "",
    staleTime: MODELS_STALE_MS,
    retry: false,
  })
  const status: ModelListStatus = query.isError ? "error" : query.data ? "ready" : "loading"
  return { models: query.data ?? NO_MODELS, status }
}
