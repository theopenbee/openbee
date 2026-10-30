import { useEffect, useId, useMemo, useRef, useState, type FormEvent, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { ChevronDown } from "lucide-react"
import { Alert } from "@/components/arc/alert/alert"
import { Badge } from "@/components/arc/badge/badge"
import { Button } from "@/components/arc/button/button"
import { Checkbox } from "@/components/arc/checkbox/checkbox"
import { Input } from "@/components/arc/input/input"
import { Textarea } from "@/components/arc/textarea/textarea"
import { EngineArgsSection } from "@/components/engine-args-section"
import { EngineSelect } from "@/components/engine-select-items"
import { SectionHeading } from "@/components/section-heading"
import { WorkerNameField } from "@/components/worker-name-field"
import { useAppConfig, useEnabledEngines, useSystemConfigs } from "@/hooks/use-config"
import { useCreateWorker } from "@/hooks/use-workers"
import { stripEmptyEngineArgs } from "@/lib/engine-args"
import { findEngine } from "@/lib/engines"
import { KNOWN_SCOPES, serializeScopes, toggleScope } from "@/lib/scopes"
import { SYSTEM_CONFIG_KEY_DEFAULT_ENGINE, availableEngine, newWorkerEngine, type Engine, type Worker } from "@/lib/types"
import { cn, getErrorMessage } from "@/lib/utils"
import { countAdvancedSettings } from "@/lib/worker-create"

function buildCreateEngineArgsPayload(engineArgs: Record<string, string>) {
  const stripped = stripEmptyEngineArgs(engineArgs)
  return Object.keys(stripped).length > 0 ? stripped : undefined
}

function FormSection({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section className="grid min-w-0 gap-5" aria-labelledby={id}>
      <div id={id}>
        <SectionHeading text={title} />
      </div>
      {children}
    </section>
  )
}

function AdvancedSection({
  open,
  onOpenChange,
  count,
  children,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  count: number
  children: ReactNode
}) {
  const { t } = useTranslation()
  const contentId = useId()

  return (
    <section className="grid min-w-0 gap-5">
      <div className="grid gap-1">
        <h2>
          <button
            type="button"
            aria-expanded={open}
            aria-controls={open ? contentId : undefined}
            onClick={() => onOpenChange(!open)}
            className="-mx-2 flex w-[calc(100%+1rem)] items-center gap-2 rounded-control px-2 py-1 text-left text-base font-medium text-foreground transition-colors hover:bg-surface-muted"
          >
            {t("workers.form.sectionAdvanced")}
            {!open && count > 0 && (
              <Badge tone="info" size="sm" className="tabular">
                {t("workers.form.advancedSet", { count })}
              </Badge>
            )}
            <ChevronDown
              size={16}
              strokeWidth={1.75}
              aria-hidden="true"
              className={cn("ml-auto shrink-0 text-muted transition-transform motion-reduce:transition-none", open && "rotate-180")}
            />
          </button>
        </h2>
        <p className="text-sm text-secondary">{t("workers.form.advancedHelper")}</p>
      </div>
      {open && (
        <div id={contentId} className="grid min-w-0 gap-5">
          {children}
        </div>
      )}
    </section>
  )
}

function CreateWorkerSkeleton() {
  return (
    <div className="grid gap-8" aria-busy="true">
      <div className="grid gap-5">
        <div className="skeleton h-5 w-28" />
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="grid gap-2">
            <div className="skeleton h-4 w-20" />
            <div className="skeleton h-11 w-full rounded-control" />
          </div>
        ))}
      </div>
      <div className="skeleton h-5 w-28" />
    </div>
  )
}

interface WorkerCreateFormProps {
  onCreated: (worker: Worker) => void
  onCancel: () => void
}

