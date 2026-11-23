import { queryOptions, useQuery } from "@tanstack/react-query"
import { api, ApiError } from "@/lib/api"
import { EXECUTION_POLL_MS } from "@/lib/execution"

export function executionGone(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404
}

export function executionQuery(id: string) {
  return queryOptions({
    queryKey: ["executions", id],
    queryFn: () => api.executions.get(id),
    refetchInterval: (query) =>
      query.state.data?.status === "running" && !executionGone(query.state.error) ? EXECUTION_POLL_MS : false,
    retry: (failures, error) => !executionGone(error) && failures < 1,
  })
}

export function useExecution(id: string) {
  return useQuery(executionQuery(id))
}
