import { useState } from "react"
import { useTranslation } from "react-i18next"
import { useQuery, useMutation } from "@tanstack/react-query"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { DetailSection } from "@/components/detail-primitives"
import {
  Select,
  SelectContent,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Button } from "@/components/ui/button"
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

  const savedEngine = sysConfigs?.[SYSTEM_CONFIG_KEY_DEFAULT_ENGINE] ?? ""
  const [pendingEngine, setPendingEngine] = useState<string | null>(null)
  const engine = pendingEngine ?? savedEngine

  const { mutate: saveEngine, isPending } = useMutation({
    mutationFn: (value: string) => api.systemConfigs.set(SYSTEM_CONFIG_KEY_DEFAULT_ENGINE, value),
    onError: () => setPendingEngine(null),
    onSuccess: () => setPendingEngine(null),
  })

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
          <div className="flex items-center gap-3">
            <Select
              value={engine}
              onValueChange={setPendingEngine}
              disabled={isPending}
            >
              <SelectTrigger className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <EngineSelectItems engines={enabledEngines} />
              </SelectContent>
            </Select>
            <Button
              onClick={() => saveEngine(engine)}
              disabled={isPending || pendingEngine === null || pendingEngine === savedEngine}
            >
              {t("common.save")}
            </Button>
          </div>
        </DetailSection>
      </div>
    </FadeIn>
  )
}
