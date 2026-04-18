import { useEffect, useState } from "react"
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

const DEFAULT_ENGINE_KEY = "default_engine"

export function SystemSettings() {
  const { t } = useTranslation()
  const enabledEngines = useEnabledEngines()

  const { data: sysConfigs } = useQuery({
    queryKey: ["system-configs"],
    queryFn: () => api.systemConfigs.get(),
  })

  const [engine, setEngine] = useState<string>("")

  useEffect(() => {
    if (sysConfigs !== undefined) {
      setEngine(sysConfigs[DEFAULT_ENGINE_KEY] ?? "")
    }
  }, [sysConfigs])

  const { mutate: saveEngine, isPending } = useMutation({
    mutationFn: (value: string) => api.systemConfigs.set(DEFAULT_ENGINE_KEY, value),
    onError: () => {
      setEngine(sysConfigs?.[DEFAULT_ENGINE_KEY] ?? "")
    },
  })

  function handleEngineChange(value: string) {
    setEngine(value)
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
