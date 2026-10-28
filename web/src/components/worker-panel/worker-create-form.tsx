import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Checkbox } from "@/components/arc/checkbox/checkbox"
import { Input } from "@/components/arc/input/input"
import { Select } from "@/components/arc/select/select"
import { Textarea } from "@/components/arc/textarea/textarea"
import { EngineArgsSection } from "@/components/engine-args-section"
import { useEngineOptions } from "@/components/engine-select-items"
import { SectionHeading } from "@/components/section-heading"
import { WorkerNameField } from "@/components/worker-name-field"
import { useEnabledEngines } from "@/hooks/use-config"
import { useCreateWorker, useWorker } from "@/hooks/use-workers"
import { stripEmptyEngineArgs } from "@/lib/engine-args"
import { KNOWN_SCOPES, parseScopes, serializeScopes, toggleScope } from "@/lib/scopes"
import { DEFAULT_ENGINE, availableEngine, pickDefaultEngine, type Engine, type Worker } from "@/lib/types"
import { getErrorMessage } from "@/lib/utils"

interface WorkerInitialValues {
  name: string
  description: string
  constraints: string
  work_dir: string
  permission_scopes: string
  engine: Engine
  engine_args: Record<string, string>
}

function workerToInitialValues(worker: Worker): WorkerInitialValues {
  return {
    name: worker.name,
    description: worker.description,
    constraints: worker.constraints,
    work_dir: worker.work_dir,
    permission_scopes: worker.permission_scopes ?? "",
    engine: worker.engine ?? DEFAULT_ENGINE,
    engine_args: worker.engine_args ?? {},
  }
}

function buildCreateEngineArgsPayload(engineArgs: Record<string, string>) {
  const stripped = stripEmptyEngineArgs(engineArgs)
  return Object.keys(stripped).length > 0 ? stripped : undefined
}

function FormSection({
  id,
  title,
  description,
  badge,
  children,
}: {
  id: string
  title: string
  description?: string
  badge?: number
  children: ReactNode
}) {
  return (
    <section className="grid min-w-0 gap-5" aria-labelledby={id}>
      <div className="grid gap-1">
        <div id={id}>
          <SectionHeading text={title} badge={badge} />
        </div>
        {description && <p className="text-sm text-secondary">{description}</p>}
      </div>
      {children}
    </section>
  )
}

function CreateWorkerSkeleton() {
  return (
    <div className="grid gap-8" aria-busy="true">
      {[3, 2, 2].map((fields, section) => (
        <div key={section} className="grid gap-5">
          <div className="skeleton h-5 w-28" />
          {Array.from({ length: fields }).map((_, i) => (
            <div key={i} className="grid gap-2">
              <div className="skeleton h-4 w-20" />
              <div className="skeleton h-11 w-full rounded-control" />
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}

interface WorkerCreateFormProps {
  copyFrom?: string
  onCreated: (worker: Worker) => void
  onCancel: () => void
}

export function WorkerCreateForm({ copyFrom, onCreated, onCancel }: WorkerCreateFormProps) {
  const { t } = useTranslation()
  const isCopy = !!copyFrom

  const createWorker = useCreateWorker()
  const enabledEngines = useEnabledEngines()
  const engineOptions = useEngineOptions(enabledEngines)
  const { data: copyWorker, error: copyError, refetch: refetchCopy } = useWorker(copyFrom ?? "", { enabled: isCopy })

  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [constraints, setConstraints] = useState("")
  const [workDir, setWorkDir] = useState("")
  const [selectedScopes, setSelectedScopes] = useState<string[]>([])
  const [engine, setEngine] = useState<Engine>(DEFAULT_ENGINE)
  const [engineArgs, setEngineArgs] = useState<Record<string, string>>({})
  const [submitError, setSubmitError] = useState("")
  const selectedEngine = availableEngine(engine, enabledEngines)

  const errorRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (submitError) {
      errorRef.current?.scrollIntoView({ behavior: "smooth", block: "center" })
    }
  }, [submitError])

  const seededRef = useRef(false)
  useEffect(() => {
    if (seededRef.current) return
    if (isCopy && !copyWorker) return
    seededRef.current = true
    const iv = copyWorker ? workerToInitialValues(copyWorker) : undefined
    setName(iv ? `${iv.name} ${t("workers.form.copySuffix")}` : "")
    setDescription(iv?.description ?? "")
    setConstraints(iv?.constraints ?? "")
    setWorkDir(iv?.work_dir ?? "")
    setEngine(pickDefaultEngine(iv?.engine, enabledEngines))
    setSelectedScopes(iv ? parseScopes(iv.permission_scopes) : [])
    setEngineArgs(iv?.engine_args ?? {})
  }, [isCopy, copyWorker, enabledEngines, t])

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
    }
  }

  const isPending = createWorker.isPending
  const isCopyLoading = isCopy && !copyWorker && !copyError
  const submitLabel = isCopy ? t("workers.copyWorker") : t("workers.createWorker")

  if (copyError && !copyWorker) {
    return (
      <Alert tone="danger" title={t("workers.form.copyLoadError")}>
        <span className="grid justify-items-start gap-3">
          <span>{copyError.message}</span>
          <Button variant="secondary" size="sm" onClick={() => refetchCopy()}>
            {t("common.retry")}
          </Button>
        </span>
      </Alert>
    )
  }

  if (isCopyLoading) return <CreateWorkerSkeleton />

  return (
    <form onSubmit={handleSubmit} className="grid grid-cols-[minmax(0,1fr)] gap-8">
      {isCopy && copyWorker && (
        <p className="text-sm text-secondary">{t("workers.form.copySubtitle", { name: copyWorker.name })}</p>
      )}

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
        <Select
          id="cw-engine"
          label={t("workers.form.engine")}
          description={t("workers.form.engineHelper")}
          placeholder={t("workers.form.engineDefault")}
          options={engineOptions}
          value={selectedEngine}
          onValueChange={(v) => v && setEngine(v as Engine)}
          disabled={isPending}
        />
      </FormSection>

      <FormSection id="cw-section-enhancement" title={t("workers.form.sectionEnhancement")}>
        <Textarea
          id="cw-desc"
          label={t("workers.form.description")}
          description={t("workers.form.descriptionHelper")}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder={t("workers.form.descriptionPlaceholder")}
          rows={3}
          disabled={isPending}
        />
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
      </FormSection>

      <FormSection id="cw-section-other" title={t("workers.form.sectionOther")}>
        <EngineArgsSection
          engines={[selectedEngine]}
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
      </FormSection>

      <FormSection
        id="cw-section-permissions"
        title={t("workers.form.sectionPermissions")}
        description={t("workers.form.permissionsHelper")}
        badge={selectedScopes.length}
      >
        <div className="-ml-3 grid gap-y-1">
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
      </FormSection>

      {submitError && (
        <div ref={errorRef}>
          <Alert tone="danger" title={isCopy ? t("workers.form.copyError") : t("workers.form.createError")}>
            {submitError}
          </Alert>
        </div>
      )}

      <div className="flex justify-end gap-2 border-t border-border-subtle pt-6">
        <Button type="button" variant="secondary" onClick={onCancel} disabled={isPending}>
          {t("common.cancel")}
        </Button>
        <Button type="submit" loading={isPending} disabled={!name.trim()}>
          {submitLabel}
        </Button>
      </div>
    </form>
  )
}
