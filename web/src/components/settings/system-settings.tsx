import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { useQueryClient } from "@tanstack/react-query"
import { Check } from "lucide-react"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { FadeIn } from "@/components/fade-in"
import { EngineSelect } from "@/components/engine-select-items"
import { EngineArgsSection } from "@/components/engine-args-section"
import { useEnabledEngines, useSystemConfigs } from "@/hooks/use-config"
import { api } from "@/lib/api"
import { getErrorMessage } from "@/lib/utils"
import { engineArgsEqual, parseEngineArgs, stripEmptyEngineArgs } from "@/lib/engine-args"
import {
  SYSTEM_CONFIG_KEY_DEFAULT_ENGINE,
  SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL,
  SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE,
} from "@/lib/types"

const SAVED_RESET_MS = 2400

type SaveStatus = "idle" | "saving" | "saved" | "error"

function SettingsRow({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <div className="grid gap-4 px-4 py-5 @lg:px-6 @2xl:grid-cols-[minmax(0,14rem)_minmax(0,1fr)] @2xl:gap-8">
      <div className="grid content-start gap-1">
        <h3 className="text-sm font-medium text-foreground">{title}</h3>
        <p className="text-sm text-secondary">{description}</p>
      </div>
      <div className="min-w-0">{children}</div>
    </div>
  )
}

function SettingsGroup({ children }: { children: ReactNode }) {
  return (
    <div className="divide-y divide-border-subtle overflow-hidden rounded-inset border border-border bg-surface">
      {children}
    </div>
  )
}

