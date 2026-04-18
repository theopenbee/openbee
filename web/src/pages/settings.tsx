import { useState } from "react"
import { useTranslation } from "react-i18next"
import { useQuery, useMutation } from "@tanstack/react-query"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { DetailSection } from "@/components/detail-primitives"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { EngineSelectItems } from "@/components/engine-select-items"
import { useEnabledEngines } from "@/hooks/use-config"
import { api } from "@/lib/api"
import { SYSTEM_CONFIG_KEY_DEFAULT_ENGINE } from "@/lib/types"

export function SystemSettings() {
  const { t } = useTranslation()
  const enabledEngines = useEnabledEngines()

  const { data: sysConfigs } = useQuery({
    queryKey: ["system-configs"],
    queryFn: () => api.systemConfigs.get(),
  })

  const [optimisticEngine, setOptimisticEngine] = useState<string | null>(null)
  const engine = optimisticEngine ?? sysConfigs?.[SYSTEM_CONFIG_KEY_DEFAULT_ENGINE] ?? ""

  const { mutate: saveEngine, isPending } = useMutation({
    mutationFn: (value: string) => api.systemConfigs.set(SYSTEM_CONFIG_KEY_DEFAULT_ENGINE, value),
    onError: () => setOptimisticEngine(null),
    onSuccess: () => setOptimisticEngine(null),
  })

  function handleEngineChange(value: string) {
    setOptimisticEngine(value)
    saveEngine(value)
  }

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader title={t("systemSettings.title")} />

        <DetailSection className="p-5 sm:p-6 space-y-4">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
              {t("systemSettings.engineSection.title")}
            </p>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              {t("systemSettings.engineSection.hint")}
            </p>
          </div>
          <Select
            value={engine}
            onValueChange={handleEngineChange}
            disabled={isPending}
          >
            <SelectTrigger className="w-48">
              <SelectValue placeholder={t("systemSettings.engineSection.systemDefault")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">
                {t("systemSettings.engineSection.systemDefault")}
              </SelectItem>
              <EngineSelectItems engines={enabledEngines} />
            </SelectContent>
          </Select>
        </DetailSection>
      </div>
    </FadeIn>
  )
}
