import { useQuery } from "@tanstack/react-query"
import { api, ApiError } from "@/lib/api"
import { EXECUTION_POLL_MS } from "@/lib/execution"

export function useExecution(id: string) {
  return useQuery({
    queryKey: ["executions", id],
    queryFn: () => api.executions.get(id),
    refetchInterval: (query) => (query.state.data?.status === "running" ? EXECUTION_POLL_MS : false),
    retry: (failures, error) => !(error instanceof ApiError && error.status === 404) && failures < 1,
  })
}
