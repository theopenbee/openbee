import { useState, useEffect, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { Search } from "lucide-react"
import { useUpdateWorker } from "@/hooks/use-workers"
import { useFlatDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { useEnabledEngines } from "@/hooks/use-config"
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
  SheetFooter,
} from "@/components/ui/sheet"
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
import { getErrorMessage } from "@/lib/utils"
import { engineArgsEqual, stripEmptyEngineArgs } from "@/lib/engine-args"
import type { Worker, Engine } from "@/lib/types"
import { DEFAULT_ENGINE, pickDefaultEngine } from "@/lib/types"

interface EditWorkerInfoSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  worker: Worker
}

export function EditWorkerInfoSheet({ open, onOpenChange, worker }: EditWorkerInfoSheetProps) {
  const { t } = useTranslation()
  const updateWorker = useUpdateWorker()
  const setWorkerDepts = useSetWorkerDepartments()
  const flatDepts = useFlatDepartments()
  const enabledEngines = useEnabledEngines()

  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [workDir, setWorkDir] = useState("")
  const [engine, setEngine] = useState<Engine>(DEFAULT_ENGINE)
  const [selectedDeptIds, setSelectedDeptIds] = useState<Set<string>>(new Set())
  const [engineArgs, setEngineArgs] = useState<Record<string, string>>({})
  const [deptSearch, setDeptSearch] = useState("")
  const [submitError, setSubmitError] = useState("")

  useEffect(() => {
    if (open) {
      setName(worker.name ?? "")
      setDescription(worker.description ?? "")
      setWorkDir(worker.work_dir ?? "")
      setEngine(pickDefaultEngine(worker.engine, enabledEngines))
      setSelectedDeptIds(new Set(worker.departments?.map((d) => d.id) ?? []))
      setEngineArgs(worker.engine_args ?? {})
      setDeptSearch("")
      setSubmitError("")
    }
  }, [open, worker, enabledEngines])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setSubmitError("")
    try {
      const originalDeptIds = worker.departments?.map((d) => d.id).sort().join(",") ?? ""
      const newDeptIds = [...selectedDeptIds].sort().join(",")
      const engineArgsChanged = !engineArgsEqual(
        stripEmptyEngineArgs(engineArgs),
        worker.engine_args ?? {},
      )
      const nameChanged = name !== worker.name
      const trimmedWorkDir = workDir.trim()
      const workDirChanged = trimmedWorkDir !== (worker.work_dir ?? "")
      const workerChanged =
        nameChanged ||
        description !== (worker.description ?? "") ||
        workDirChanged ||
        engine !== pickDefaultEngine(worker.engine, enabledEngines) ||
        engineArgsChanged
      const deptsChanged = newDeptIds !== originalDeptIds

      const ops: Promise<unknown>[] = []
      if (workerChanged) {
        const data: Record<string, unknown> = { description, engine, engine_args: engineArgs }
        if (nameChanged) data.name = name
        if (workDirChanged) data.work_dir = trimmedWorkDir
        ops.push(updateWorker.mutateAsync({ id: worker.id, data }))
      }
      if (deptsChanged) {
        ops.push(setWorkerDepts.mutateAsync({ workerId: worker.id, departmentIds: [...selectedDeptIds] }))
      }
      await Promise.all(ops)
      onOpenChange(false)
    } catch (err) {
      setSubmitError(getErrorMessage(err))
    }
  }

  const isPending = updateWorker.isPending || setWorkerDepts.isPending

  const filteredDepts = deptSearch.trim()
    ? flatDepts.filter(({ dept }) =>
        dept.name.toLowerCase().includes(deptSearch.toLowerCase())
      )
    : flatDepts

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-[26rem] p-0 gap-0">
        <SheetHeader className="px-6 pt-6 pb-4">
          <SheetTitle>{t("workerDetail.workerInfo")}</SheetTitle>
        </SheetHeader>

        <Separator />

        <form
          id="edit-worker-info-form"
          onSubmit={handleSubmit}
          className="flex-1 overflow-y-auto"
        >
          <div className="px-6 py-5 space-y-5">
            {submitError && (
              <div role="alert" className="rounded-lg border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                {submitError}
              </div>
            )}

            <WorkerNameField
              id="ewis-name"
              open={open}
              value={name}
              onChange={setName}
              onError={setSubmitError}
            />

            <div className="space-y-1.5">
              <Label htmlFor="ewis-desc">{t("workers.form.description")}</Label>
              <Textarea
                id="ewis-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder={t("workers.form.descriptionPlaceholder")}
                rows={3}
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.descriptionHelper")}</p>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="ewis-workdir">{t("workers.form.workDir")}</Label>
              <Input
                id="ewis-workdir"
                value={workDir}
                onChange={(e) => setWorkDir(e.target.value)}
                placeholder={t("workers.form.workDirPlaceholder")}
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.workDirEditHelper")}</p>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="ewis-engine">{t("workers.form.engine")}</Label>
              <Select value={engine} onValueChange={(v) => v && setEngine(v as Engine)}>
                <SelectTrigger id="ewis-engine">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <EngineSelectItems engines={enabledEngines} />
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{t("workers.form.engineHelper")}</p>
            </div>

            <EngineArgsSection
              engines={[engine]}
              value={engineArgs}
              onChange={setEngineArgs}
            />
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
            disabled={isPending}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            form="edit-worker-info-form"
            disabled={isPending || !name.trim() || !workDir.trim()}
            className="flex-1"
          >
            {t("common.save")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
