import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { isActiveStatus } from "@/lib/format"

export function useSessionDetail(sessionId: string) {
  return useQuery({
    queryKey: ["sessions", sessionId],
    queryFn: () => api.sessions.get(sessionId),
    enabled: !!sessionId,
    refetchInterval: (query) => {
      const executions = query.state.data?.executions ?? []
      return executions.some((e) => isActiveStatus(e.status)) ? 500 : false
    },
  })
}
