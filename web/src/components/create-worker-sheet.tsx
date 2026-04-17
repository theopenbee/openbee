import { useState, useEffect, useRef, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { useCreateWorker } from "@/hooks/use-workers"
import { useFlatDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Separator } from "@/components/ui/separator"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from "@/components/ui/sheet"
import {
  Select,
  SelectContent,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { EngineSelectItems } from "@/components/engine-select-items"
import { ScopeToggleCard } from "@/components/scope-toggle-card"
import { KNOWN_SCOPES, serializeScopes, parseScopes, toggleScope } from "@/lib/scopes"
import { getErrorMessage } from "@/lib/utils"
import type { Worker } from "@/lib/types"
import { DEFAULT_ENGINE } from "@/lib/types"
import type { Engine } from "@/lib/types"

export interface WorkerInitialValues {
  name: string
  description: string
  memory: string
  work_dir: string
  permission_scopes: string
  engine: Engine
  departmentIds: string[]
}

export function workerToInitialValues(worker: Worker): WorkerInitialValues {
  return {
    name: worker.name,
    description: worker.description,
    memory: worker.memory,
    work_dir: worker.work_dir,
    permission_scopes: worker.permission_scopes ?? "",
    engine: worker.engine ?? DEFAULT_ENGINE,
    departmentIds: worker.departments?.map((d) => d.id) ?? [],
  }
}

function SectionHeading({ text }: { text: string }) {
  return (
    <p className="text-xs font-medium uppercase tracking-[0.1em] text-muted-foreground">{text}</p>
  )
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
  const isCopy = initialValues !== undefined
  const initialValuesRef = useRef(initialValues)
  initialValuesRef.current = initialValues

  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [memory, setMemory] = useState("")
  const [workDir, setWorkDir] = useState("")
  const [selectedScopes, setSelectedScopes] = useState<string[]>([])
  const [engine, setEngine] = useState<Engine>(DEFAULT_ENGINE)
  const [selectedDeptIds, setSelectedDeptIds] = useState<Set<string>>(new Set())
  const [submitError, setSubmitError] = useState("")

  useEffect(() => {
    if (open) {
      const iv = initialValuesRef.current
      setName(iv ? `${iv.name} ${t("workers.form.copySuffix")}` : "")
      setDescription(iv?.description ?? "")
      setMemory(iv?.memory ?? "")
      setWorkDir(iv?.work_dir ?? "")
      setEngine(iv?.engine ?? DEFAULT_ENGINE)
      setSelectedScopes(iv ? parseScopes(iv.permission_scopes) : [])
      setSelectedDeptIds(iv ? new Set(iv.departmentIds) : new Set())
      setSubmitError("")
    }
  }, [open])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setSubmitError("")
    try {
      const worker = await createWorker.mutateAsync({
        name: name.trim(),
        engine,
        description,
        memory: memory || undefined,
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

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-[26rem] p-0 gap-0">
        <SheetHeader className="px-6 pt-6 pb-4">
          <SheetTitle>
            {isCopy ? t("workers.copyWorker") : t("workers.createWorker")}
          </SheetTitle>
          <SheetDescription>
            {isCopy ? t("workers.form.copyPanelDescription") : t("workers.form.panelDescription")}
          </SheetDescription>
        </SheetHeader>
        <Separator />
        <form
          id="create-worker-form"
          onSubmit={handleSubmit}
          className="flex-1 overflow-y-auto px-6 py-5 space-y-6"
        >
          {submitError && (
            <div role="alert" className="rounded-2xl border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {submitError}
            </div>
          )}

          <div className="space-y-4">
            <SectionHeading text={t("workers.form.sectionBasic")} />
            <div className="space-y-1.5">
              <Label htmlFor="cws-name">
                {t("workers.form.name")}
                <span className="ml-1 text-destructive" aria-hidden>*</span>
              </Label>
              <Input
                id="cws-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t("workers.form.namePlaceholder")}
                required
                autoFocus
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.nameHelper")}</p>
            </div>
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
          </div>

          <Separator />

          <div className="space-y-4">
            <SectionHeading text={t("workers.form.sectionConfig")} />
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
                  <EngineSelectItems />
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{t("workers.form.engineHelper")}</p>
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
              <Label htmlFor="cws-memory">{t("workers.form.memory")}</Label>
              <Textarea
                id="cws-memory"
                value={memory}
                onChange={(e) => setMemory(e.target.value)}
                placeholder={t("workers.form.memoryPlaceholder")}
                rows={5}
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.memoryHelper")}</p>
            </div>
          </div>

          <Separator />

          <div className="space-y-4">
            <SectionHeading text={t("workers.form.sectionPermissions")} />
            <div className="space-y-2">
              {KNOWN_SCOPES.map((scope) => (
                <ScopeToggleCard
                  key={scope.id}
                  scope={scope}
                  checked={selectedScopes.includes(scope.id)}
                  onToggle={(scopeId, val) =>
                    setSelectedScopes((prev) => toggleScope(prev, scopeId, val))
                  }
                  disabled={createWorker.isPending}
                />
              ))}
            </div>
          </div>

          {flatDepts.length > 0 && (
            <>
              <Separator />
              <div className="space-y-4">
                <SectionHeading text={t("workers.form.sectionDepartment")} />
                <div className="space-y-2 max-h-40 overflow-y-auto">
                  {flatDepts.map(({ dept, depth }) => (
                    <div
                      key={dept.id}
                      className="flex items-center gap-2"
                      style={{ paddingLeft: `${depth * 12}px` }}
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
                        className="size-4 cursor-pointer rounded accent-primary"
                      />
                      <Label htmlFor={`cws-dept-${dept.id}`} className="cursor-pointer text-sm font-normal">
                        {dept.name}
                      </Label>
                    </div>
                  ))}
                </div>
                <p className="text-xs text-muted-foreground">{t("workers.form.departmentHelper")}</p>
              </div>
            </>
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
            disabled={createWorker.isPending || setWorkerDepts.isPending || !name.trim() || !engine}
            className="flex-1"
          >
            {isCopy ? t("workers.copyWorker") : t("workers.createWorker")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
