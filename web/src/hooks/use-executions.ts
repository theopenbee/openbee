import { useQuery, keepPreviousData } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { isActiveStatus } from "@/lib/format"

export function useExecutions(page: number = 1, pageSize: number = 20) {
  return useQuery({
    queryKey: ["executions", page, pageSize],
    queryFn: () => api.executions.list(page, pageSize),
    placeholderData: keepPreviousData,
    refetchInterval: (query) => {
      const items = query.state.data?.items ?? []
      return items.some((e) => isActiveStatus(e.status)) ? 500 : false
    },
  })
}

export function useSessionExecutions(sessionId: string) {
  return useQuery({
    queryKey: ["sessions", sessionId, "executions"],
    queryFn: () => api.sessions.executions(sessionId),
    enabled: !!sessionId,
    refetchInterval: (query) => {
      const executions = query.state.data ?? []
      return executions.some((e) => isActiveStatus(e.status)) ? 500 : false
    },
  })
}
