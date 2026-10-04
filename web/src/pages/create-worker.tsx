import { useState, useEffect, useRef, type FormEvent } from "react"
import { Link, useNavigate, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { ArrowLeft, Search } from "lucide-react"
import { useCreateWorker, useWorker } from "@/hooks/use-workers"
import { useFlatDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { useEnabledEngines } from "@/hooks/use-config"
import { FadeIn } from "@/components/fade-in"
import { DetailSection } from "@/components/detail-primitives"
import { PageHeader } from "@/components/page-header"
import { Panel } from "@/components/panel"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Select,
  SelectContent,
  SelectTrigger,
} from "@/components/ui/select"
import { EngineSelectItems, EngineSelectValue } from "@/components/engine-select-items"
import { EngineArgsSection } from "@/components/engine-args-section"
import { WorkerNameField } from "@/components/worker-name-field"
import { KNOWN_SCOPES, serializeScopes, parseScopes, toggleScope } from "@/lib/scopes"
import { stripEmptyEngineArgs } from "@/lib/engine-args"
import { cn, getErrorMessage } from "@/lib/utils"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
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

  // Panel titles carry an optional selection count, shown as a neutral badge.
  const countTitle = (text: string, count: number) => (
    <span className="flex items-center gap-2">
      {text}
      {count > 0 && (
        <Badge variant="secondary" className="tabular-nums">
          {count}
        </Badge>
      )}
    </span>
  )

  return (
    <FadeIn>
      <div className="w-full max-w-3xl">
        <Link
          to="/workers"
          className="mb-3 inline-flex items-center gap-1.5 rounded-sm text-[13px] text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          <ArrowLeft className="size-3.5" />
          {t("workers.backToList")}
        </Link>
        <PageHeader title={isCopy ? t("workers.copyWorker") : t("workers.createWorker")} />

        {submitError && (
          <div ref={errorRef} role="alert" className={cn(ALERT_DESTRUCTIVE, "mb-6")}>
            {submitError}
          </div>
        )}

        {isCopyLoading ? (
          <div className="space-y-6" aria-hidden>
            {[0, 1, 2].map((i) => (
              <DetailSection key={i} className="space-y-4 p-4">
                <Skeleton className="h-4 w-28" />
                <Skeleton className="h-9 w-full" />
                <Skeleton className="h-9 w-3/4" />
              </DetailSection>
            ))}
          </div>
        ) : (
        <form id="create-worker-form" onSubmit={handleSubmit} className="space-y-6">
          <Panel title={t("workers.form.sectionBasic")} bodyClassName="space-y-5">
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
                <SelectTrigger id="cw-engine" className="w-full max-w-sm">
                  <EngineSelectValue placeholder={t("workers.form.engineDefault")} />
                </SelectTrigger>
                <SelectContent>
                  <EngineSelectItems engines={enabledEngines} />
                </SelectContent>
              </Select>
            </div>
          </Panel>

          <Panel title={t("workers.form.sectionEnhancement")} bodyClassName="space-y-5">
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
            </div>
          </Panel>

          {flatDepts.length > 0 && (
            <Panel
              title={countTitle(t("workers.form.sectionDepartment"), selectedDeptIds.size)}
              flush
            >
              <div className="border-b border-hairline p-3">
                <div className="relative">
                  <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    value={deptSearch}
                    onChange={(e) => setDeptSearch(e.target.value)}
                    placeholder={t("workers.form.searchDepartments")}
                    aria-label={t("workers.form.searchDepartments")}
                    className="pl-8"
                  />
                </div>
              </div>

              <div className="max-h-56 overflow-y-auto p-1.5">
                {filteredDepts.length === 0 ? (
                  <p className="py-4 text-center text-[13px] text-muted-foreground">
                    {t("workers.form.noMatchingDepartments")}
                  </p>
                ) : (
                  filteredDepts.map(({ dept, depth }) => (
                    <label
                      key={dept.id}
                      className="flex h-8.5 cursor-pointer items-center gap-2.5 rounded-sm px-3 transition-colors hover:bg-accent"
                      style={{ paddingLeft: `${12 + depth * 16}px` }}
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
                        className="size-4 shrink-0 cursor-pointer rounded-sm accent-primary dark:scheme-dark"
                      />
                      <span className="truncate text-sm text-foreground">{dept.name}</span>
                    </label>
                  ))
                )}
              </div>
            </Panel>
          )}

          <Panel title={t("workers.form.sectionOther")} bodyClassName="space-y-5">
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
                className="font-mono text-[13px]"
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.workDirHelper")}</p>
            </div>
          </Panel>

          <Panel
            title={countTitle(t("workers.form.sectionPermissions"), selectedScopes.length)}
            bodyClassName="p-1.5"
          >
            <div className="grid grid-cols-1 gap-x-2 sm:grid-cols-2">
              {KNOWN_SCOPES.map((scope) => (
                <label
                  key={scope.id}
                  className="flex h-8.5 cursor-pointer items-center gap-2.5 rounded-sm px-3 transition-colors hover:bg-accent"
                >
                  <input
                    type="checkbox"
                    checked={selectedScopes.includes(scope.id)}
                    onChange={(e) =>
                      setSelectedScopes((prev) => toggleScope(prev, scope.id, e.target.checked))
                    }
                    disabled={isPending}
                    className="size-4 shrink-0 cursor-pointer rounded-sm accent-primary dark:scheme-dark"
                  />
                  <span className="truncate text-sm text-foreground">
                    {t(scope.titleKey)}
                  </span>
                </label>
              ))}
            </div>
          </Panel>

          <div className="flex justify-end gap-2 border-t border-border pt-4">
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
