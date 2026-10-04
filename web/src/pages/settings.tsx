import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { LoaderCircle } from "lucide-react"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { Panel } from "@/components/panel"
import {
  Select,
  SelectContent,
  SelectTrigger,
} from "@/components/ui/select"
import { Button } from "@/components/ui/button"
import { EngineSelectItems, EngineSelectValue } from "@/components/engine-select-items"
import { EngineArgsSection } from "@/components/engine-args-section"
import { useEnabledEngines } from "@/hooks/use-config"
import { Can } from "@/components/guard"
import { useCan } from "@/hooks/use-can"
import { Perm } from "@/lib/permissions"
import { ALERT_INFO } from "@/lib/styles"
import { cn } from "@/lib/utils"
import { api } from "@/lib/api"
import { engineArgsEqual, parseEngineArgs, stripEmptyEngineArgs } from "@/lib/engine-args"
import {
  SYSTEM_CONFIG_KEY_DEFAULT_ENGINE,
  SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL,
  SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE,
} from "@/lib/types"

interface EngineArgsConfigSectionProps {
  configKey: string
  savedValue: Record<string, string>
  title: string
  successMessage: string
}

function EngineArgsConfigSection({
  configKey,
  savedValue,
  title,
  successMessage,
}: EngineArgsConfigSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const enabledEngines = useEnabledEngines()
  const [pendingValue, setPendingValue] = useState<Record<string, string> | null>(null)
  const value = pendingValue ?? savedValue

  const { mutate: save, isPending } = useMutation({
    mutationFn: (v: Record<string, string>) =>
      api.systemConfigs.set(configKey, JSON.stringify(stripEmptyEngineArgs(v))),
    onError: () => setPendingValue(null),
    onSuccess: () => {
      setPendingValue(null)
      queryClient.invalidateQueries({ queryKey: ["system-configs"] })
      toast.success(successMessage)
    },
  })

  const isDirty =
    pendingValue !== null && !engineArgsEqual(stripEmptyEngineArgs(pendingValue), savedValue)

  return (
    <Panel title={title} flush>
      <div className="p-4">
        <EngineArgsSection
          engines={enabledEngines}
          value={value}
          onChange={setPendingValue}
          showLabel={false}
        />
      </div>
      <SettingsSaveFooter onSave={() => save(value)} disabled={!isDirty} saving={isPending} />
    </Panel>
  )
}

// Footer strip of a settings Panel: a gray bar under the white body that holds
// the section's save action bottom-right (Kumo LayerCard footer). This is the
// single system_config:write gate for saving: read-only viewers get neither the
// bar nor the button. Pass `saving` while the mutation is in flight to show a
// spinner and hold the button disabled.
function SettingsSaveFooter({
  onSave,
  disabled,
  saving,
}: {
  onSave: () => void
  disabled: boolean
  saving: boolean
}) {
  const { t } = useTranslation()
  return (
    <Can perm={Perm.SystemConfigWrite}>
      <div className="flex items-center justify-end gap-2 rounded-b-sm border-t border-border bg-elevated px-4 py-2.5">
        <Button onClick={onSave} disabled={disabled || saving} aria-busy={saving || undefined}>
          {saving && (
            <LoaderCircle aria-hidden="true" className="size-4 animate-spin motion-reduce:animate-none" />
          )}
          {t("common.save")}
        </Button>
      </div>
    </Can>
  )
}

export function SystemSettings() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const enabledEngines = useEnabledEngines()
  const canWrite = useCan(Perm.SystemConfigWrite)

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

  const globalArgsRaw = sysConfigs?.[SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL]
  const beeArgsRaw = sysConfigs?.[SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE]
  const savedGlobalArgs = useMemo(() => parseEngineArgs(globalArgsRaw), [globalArgsRaw])
  const savedBeeArgs = useMemo(() => parseEngineArgs(beeArgsRaw), [beeArgsRaw])

  return (
    <FadeIn>
      <div className="w-full max-w-3xl">
        <PageHeader title={t("systemSettings.title")} />

        {!canWrite && (
          <div role="status" className={cn(ALERT_INFO, "mb-6")}>
            {t("systemSettings.readonlyHint")}
          </div>
        )}

        <fieldset disabled={!canWrite} className="m-0 min-w-0 space-y-6 border-0 p-0">
          <Panel title={t("systemSettings.engineSection.title")} flush>
            <div className="p-4">
              <Select
                value={engine}
                onValueChange={setPendingEngine}
                disabled={isPending}
              >
                <SelectTrigger
                  className="w-full sm:w-64"
                  aria-label={t("systemSettings.engineSection.title")}
                >
                  <EngineSelectValue />
                </SelectTrigger>
                <SelectContent>
                  <EngineSelectItems engines={enabledEngines} />
                </SelectContent>
              </Select>
            </div>
            <SettingsSaveFooter
              onSave={() => saveEngine(engine)}
              disabled={pendingEngine === null || pendingEngine === savedEngine}
              saving={isPending}
            />
          </Panel>

          <EngineArgsConfigSection
            configKey={SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL}
            savedValue={savedGlobalArgs}
            title={t("systemSettings.globalArgsSection.title")}
            successMessage={t("systemSettings.globalArgsSection.updated")}
          />

          <EngineArgsConfigSection
            configKey={SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE}
            savedValue={savedBeeArgs}
            title={t("systemSettings.beeArgsSection.title")}
            successMessage={t("systemSettings.beeArgsSection.updated")}
          />
        </fieldset>
      </div>
    </FadeIn>
  )
}
