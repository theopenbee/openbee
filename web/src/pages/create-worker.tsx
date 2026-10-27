import { useState, useEffect, useRef, type FormEvent, type ReactNode } from "react"
import { Link, useNavigate, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { ArrowLeft } from "lucide-react"
import { useCreateWorker, useWorker } from "@/hooks/use-workers"
import { useFlatDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { useEnabledEngines } from "@/hooks/use-config"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Checkbox } from "@/components/arc/checkbox/checkbox"
import { Input } from "@/components/arc/input/input"
import { Select } from "@/components/arc/select/select"
import { Textarea } from "@/components/arc/textarea/textarea"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { SectionHeading } from "@/components/section-heading"
import { useEngineOptions } from "@/components/engine-select-items"
import { EngineArgsSection } from "@/components/engine-args-section"
import { WorkerDepartmentPicker } from "@/components/worker-department-picker"
import { WorkerNameField } from "@/components/worker-name-field"
import { KNOWN_SCOPES, serializeScopes, parseScopes, toggleScope } from "@/lib/scopes"
import { stripEmptyEngineArgs } from "@/lib/engine-args"
import { getErrorMessage } from "@/lib/utils"
import type { Worker, Engine } from "@/lib/types"
import { DEFAULT_ENGINE, pickDefaultEngine } from "@/lib/types"

interface WorkerInitialValues {
  name: string
  description: string
  constraints: string
  work_dir: string
  permission_scopes: string
  engine: Engine
  departmentIds: string[]
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
    departmentIds: worker.departments?.map((d) => d.id) ?? [],
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
    <div className="grid gap-10" aria-busy="true">
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

export function CreateWorker() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const copyId = searchParams.get("copy")
  const isCopy = !!copyId

  const createWorker = useCreateWorker()
  const setWorkerDepts = useSetWorkerDepartments()
  const flatDepts = useFlatDepartments()
  const enabledEngines = useEnabledEngines()
  const engineOptions = useEngineOptions(enabledEngines)
  const { data: copyWorker, error: copyError, refetch: refetchCopy } = useWorker(copyId ?? "", { enabled: isCopy })

  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [constraints, setConstraints] = useState("")
  const [workDir, setWorkDir] = useState("")
  const [selectedScopes, setSelectedScopes] = useState<string[]>([])
  const [engine, setEngine] = useState<Engine>(DEFAULT_ENGINE)
  const [selectedDeptIds, setSelectedDeptIds] = useState<Set<string>>(new Set())
  const [engineArgs, setEngineArgs] = useState<Record<string, string>>({})
  const [submitError, setSubmitError] = useState("")

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
    setSelectedDeptIds(iv ? new Set(iv.departmentIds) : new Set())
    setEngineArgs(iv?.engine_args ?? {})
  }, [isCopy, copyWorker, enabledEngines, t])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setSubmitError("")
    try {
      const worker = await createWorker.mutateAsync({
        name: name.trim(),
        engine,
        description,
        constraints: constraints || undefined,
        work_dir: workDir || undefined,
        permission_scopes: serializeScopes(selectedScopes) || undefined,
        engine_args: buildCreateEngineArgsPayload(engineArgs),
      })
      if (selectedDeptIds.size > 0) {
        await setWorkerDepts.mutateAsync({ workerId: worker.id, departmentIds: [...selectedDeptIds] })
      }
      navigate("/workers")
    } catch (err) {
      setSubmitError(getErrorMessage(err))
    }
  }

  const isPending = createWorker.isPending || setWorkerDepts.isPending
  const isCopyLoading = isCopy && !copyWorker && !copyError
  const title = isCopy ? t("workers.copyWorker") : t("workers.createWorker")

  return (
    <FadeIn>
      <div className="max-w-form">
        <Link
          to="/workers"
          className="-ml-3 mb-4 inline-flex h-9 items-center gap-2 rounded-control px-3 text-sm text-secondary transition-colors hover:bg-surface-muted hover:text-foreground"
        >
          <ArrowLeft size={16} strokeWidth={1.75} aria-hidden="true" />
          {t("workers.backToList")}
        </Link>

        <PageHeader
          title={title}
          subtitle={isCopy && copyWorker ? t("workers.form.copySubtitle", { name: copyWorker.name }) : undefined}
        />

        {copyError && !copyWorker ? (
          <Alert tone="danger" title={t("workers.form.copyLoadError")}>
            <span className="grid justify-items-start gap-3">
              <span>{copyError.message}</span>
              <Button variant="secondary" size="sm" onClick={() => refetchCopy()}>
                {t("common.retry")}
              </Button>
            </span>
          </Alert>
        ) : isCopyLoading ? (
          <CreateWorkerSkeleton />
        ) : (
          <form id="create-worker-form" onSubmit={handleSubmit} className="grid gap-10">
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
                value={engine}
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

            {flatDepts.length > 0 && (
              <FormSection
                id="cw-section-department"
                title={t("workers.form.sectionDepartment")}
                description={t("workers.form.departmentHelper")}
                badge={selectedDeptIds.size}
              >
                <WorkerDepartmentPicker
                  selected={selectedDeptIds}
                  onChange={setSelectedDeptIds}
                  disabled={isPending}
                />
              </FormSection>
            )}

            <FormSection id="cw-section-other" title={t("workers.form.sectionOther")}>
              <EngineArgsSection
                engines={[engine]}
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
              <div className="-ml-3 grid gap-x-6 gap-y-1 sm:grid-cols-2">
                {KNOWN_SCOPES.map((scope) => (
                  <Checkbox
                    key={scope.id}
                    id={`cw-scope-${scope.id}`}
                    label={t(scope.titleKey)}
                    description={t(scope.descriptionKey)}
                    checked={selectedScopes.includes(scope.id)}
                    onCheckedChange={(state) =>
                      setSelectedScopes((prev) => toggleScope(prev, scope.id, state === true))
                    }
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

            <div className="flex flex-col-reverse gap-2 border-t border-border-subtle pt-6 sm:flex-row sm:justify-end">
              <Button type="button" variant="secondary" onClick={() => navigate("/workers")} disabled={isPending}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" loading={isPending} disabled={!name.trim()}>
                {title}
              </Button>
            </div>
          </form>
        )}
      </div>
    </FadeIn>
  )
}
