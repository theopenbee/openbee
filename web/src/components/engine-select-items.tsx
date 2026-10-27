import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import { formatEngineLabel } from "@/lib/format"
import { ENGINES } from "@/lib/types"
import type { Engine } from "@/lib/types"

export interface EngineOption {
  value: Engine
  label: string
}

export function useEngineOptions(engines: readonly Engine[] = ENGINES): EngineOption[] {
  const { t } = useTranslation()
  return useMemo(
    () => engines.map((engine) => ({ value: engine, label: formatEngineLabel(engine, t) })),
    [engines, t],
  )
}
