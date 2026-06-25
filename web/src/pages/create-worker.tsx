import { useState, useEffect, useRef, type FormEvent } from "react"
import { Link, useNavigate, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { ArrowLeft, Search } from "lucide-react"
import { useCreateWorker, useWorker } from "@/hooks/use-workers"
import { useFlatDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { useEnabledEngines } from "@/hooks/use-config"
import { FadeIn } from "@/components/fade-in"
import { DetailSection } from "@/components/detail-primitives"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Select,
  SelectContent,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { EngineSelectItems } from "@/components/engine-select-items"
import { EngineArgsSection } from "@/components/engine-args-section"
import { SectionHeading } from "@/components/section-heading"
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
  const { data: copyWorker } = useWorker(copyId ?? "", { enabled: isCopy })

  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [constraints, setConstraints] = useState("")
  const [workDir, setWorkDir] = useState("")
  const [selectedScopes, setSelectedScopes] = useState<string[]>([])
  const [engine, setEngine] = useState<Engine>(DEFAULT_ENGINE)
  const [selectedDeptIds, setSelectedDeptIds] = useState<Set<string>>(new Set())
  const [engineArgs, setEngineArgs] = useState<Record<string, string>>({})
  const [submitError, setSubmitError] = useState("")
  const [deptSearch, setDeptSearch] = useState("")

  // Bring submission failures into view: on a long form the alert sits above
  // the submit button, so scroll it into the viewport when it appears.
  const errorRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (submitError) {
      errorRef.current?.scrollIntoView({ behavior: "smooth", block: "center" })
    }
  }, [submitError])

  // Seed the form once: for a copy we must wait for the source worker to load,
  // otherwise we initialize immediately with defaults.
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
  const isCopyLoading = isCopy && !copyWorker

  const filteredDepts = deptSearch.trim()
    ? flatDepts.filter(({ dept }) =>
        dept.name.toLowerCase().includes(deptSearch.toLowerCase())
      )
    : flatDepts

  return (
    <FadeIn>
      <div className="mx-auto w-full max-w-2xl space-y-6">
        <div className="space-y-3">
          <Link
            to="/workers"
            className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
          >
            <ArrowLeft className="size-4" />
            {t("workers.backToList")}
          </Link>
          <div>
            <h1 className="text-xl font-semibold tracking-tight">
              {isCopy ? t("workers.copyWorker") : t("workers.createWorker")}
            </h1>
            {isCopy && (
              <p className="mt-1 text-sm text-muted-foreground">
                {t("workers.form.copyPanelDescription")}
              </p>
            )}
          </div>
        </div>

        {submitError && (
          <div ref={errorRef} role="alert" className="rounded-sm border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {submitError}
          </div>
        )}

        {isCopyLoading ? (
          <div className="space-y-6" aria-hidden>
            {[0, 1, 2].map((i) => (
              <DetailSection key={i} className="space-y-4 p-5 sm:p-6">
                <Skeleton className="h-4 w-28" />
                <Skeleton className="h-8 w-full" />
                <Skeleton className="h-8 w-3/4" />
              </DetailSection>
            ))}
          </div>
        ) : (
        <form id="create-worker-form" onSubmit={handleSubmit} className="space-y-6">
          <DetailSection className="space-y-5 p-5 sm:p-6">
            <SectionHeading text={t("workers.form.sectionBasic")} />

            <div className="max-w-sm">
              <WorkerNameField
                id="cw-name"
                open
                value={name}
                onChange={setName}
                onError={setSubmitError}
                autoFocus
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="cw-engine">{t("workers.form.engine")}</Label>
              <Select value={engine} onValueChange={(v) => v && setEngine(v as Engine)}>
                <SelectTrigger id="cw-engine">
                  <SelectValue placeholder={t("workers.form.engineDefault")} />
                </SelectTrigger>
                <SelectContent>
                  <EngineSelectItems engines={enabledEngines} />
                </SelectContent>
              </Select>
            </div>
          </DetailSection>

          <DetailSection className="space-y-5 p-5 sm:p-6">
            <SectionHeading text={t("workers.form.sectionEnhancement")} />

            <div className="space-y-1.5">
              <Label htmlFor="cw-desc">{t("workers.form.description")}</Label>
              <Textarea
                id="cw-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder={t("workers.form.descriptionPlaceholder")}
                rows={2}
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.descriptionHelper")}</p>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="cw-constraints">{t("workers.form.constraints")}</Label>
              <Textarea
                id="cw-constraints"
                value={constraints}
                onChange={(e) => setConstraints(e.target.value)}
                placeholder={t("workers.form.constraintsPlaceholder")}
                rows={4}
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.constraintsHelper")}</p>
            </div>
          </DetailSection>

          {flatDepts.length > 0 && (
            <DetailSection className="space-y-3 p-5 sm:p-6">
              <SectionHeading
                text={t("workers.form.sectionDepartment")}
                badge={selectedDeptIds.size}
              />

              <div className="relative">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={deptSearch}
                  onChange={(e) => setDeptSearch(e.target.value)}
                  placeholder={t("workers.form.searchDepartments")}
                  className="h-8 pl-8 text-xs"
                />
              </div>

              <div className="-mx-1 max-h-48 space-y-0.5 overflow-y-auto">
                {filteredDepts.length === 0 ? (
                  <p className="py-3 text-center text-xs text-muted-foreground">
                    {t("workers.form.noMatchingDepartments")}
                  </p>
                ) : (
                  filteredDepts.map(({ dept, depth }) => (
                    <label
                      key={dept.id}
                      className="flex cursor-pointer items-center gap-2 rounded-sm px-3 py-1.5 transition-colors hover:bg-muted/50"
                      style={{ paddingLeft: `${12 + depth * 12}px` }}
                    >
                      <input
                        type="checkbox"
                        id={`cw-dept-${dept.id}`}
                        checked={selectedDeptIds.has(dept.id)}
                        onChange={(e) => {
                          const next = new Set(selectedDeptIds)
                          if (e.target.checked) next.add(dept.id)
                          else next.delete(dept.id)
                          setSelectedDeptIds(next)
                        }}
                        className="size-3.5 shrink-0 cursor-pointer rounded-sm accent-primary"
                      />
                      <span className="text-sm leading-snug text-foreground/75">{dept.name}</span>
                    </label>
                  ))
                )}
              </div>
            </DetailSection>
          )}

          <DetailSection className="space-y-5 p-5 sm:p-6">
            <SectionHeading text={t("workers.form.sectionOther")} />

            <EngineArgsSection
              engines={[engine]}
              value={engineArgs}
              onChange={setEngineArgs}
            />

            <div className="space-y-1.5">
              <Label htmlFor="cw-workdir">{t("workers.form.workDir")}</Label>
              <Input
                id="cw-workdir"
                value={workDir}
                onChange={(e) => setWorkDir(e.target.value)}
                placeholder={t("workers.form.workDirPlaceholder")}
                className="font-mono text-xs"
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.workDirHelper")}</p>
            </div>
          </DetailSection>

          <DetailSection className="space-y-3 p-5 sm:p-6">
            <SectionHeading
              text={t("workers.form.sectionPermissions")}
              badge={selectedScopes.length}
            />
            <div className="grid grid-cols-1 gap-0.5 sm:grid-cols-2">
              {KNOWN_SCOPES.map((scope) => (
                <label
                  key={scope.id}
                  className="flex cursor-pointer items-center gap-2 rounded-sm px-3 py-1.5 transition-colors hover:bg-muted/50"
                >
                  <input
                    type="checkbox"
                    checked={selectedScopes.includes(scope.id)}
                    onChange={(e) =>
                      setSelectedScopes((prev) => toggleScope(prev, scope.id, e.target.checked))
                    }
                    disabled={isPending}
                    className="size-3.5 shrink-0 cursor-pointer rounded-sm accent-primary"
                  />
                  <span className="text-sm leading-snug text-foreground/75">
                    {t(scope.titleKey)}
                  </span>
                </label>
              ))}
            </div>
          </DetailSection>

          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => navigate("/workers")}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={isPending || !name.trim()}
            >
              {isCopy ? t("workers.copyWorker") : t("workers.createWorker")}
            </Button>
          </div>
        </form>
        )}
      </div>
    </FadeIn>
  )
}
