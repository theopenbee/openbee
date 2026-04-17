import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { ENGINES } from "@/lib/types"
import type { Engine } from "@/lib/types"

export function useEnabledEngines(): Engine[] {
  const { data } = useQuery({
    queryKey: ["config"],
    queryFn: () => api.config.get(),
    staleTime: Infinity, // config doesn't change at runtime
  })

  if (!data?.enabled_engines?.length) {
    // Fall back to all engines if config not yet loaded or empty
    return [...ENGINES]
  }

  // Preserve canonical ordering from ENGINES constant
  return ENGINES.filter((e) => data.enabled_engines.includes(e))
}
