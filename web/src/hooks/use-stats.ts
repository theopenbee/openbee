import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useStatsOverview() {
  return useQuery({
    queryKey: ["stats", "overview"],
    queryFn: () => api.stats.overview(),
    refetchInterval: 30_000,
  })
}

function useStatsDayTrend<T>(key: string, fetcher: (days: 7 | 15 | 30) => Promise<T>, days: 7 | 15 | 30) {
  return useQuery({ queryKey: ["stats", key, days], queryFn: () => fetcher(days), staleTime: 60_000 })
}

export const useStatsTrend = (days: 7 | 15 | 30) => useStatsDayTrend("trend", api.stats.trend, days)
export const useExecutionDurationTrend = (days: 7 | 15 | 30) => useStatsDayTrend("execution-duration-trend", api.stats.executionDurationTrend, days)
