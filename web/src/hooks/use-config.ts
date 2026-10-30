import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { listEnabledEngines } from "@/lib/engines"
import type { EngineChoice } from "@/lib/types"

const CONFIG_STALE_MS = 30_000

export function useSystemConfigs() {
  return useQuery({
    queryKey: ["system-configs"],
    queryFn: () => api.systemConfigs.get(),
  })
}

export function useAppConfig() {
  return useQuery({
    queryKey: ["config"],
    queryFn: () => api.config.get(),
    staleTime: CONFIG_STALE_MS,
  })
}

export function useEnabledEngines(): EngineChoice[] {
  const { data } = useAppConfig()
  const { i18n } = useTranslation()
  const lang = i18n.language

  return useMemo(() => listEnabledEngines(data, lang), [data, lang])
}
