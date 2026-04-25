import { useState, useMemo } from "react"
import { useTranslation } from "react-i18next"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
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
import { EngineArgsSection } from "@/components/engine-args-section"
import { useEnabledEngines } from "@/hooks/use-config"
import { api } from "@/lib/api"
import {
  SYSTEM_CONFIG_KEY_DEFAULT_ENGINE,
  SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL,
  SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE,
} from "@/lib/types"

function recordsEqual(a: Record<string, string>, b: Record<string, string>): boolean {
  const keysA = Object.keys(a)
  return keysA.length === Object.keys(b).length && keysA.every((k) => a[k] === b[k])
}

function stripEmptyArgs(v: Record<string, string>): Record<string, string> {
  const result: Record<string, string> = {}
  for (const [k, val] of Object.entries(v)) {
    if (val.trim() !== "") result[k] = val
  }
  return result
}

interface EngineArgsConfigSectionProps {
  configKey: string
  savedValue: Record<string, string>
  title: string
  hint: string
  successMessage: string
}

function EngineArgsConfigSection({
  configKey,
  savedValue,
  title,
  hint,
  successMessage,
}: EngineArgsConfigSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const enabledEngines = useEnabledEngines()
  const [pendingValue, setPendingValue] = useState<Record<string, string> | null>(null)
  const value = pendingValue ?? savedValue

  const { mutate: save, isPending } = useMutation({
    mutationFn: (v: Record<string, string>) =>
      api.systemConfigs.set(configKey, JSON.stringify(stripEmptyArgs(v))),
    onError: () => setPendingValue(null),
    onSuccess: () => {
      setPendingValue(null)
      queryClient.invalidateQueries({ queryKey: ["system-configs"] })
      toast.success(successMessage)
    },
  })

  const isDirty =
    pendingValue !== null && !recordsEqual(stripEmptyArgs(pendingValue), savedValue)

  return (
    <DetailSection className="p-5 sm:p-6 space-y-4">
      <div>
        <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
          {title}
        </p>
        <p className="mt-1 text-sm leading-6 text-muted-foreground">{hint}</p>
      </div>
      <EngineArgsSection
        engines={enabledEngines}
        value={value}
        onChange={setPendingValue}
      />
      <div>
        <Button onClick={() => save(value)} disabled={isPending || !isDirty}>
          {t("common.save")}
        </Button>
      </div>
    </DetailSection>
  )
}

export function SystemSettings() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const enabledEngines = useEnabledEngines()

  const { data: sysConfigs } = useQuery({
    queryKey: ["system-configs"],
    queryFn: () => api.systemConfigs.get(),
  })

  const savedEngine = sysConfigs?.[SYSTEM_CONFIG_KEY_DEFAULT_ENGINE] ?? ""
  const [pendingEngine, setPendingEngine] = useState<string | null>(null)
  const engine = pendingEngine ?? savedEngine

  const { mutate: saveEngine, isPending } = useMutation({
    mutationFn: (value: string) =>
      api.systemConfigs.set(SYSTEM_CONFIG_KEY_DEFAULT_ENGINE, value),
    onError: () => setPendingEngine(null),
    onSuccess: () => {
      setPendingEngine(null)
      queryClient.invalidateQueries({ queryKey: ["system-configs"] })
      toast.success(t("systemSettings.updated"))
    },
  })

  function parseEngineArgs(key: string): Record<string, string> {
    const raw = sysConfigs?.[key]
    if (!raw) return {}
    try {
      const parsed: unknown = JSON.parse(raw)
      if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return {}
      return parsed as Record<string, string>
    } catch {
      return {}
    }
  }

  const savedGlobalArgs = useMemo(
    () => parseEngineArgs(SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sysConfigs],
  )
  const savedBeeArgs = useMemo(
    () => parseEngineArgs(SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sysConfigs],
  )

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
              disabled={
                isPending ||
                pendingEngine === null ||
                pendingEngine === savedEngine
              }
            >
              {t("common.save")}
            </Button>
          </div>
        </DetailSection>

        <EngineArgsConfigSection
          configKey={SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL}
          savedValue={savedGlobalArgs}
          title={t("systemSettings.globalArgsSection.title")}
          hint={t("systemSettings.globalArgsSection.hint")}
          successMessage={t("systemSettings.globalArgsSection.updated")}
        />

        <EngineArgsConfigSection
          configKey={SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE}
          savedValue={savedBeeArgs}
          title={t("systemSettings.beeArgsSection.title")}
          hint={t("systemSettings.beeArgsSection.hint")}
          successMessage={t("systemSettings.beeArgsSection.updated")}
        />
      </div>
    </FadeIn>
  )
}
