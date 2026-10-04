import { useState, useEffect, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { useUpdateWorker } from "@/hooks/use-workers"
import { useFlatDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { useEnabledEngines } from "@/hooks/use-config"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
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
} from "@/components/ui/select"
import { EngineSelectItems, EngineSelectValue } from "@/components/engine-select-items"
import { EngineArgsSection } from "@/components/engine-args-section"
import { DepartmentChecklist } from "@/components/department-checklist"
import { SectionHeading } from "@/components/section-heading"
import { WorkerNameField } from "@/components/worker-name-field"
import { getErrorMessage } from "@/lib/utils"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
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
  const [submitError, setSubmitError] = useState("")

  useEffect(() => {
    if (open) {
      setName(worker.name ?? "")
      setDescription(worker.description ?? "")
      setWorkDir(worker.work_dir ?? "")
      setEngine(pickDefaultEngine(worker.engine, enabledEngines))
      setSelectedDeptIds(new Set(worker.departments?.map((d) => d.id) ?? []))
      setEngineArgs(worker.engine_args ?? {})
      setSubmitError("")
    }
  }, [open, worker, enabledEngines])

  const trimmedWorkDir = workDir.trim()

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

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="gap-0 p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-[28rem]">
        <SheetHeader className="border-b border-border px-6 py-4 pr-12">
          <SheetTitle className="text-base font-semibold text-strong">{t("workerDetail.workerInfo")}</SheetTitle>
        </SheetHeader>

        <form
          id="edit-worker-info-form"
          onSubmit={handleSubmit}
          className="flex-1 overflow-y-auto"
        >
          <div className="space-y-5 px-6 py-5">
            {submitError && (
              <div role="alert" className={ALERT_DESTRUCTIVE}>
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
                className="font-mono text-base md:text-body-sm"
              />
              <p className="text-xs text-muted-foreground">{t("workers.form.workDirEditHelper")}</p>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="ewis-engine">{t("workers.form.engine")}</Label>
              <Select value={engine} onValueChange={(v) => v && setEngine(v as Engine)}>
                <SelectTrigger id="ewis-engine" className="w-full">
                  <EngineSelectValue />
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
            <div className="space-y-3 border-t border-hairline px-6 py-5">
              <SectionHeading
                text={t("workers.form.sectionDepartment")}
                badge={selectedDeptIds.size}
              />

              <DepartmentChecklist selected={selectedDeptIds} onChange={setSelectedDeptIds} />

              <p className="text-xs text-muted-foreground">{t("workers.form.departmentHelper")}</p>
            </div>
          )}
        </form>

        <SheetFooter className="mt-0 flex-row justify-end gap-2 border-t border-border bg-elevated px-6 py-3">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={isPending}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            form="edit-worker-info-form"
            disabled={isPending || !name.trim() || !trimmedWorkDir}
          >
            {t("common.save")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
