import { useState, useEffect, useRef, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { ChevronDown, Search, Shuffle, Loader2 } from "lucide-react"
import { useCreateWorker, useRandomWorkerName } from "@/hooks/use-workers"
import { useFlatDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { useEnabledEngines } from "@/hooks/use-config"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Separator } from "@/components/ui/separator"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from "@/components/ui/sheet"
import {
  Select,
  SelectContent,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { EngineSelectItems } from "@/components/engine-select-items"
import { SectionHeading } from "@/components/section-heading"
import { KNOWN_SCOPES, serializeScopes, parseScopes, toggleScope } from "@/lib/scopes"
import { cn, getErrorMessage } from "@/lib/utils"
import type { Worker, Engine } from "@/lib/types"
import { DEFAULT_ENGINE, pickDefaultEngine } from "@/lib/types"

export interface WorkerInitialValues {
  name: string
  description: string
  constraints: string
  work_dir: string
  permission_scopes: string
  engine: Engine
  departmentIds: string[]
}

export function workerToInitialValues(worker: Worker): WorkerInitialValues {
  return {
    name: worker.name,
    description: worker.description,
    constraints: worker.constraints,
    work_dir: worker.work_dir,
    permission_scopes: worker.permission_scopes ?? "",
    engine: worker.engine ?? DEFAULT_ENGINE,
    departmentIds: worker.departments?.map((d) => d.id) ?? [],
  }
}

interface CreateWorkerSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialValues?: WorkerInitialValues
}

export function CreateWorkerSheet({ open, onOpenChange, initialValues }: CreateWorkerSheetProps) {
  const { t } = useTranslation()
  const createWorker = useCreateWorker()
  const setWorkerDepts = useSetWorkerDepartments()
  const flatDepts = useFlatDepartments()
  const enabledEngines = useEnabledEngines()
  const isCopy = initialValues !== undefined
  const initialValuesRef = useRef(initialValues)
  initialValuesRef.current = initialValues

  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [constraints, setConstraints] = useState("")
  const [workDir, setWorkDir] = useState("")
  const [selectedScopes, setSelectedScopes] = useState<string[]>([])
  const [engine, setEngine] = useState<Engine>(DEFAULT_ENGINE)
  const [selectedDeptIds, setSelectedDeptIds] = useState<Set<string>>(new Set())
  const [submitError, setSubmitError] = useState("")
  const [showOptional, setShowOptional] = useState(false)
  const [deptSearch, setDeptSearch] = useState("")
  const [nameExhausted, setNameExhausted] = useState(false)
  const randomName = useRandomWorkerName()

  useEffect(() => {
    if (open) {
      const iv = initialValuesRef.current
      setName(iv ? `${iv.name} ${t("workers.form.copySuffix")}` : "")
      setDescription(iv?.description ?? "")
      setConstraints(iv?.constraints ?? "")
      setWorkDir(iv?.work_dir ?? "")
      setEngine(pickDefaultEngine(iv?.engine, enabledEngines))
      setSelectedScopes(iv ? parseScopes(iv.permission_scopes) : [])
      setSelectedDeptIds(iv ? new Set(iv.departmentIds) : new Set())
      setSubmitError("")
      setShowOptional(isCopy && !!(iv?.description || iv?.constraints || iv?.work_dir))
      setDeptSearch("")
      setNameExhausted(false)
    }
  }, [open, enabledEngines])

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
      })
      if (selectedDeptIds.size > 0) {
        await setWorkerDepts.mutateAsync({ workerId: worker.id, departmentIds: [...selectedDeptIds] })
      }
      onOpenChange(false)
    } catch (err) {
      setSubmitError(getErrorMessage(err))
    }
  }

  const handleRandomName = async () => {
    try {
      const result = await randomName.mutateAsync()
      if (result.exhausted) {
        setNameExhausted(true)
      } else if (result.name) {
        setName(result.name)
      }
    } catch {
      // button returns to non-pending state automatically via useMutation
    }
  }

  const isPending = createWorker.isPending || setWorkerDepts.isPending

  const filteredDepts = deptSearch.trim()
    ? flatDepts.filter(({ dept }) =>
        dept.name.toLowerCase().includes(deptSearch.toLowerCase())
      )
    : flatDepts

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-[26rem] p-0 gap-0">
        <SheetHeader className="px-6 pt-6 pb-4">
          <SheetTitle>
            {isCopy ? t("workers.copyWorker") : t("workers.createWorker")}
          </SheetTitle>
        </SheetHeader>

        <Separator />

        <form
          id="create-worker-form"
          onSubmit={handleSubmit}
          className="flex-1 overflow-y-auto"
        >
          <div className="px-6 py-5 space-y-5">
            {submitError && (
              <div role="alert" className="rounded-lg border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                {submitError}
              </div>
            )}

            <div className="space-y-1.5">
              <Label htmlFor="cws-name">
                {t("workers.form.name")}
                <span className="ml-1 text-destructive" aria-hidden>*</span>
              </Label>
              <div className="flex gap-2">
                <Input
                  id="cws-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t("workers.form.namePlaceholder")}
                  required
                  autoFocus
                  className="flex-1"
                />
                <TooltipProvider>
                  <Tooltip open={nameExhausted || undefined}>
                    <TooltipTrigger asChild>
                      <span className="inline-flex">
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          disabled={nameExhausted || randomName.isPending}
                          onClick={handleRandomName}
                          aria-label={t("workers.form.randomName")}
                          style={{ pointerEvents: nameExhausted ? "none" : undefined }}
                        >
                          {randomName.isPending
                            ? <Loader2 className="size-4 animate-spin" />
                            : <Shuffle className="size-4" />
                          }
                        </Button>
                      </span>
                    </TooltipTrigger>
                    {nameExhausted && (
                      <TooltipContent>
                        <p>{t("workers.form.randomNameExhausted")}</p>
                      </TooltipContent>
                    )}
                  </Tooltip>
                </TooltipProvider>
              </div>
              <p className="text-xs text-muted-foreground">{t("workers.form.nameHelper")}</p>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="cws-engine">
                {t("workers.form.engine")}
                <span className="ml-1 text-destructive" aria-hidden>*</span>
              </Label>
              <Select value={engine} onValueChange={(v) => v && setEngine(v as Engine)}>
                <SelectTrigger id="cws-engine">
                  <SelectValue placeholder={t("workers.form.engineDefault")} />
                </SelectTrigger>
                <SelectContent>
                  <EngineSelectItems engines={enabledEngines} />
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{t("workers.form.engineHelper")}</p>
            </div>
          </div>

          <div className="border-t border-border/60">
            <button
              type="button"
              onClick={() => setShowOptional((v) => !v)}
              aria-expanded={showOptional}
              className="flex w-full items-center justify-between px-6 py-3 text-left text-xs font-medium text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            >
              <span>{t("workers.form.optionalSettings")}</span>
              <ChevronDown
                className={cn(
                  "size-3.5 transition-transform duration-200",
                  showOptional && "rotate-180"
                )}
              />
            </button>

            <div
              className="grid transition-[grid-template-rows] duration-200 ease-out"
              style={{ gridTemplateRows: showOptional ? "1fr" : "0fr" }}
            >
              <div className="overflow-hidden">
                <div className="px-6 pb-5 space-y-5">
                  <div className="space-y-1.5">
                    <Label htmlFor="cws-desc">{t("workers.form.description")}</Label>
                    <Textarea
                      id="cws-desc"
                      value={description}
                      onChange={(e) => setDescription(e.target.value)}
                      placeholder={t("workers.form.descriptionPlaceholder")}
                      rows={2}
                    />
                    <p className="text-xs text-muted-foreground">{t("workers.form.descriptionHelper")}</p>
                  </div>

                  <div className="space-y-1.5">
                    <Label htmlFor="cws-workdir">{t("workers.form.workDir")}</Label>
                    <Input
                      id="cws-workdir"
                      value={workDir}
                      onChange={(e) => setWorkDir(e.target.value)}
                      placeholder={t("workers.form.workDirPlaceholder")}
                      className="font-mono text-xs"
                    />
                    <p className="text-xs text-muted-foreground">{t("workers.form.workDirHelper")}</p>
                  </div>

                  <div className="space-y-1.5">
                    <Label htmlFor="cws-constraints">{t("workers.form.constraints")}</Label>
                    <Textarea
                      id="cws-constraints"
                      value={constraints}
                      onChange={(e) => setConstraints(e.target.value)}
                      placeholder={t("workers.form.constraintsPlaceholder")}
                      rows={4}
                    />
                    <p className="text-xs text-muted-foreground">{t("workers.form.constraintsHelper")}</p>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div className="border-t border-border/60 px-6 py-5 space-y-3">
            <SectionHeading
              text={t("workers.form.sectionPermissions")}
              badge={selectedScopes.length}
            />
            <div className="grid grid-cols-2 gap-x-4 gap-y-2.5">
              {KNOWN_SCOPES.map((scope) => (
                <label
                  key={scope.id}
                  className="flex items-center gap-2 cursor-pointer group"
                >
                  <input
                    type="checkbox"
                    checked={selectedScopes.includes(scope.id)}
                    onChange={(e) =>
                      setSelectedScopes((prev) => toggleScope(prev, scope.id, e.target.checked))
                    }
                    disabled={isPending}
                    className="size-3.5 shrink-0 cursor-pointer rounded accent-primary"
                  />
                  <span className="text-sm text-foreground/75 group-hover:text-foreground transition-colors leading-snug">
                    {t(scope.titleKey)}
                  </span>
                </label>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">{t("workers.form.permissionsHelper")}</p>
          </div>

          {flatDepts.length > 0 && (
            <div className="border-t border-border/60 px-6 py-5 space-y-3">
              <SectionHeading
                text={t("workers.form.sectionDepartment")}
                badge={selectedDeptIds.size}
              />

              <div className="relative">
                <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground pointer-events-none" />
                <Input
                  value={deptSearch}
                  onChange={(e) => setDeptSearch(e.target.value)}
                  placeholder={t("workers.form.searchDepartments")}
                  className="pl-8 h-8 text-xs"
                />
              </div>

              <div className="space-y-0.5 max-h-48 overflow-y-auto -mx-1">
                {filteredDepts.length === 0 ? (
                  <p className="py-3 text-xs text-muted-foreground text-center">
                    {t("workers.form.noMatchingDepartments")}
                  </p>
                ) : (
                  filteredDepts.map(({ dept, depth }) => (
                    <label
                      key={dept.id}
                      className="flex items-center gap-2 rounded px-3 py-1.5 cursor-pointer hover:bg-muted/50 transition-colors"
                      style={{ paddingLeft: `${12 + depth * 12}px` }}
                    >
                      <input
                        type="checkbox"
                        id={`cws-dept-${dept.id}`}
                        checked={selectedDeptIds.has(dept.id)}
                        onChange={(e) => {
                          const next = new Set(selectedDeptIds)
                          if (e.target.checked) next.add(dept.id)
                          else next.delete(dept.id)
                          setSelectedDeptIds(next)
                        }}
                        className="size-3.5 shrink-0 cursor-pointer rounded accent-primary"
                      />
                      <span className="text-sm text-foreground/75 leading-snug">{dept.name}</span>
                    </label>
                  ))
                )}
              </div>

              <p className="text-xs text-muted-foreground">{t("workers.form.departmentHelper")}</p>
            </div>
          )}
        </form>

        <Separator />
        <SheetFooter className="px-6 py-4 flex-row gap-2">
          <Button
            type="button"
            variant="outline"
            className="flex-1"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            form="create-worker-form"
            disabled={isPending || !name.trim()}
            className="flex-1"
          >
            {isCopy ? t("workers.copyWorker") : t("workers.createWorker")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
