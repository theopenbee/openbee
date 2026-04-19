import { useMemo } from "react"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { ENGINES } from "@/lib/types"
import type { Engine } from "@/lib/types"

function useAppConfig() {
  return useQuery({
    queryKey: ["config"],
    queryFn: () => api.config.get(),
    staleTime: Infinity, // static deployment setting
  })
}

export function useEnabledEngines(): readonly Engine[] {
  const { data } = useAppConfig()

  return useMemo(() => {
    if (!data?.enabled_engines?.length) {
      return ENGINES
    }
    // Preserve canonical ordering from ENGINES constant
    return ENGINES.filter((e) => data.enabled_engines.includes(e))
  }, [data?.enabled_engines])
}