function SettingsSkeleton() {
  return (
    <div className="grid gap-10" aria-busy="true">
      {[1, 2].map((group) => (
        <div key={group} className="grid gap-4">
          <div className="skeleton h-5 w-32" />
          <div className="divide-y divide-border-subtle rounded-inset border border-border bg-surface">
            {Array.from({ length: group }).map((_, row) => (
              <div key={row} className="grid gap-4 px-6 py-5 @2xl:grid-cols-[14rem_1fr] @2xl:gap-8">
                <div className="grid content-start gap-2">
                  <div className="skeleton h-4 w-24" />
                  <div className="skeleton h-4 w-40" />
                </div>
                <div className="skeleton h-11 w-full rounded-control" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

export function SystemSettings() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const enabledEngines = useEnabledEngines()

  const { data: sysConfigs, error: loadError, isLoading, refetch } = useSystemConfigs()

  const savedEngine = sysConfigs?.[SYSTEM_CONFIG_KEY_DEFAULT_ENGINE] ?? ""
  const globalArgsRaw = sysConfigs?.[SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL]
  const beeArgsRaw = sysConfigs?.[SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE]
  const savedGlobalArgs = useMemo(() => parseEngineArgs(globalArgsRaw), [globalArgsRaw])
  const savedBeeArgs = useMemo(() => parseEngineArgs(beeArgsRaw), [beeArgsRaw])

  const [pendingEngine, setPendingEngine] = useState<string | null>(null)
  const [pendingGlobalArgs, setPendingGlobalArgs] = useState<Record<string, string> | null>(null)
  const [pendingBeeArgs, setPendingBeeArgs] = useState<Record<string, string> | null>(null)
  const [status, setStatus] = useState<SaveStatus>("idle")
  const [saveError, setSaveError] = useState("")

  const engine = pendingEngine ?? savedEngine
  const globalArgs = pendingGlobalArgs ?? savedGlobalArgs
  const beeArgs = pendingBeeArgs ?? savedBeeArgs

  const engineDirty = pendingEngine !== null && pendingEngine !== savedEngine
  const globalDirty =
    pendingGlobalArgs !== null && !engineArgsEqual(stripEmptyEngineArgs(pendingGlobalArgs), savedGlobalArgs)
  const beeDirty =
    pendingBeeArgs !== null && !engineArgsEqual(stripEmptyEngineArgs(pendingBeeArgs), savedBeeArgs)
  const dirty = engineDirty || globalDirty || beeDirty
  const saving = status === "saving"

  useEffect(() => {
    if (status !== "saved") return
    const timer = window.setTimeout(() => setStatus("idle"), SAVED_RESET_MS)
    return () => window.clearTimeout(timer)
  }, [status])

  const edit = <T,>(setter: (value: T) => void) => (value: T) => {
    setter(value)
    if (status === "saved" || status === "error") setStatus("idle")
  }

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (!dirty || saving) return
    setStatus("saving")
    setSaveError("")
    const writes: Promise<unknown>[] = []
    if (engineDirty) writes.push(api.systemConfigs.set(SYSTEM_CONFIG_KEY_DEFAULT_ENGINE, engine))
    if (globalDirty) {
      writes.push(
        api.systemConfigs.set(SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL, JSON.stringify(stripEmptyEngineArgs(globalArgs))),
      )
    }
    if (beeDirty) {
      writes.push(
        api.systemConfigs.set(SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE, JSON.stringify(stripEmptyEngineArgs(beeArgs))),
      )
    }
    const results = await Promise.allSettled(writes)
    await queryClient.invalidateQueries({ queryKey: ["system-configs"] })
    const failed = results.find((r): r is PromiseRejectedResult => r.status === "rejected")
    if (failed) {
      setSaveError(getErrorMessage(failed.reason))
      setStatus("error")
      return
    }
    setPendingEngine(null)
    setPendingGlobalArgs(null)
    setPendingBeeArgs(null)
    setStatus("saved")
  }

  return (
    <FadeIn>
      {loadError ? (
        <Alert tone="danger" title={t("systemSettings.loadError")}>
          <span className="grid justify-items-start gap-3">
            <span>{loadError.message}</span>
            <Button variant="secondary" size="sm" onClick={() => refetch()}>
              {t("common.retry")}
            </Button>
          </span>
        </Alert>
      ) : isLoading ? (
        <SettingsSkeleton />
      ) : (
        <form onSubmit={handleSubmit} className="grid gap-10">
          <section className="grid gap-4" aria-labelledby="settings-engine">
            <h2 id="settings-engine" className="text-base font-medium text-foreground">
              {t("systemSettings.engineSection.title")}
            </h2>
            <SettingsGroup>
              <SettingsRow
                title={t("systemSettings.engineSection.rowTitle")}
                description={t("systemSettings.engineSection.description")}
              >
                <EngineSelect
                  label={t("systemSettings.engineSection.label")}
                  engines={enabledEngines}
                  value={engine}
                  onValueChange={edit(setPendingEngine)}
                  disabled={saving}
                />
              </SettingsRow>
            </SettingsGroup>
          </section>

          <section className="grid gap-4" aria-labelledby="settings-args">
            <div className="grid gap-1">
              <h2 id="settings-args" className="text-base font-medium text-foreground">
                {t("systemSettings.argsSection.title")}
              </h2>
              <p className="text-sm text-secondary">{t("systemSettings.argsSection.description")}</p>
            </div>
            <SettingsGroup>
              <SettingsRow
                title={t("systemSettings.globalArgsSection.title")}
                description={t("systemSettings.globalArgsSection.description")}
              >
                <EngineArgsSection
                  engines={enabledEngines}
                  value={globalArgs}
                  onChange={edit(setPendingGlobalArgs)}
                  idPrefix="global-args"
                  disabled={saving}
                />
              </SettingsRow>
              <SettingsRow
                title={t("systemSettings.beeArgsSection.title")}
                description={t("systemSettings.beeArgsSection.description")}
              >
                <EngineArgsSection
                  engines={enabledEngines}
                  value={beeArgs}
                  onChange={edit(setPendingBeeArgs)}
                  idPrefix="bee-args"
                  disabled={saving}
                />
              </SettingsRow>
            </SettingsGroup>
          </section>

          {status === "error" && (
            <Alert tone="danger" title={t("systemSettings.saveError")}>
              {saveError}
            </Alert>
          )}

          <div className="flex justify-end">
            <Button type="submit" loading={saving} disabled={!dirty && status !== "error"}>
              {status === "saved" ? (
                <>
                  <Check size={16} strokeWidth={1.75} aria-hidden="true" />
                  {t("systemSettings.saved")}
                </>
              ) : (
                t("systemSettings.save")
              )}
            </Button>
          </div>
        </form>
      )}
    </FadeIn>
  )
}
