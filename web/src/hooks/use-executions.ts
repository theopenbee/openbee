import { useQuery, keepPreviousData } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useExecutions(page: number = 1, pageSize: number = 20) {
  return useQuery({
    queryKey: ["executions", page, pageSize],
    queryFn: () => api.executions.list(page, pageSize),
    placeholderData: keepPreviousData,
    refetchInterval: 500,
  })
}

export function useSessionExecutions(sessionId: string) {
  return useQuery({
    queryKey: ["sessions", sessionId, "executions"],
    queryFn: () => api.sessions.executions(sessionId),
    enabled: !!sessionId,
    refetchInterval: (query) => {
      const executions = query.state.data ?? []
      const hasActive = executions.some(
        (e) => e.status === "running" || e.status === "pending"
      )
      return hasActive ? 500 : false
    },
  })
}
