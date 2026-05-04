import { useMemo, useState } from "react"
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
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { EngineSelectItems } from "@/components/engine-select-items"
import { EngineArgsSection } from "@/components/engine-args-section"
import { useEnabledEngines } from "@/hooks/use-config"
import { api } from "@/lib/api"
import { engineArgsEqual, parseEngineArgs, stripEmptyEngineArgs } from "@/lib/engine-args"
import {
  SYSTEM_CONFIG_KEY_DEFAULT_ENGINE,
  SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL,
  SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE,
  SYSTEM_CONFIG_KEY_LINEAR_PROJECTS,
} from "@/lib/types"
import { XIcon } from "lucide-react"

function parseLinearProjects(raw: string | undefined): string[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.filter((p): p is string => typeof p === "string" && p.length > 0) : []
  } catch {
    return []
  }
}

function LinearProjectsSection({ savedValue }: { savedValue: string[] }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [pending, setPending] = useState<string[] | null>(null)
  const [draft, setDraft] = useState("")
  const value = pending ?? savedValue

  const { mutate: save, isPending } = useMutation({
    mutationFn: (v: string[]) =>
      api.systemConfigs.set(SYSTEM_CONFIG_KEY_LINEAR_PROJECTS, JSON.stringify(v)),
    onError: () => setPending(null),
    onSuccess: () => {
      setPending(null)
      queryClient.invalidateQueries({ queryKey: ["system-configs"] })
      toast.success(t("systemSettings.linearProjectsSection.updated"))
    },
  })

  const isDirty =
    pending !== null &&
    (pending.length !== savedValue.length ||
      pending.some((p, i) => p !== savedValue[i]))

  const addDraft = () => {
    const trimmed = draft.trim()
    if (!trimmed || value.includes(trimmed)) {
      setDraft("")
      return
    }
    setPending([...value, trimmed])
    setDraft("")
  }

  const remove = (idx: number) => {
    setPending(value.filter((_, i) => i !== idx))
  }

  return (
    <DetailSection className="p-5 sm:p-6 space-y-4">
      <div>
        <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
          {t("systemSettings.linearProjectsSection.title")}
        </p>
        <p className="mt-1 text-sm leading-6 text-muted-foreground">
          {t("systemSettings.linearProjectsSection.hint")}
        </p>
      </div>
      {value.length === 0 ? (
        <p className="text-sm text-muted-foreground italic">
          {t("systemSettings.linearProjectsSection.empty")}
        </p>
      ) : (
        <div className="flex flex-wrap gap-2">
          {value.map((p, i) => (
            <Badge key={`${p}-${i}`} variant="secondary" className="gap-1.5 pr-1">
              <span>{p}</span>
              <button
                type="button"
                onClick={() => remove(i)}
                className="rounded-sm hover:bg-muted-foreground/10 p-0.5"
                aria-label={`remove ${p}`}
              >
                <XIcon className="size-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}
      <div className="flex items-center gap-2">
        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault()
              addDraft()
            }
          }}
          placeholder={t("systemSettings.linearProjectsSection.addPlaceholder")}
          className="max-w-xs"
        />
        <Button type="button" variant="outline" onClick={addDraft} disabled={!draft.trim()}>
          {t("systemSettings.linearProjectsSection.add")}
        </Button>
      </div>
      <Button onClick={() => save(value)} disabled={isPending || !isDirty}>
        {t("common.save")}
      </Button>
    </DetailSection>
  )
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
      <Button onClick={() => save(value)} disabled={isPending || !isDirty}>
        {t("common.save")}
      </Button>
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

  const globalArgsRaw = sysConfigs?.[SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL]
  const beeArgsRaw = sysConfigs?.[SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE]
  const linearProjectsRaw = sysConfigs?.[SYSTEM_CONFIG_KEY_LINEAR_PROJECTS]
  const savedGlobalArgs = useMemo(() => parseEngineArgs(globalArgsRaw), [globalArgsRaw])
  const savedBeeArgs = useMemo(() => parseEngineArgs(beeArgsRaw), [beeArgsRaw])
  const savedLinearProjects = useMemo(() => parseLinearProjects(linearProjectsRaw), [linearProjectsRaw])

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

        <LinearProjectsSection savedValue={savedLinearProjects} />
      </div>
    </FadeIn>
  )
}