export function WorkerCreateForm({ onCreated, onCancel }: WorkerCreateFormProps) {
  const { t } = useTranslation()

  const createWorker = useCreateWorker()
  const enabledEngines = useEnabledEngines()
  const engineIds = useMemo(() => enabledEngines.map((e) => e.id), [enabledEngines])
  const { isPending: configPending } = useAppConfig()
  const { data: sysConfigs, isPending: sysConfigsPending } = useSystemConfigs()
  const systemDefaultEngine = sysConfigs?.[SYSTEM_CONFIG_KEY_DEFAULT_ENGINE]

  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [constraints, setConstraints] = useState("")
  const [workDir, setWorkDir] = useState("")
  const [selectedScopes, setSelectedScopes] = useState<string[]>([])
  const [engine, setEngine] = useState<Engine>("")
  const [engineArgs, setEngineArgs] = useState<Record<string, string>>({})
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [submitError, setSubmitError] = useState("")
  const selectedEngine = availableEngine(engine, engineIds)
  const advancedCount = countAdvancedSettings({
    constraints,
    workDir,
    engineArgs: engineArgs[selectedEngine] ?? "",
    scopes: selectedScopes,
  })

  const errorRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (submitError) {
      errorRef.current?.scrollIntoView({ behavior: "smooth", block: "center" })
    }
  }, [submitError])

  const seededRef = useRef(false)
  useEffect(() => {
    if (seededRef.current || sysConfigsPending || configPending) return
    seededRef.current = true
    setEngine(newWorkerEngine(systemDefaultEngine, engineIds))
  }, [sysConfigsPending, configPending, systemDefaultEngine, engineIds])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setSubmitError("")
    try {
      const worker = await createWorker.mutateAsync({
        name: name.trim(),
        engine: selectedEngine,
        description,
        constraints: constraints || undefined,
        work_dir: workDir || undefined,
        permission_scopes: serializeScopes(selectedScopes) || undefined,
        engine_args: buildCreateEngineArgsPayload(engineArgs),
      })
      onCreated(worker)
    } catch (err) {
      setSubmitError(getErrorMessage(err))
      if (advancedCount > 0) setAdvancedOpen(true)
    }
  }

  const isPending = createWorker.isPending

  if (sysConfigsPending || configPending) return <CreateWorkerSkeleton />

  return (
    <form onSubmit={handleSubmit} className="grid grid-cols-[minmax(0,1fr)] gap-8">
      <FormSection id="cw-section-basic" title={t("workers.form.sectionBasic")}>
        <WorkerNameField
          id="cw-name"
          open
          value={name}
          onChange={setName}
          onError={setSubmitError}
          autoFocus
          disabled={isPending}
        />
        <EngineSelect
          id="cw-engine"
          label={t("workers.form.engine")}
          description={t("workers.form.engineHelper")}
          placeholder={t("workers.form.engineDefault")}
          engines={enabledEngines}
          value={selectedEngine}
          onValueChange={setEngine}
          disabled={isPending}
        />
        <Textarea
          id="cw-desc"
          label={t("workers.form.descriptionOptional")}
          description={t("workers.form.descriptionCreateHelper")}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder={t("workers.form.descriptionPlaceholder")}
          rows={3}
          disabled={isPending}
        />
      </FormSection>

      <AdvancedSection open={advancedOpen} onOpenChange={setAdvancedOpen} count={advancedCount}>
        <Textarea
          id="cw-constraints"
          label={t("workers.form.constraints")}
          description={t("workers.form.constraintsHelper")}
          value={constraints}
          onChange={(e) => setConstraints(e.target.value)}
          placeholder={t("workers.form.constraintsPlaceholder")}
          rows={5}
          disabled={isPending}
        />
        <EngineArgsSection
          engines={selectedEngine ? [findEngine(enabledEngines, selectedEngine)] : []}
          value={engineArgs}
          onChange={setEngineArgs}
          idPrefix="cw-engine-args"
          disabled={isPending}
        />
        <Input
          id="cw-workdir"
          label={t("workers.form.workDir")}
          description={t("workers.form.workDirHelper")}
          value={workDir}
          onChange={(e) => setWorkDir(e.target.value)}
          placeholder={t("workers.form.workDirPlaceholder")}
          autoComplete="off"
          spellCheck={false}
          disabled={isPending}
        />
        <div role="group" aria-labelledby="cw-permissions-label" aria-describedby="cw-permissions-helper" className="grid min-w-0">
          <p id="cw-permissions-label" className="text-sm font-medium text-foreground">
            {t("workers.form.sectionPermissions")}
          </p>
          <p id="cw-permissions-helper" className="pt-2 text-xs text-muted">
            {t("workers.form.permissionsHelper")}
          </p>
          <div className="-ml-3 grid gap-y-1 pt-2">
            {KNOWN_SCOPES.map((scope) => (
              <Checkbox
                key={scope.id}
                id={`cw-scope-${scope.id}`}
                label={t(scope.titleKey)}
                description={t(scope.descriptionKey)}
                checked={selectedScopes.includes(scope.id)}
                onCheckedChange={(state) => setSelectedScopes((prev) => toggleScope(prev, scope.id, state === true))}
                disabled={isPending}
              />
            ))}
          </div>
        </div>
      </AdvancedSection>

      {submitError && (
        <div ref={errorRef}>
          <Alert tone="danger" title={t("workers.form.createError")}>
            {submitError}
          </Alert>
        </div>
      )}

      <div className="flex justify-end gap-2 border-t border-border-subtle pt-6">
        <Button type="button" variant="secondary" onClick={onCancel} disabled={isPending}>
          {t("common.cancel")}
        </Button>
        <Button type="submit" loading={isPending} disabled={!name.trim() || !selectedEngine}>
          {t("workers.createWorker")}
        </Button>
      </div>
    </form>
  )
}
