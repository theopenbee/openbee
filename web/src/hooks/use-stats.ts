import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useStatsOverview() {
  return useQuery({
    queryKey: ["stats", "overview"],
    queryFn: () => api.stats.overview(),
    refetchInterval: 30_000,
  })
}

export function useStatsTrend(days: 7 | 15 | 30) {
  return useQuery({
    queryKey: ["stats", "trend", days],
    queryFn: () => api.stats.trend(days),
    staleTime: 60_000,
  })
}
