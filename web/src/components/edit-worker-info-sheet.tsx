import { useState, useEffect, type CSSProperties, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { useUpdateWorker } from "@/hooks/use-workers"
import { useFlatDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { useEnabledEngines } from "@/hooks/use-config"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Drawer, DrawerContent } from "@/components/arc/drawer/drawer"
import { Input } from "@/components/arc/input/input"
import { Select } from "@/components/arc/select/select"
import { Textarea } from "@/components/arc/textarea/textarea"
import { useEngineOptions } from "@/components/engine-select-items"
import { EngineArgsSection } from "@/components/engine-args-section"
import { SectionHeading } from "@/components/section-heading"
import { WorkerDepartmentPicker } from "@/components/worker-department-picker"
import { WorkerNameField } from "@/components/worker-name-field"
import { getErrorMessage } from "@/lib/utils"
import { engineArgsEqual, stripEmptyEngineArgs } from "@/lib/engine-args"
import type { Worker, Engine } from "@/lib/types"
import { DEFAULT_ENGINE, pickDefaultEngine } from "@/lib/types"

const DRAWER_STYLE = { "--drawer-size": "min(34rem, calc(100vw - 1rem))" } as CSSProperties

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
  const engineOptions = useEngineOptions(enabledEngines)

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
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent
        side="right"
        title={t("workerDetail.editInfoTitle")}
        description={worker.name}
        style={DRAWER_STYLE}
      >
        <form id="edit-worker-info-form" onSubmit={handleSubmit} className="relative grid gap-8">
          <div className="grid gap-5">
            <WorkerNameField
              id="ewis-name"
              open={open}
              value={name}
              onChange={setName}
              onError={setSubmitError}
              disabled={isPending}
            />

            <Textarea
              id="ewis-desc"
              label={t("workers.form.description")}
              description={t("workers.form.descriptionHelper")}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t("workers.form.descriptionPlaceholder")}
              rows={3}
              disabled={isPending}
            />

            <div className="grid min-w-0">
              <Input
                id="ewis-workdir"
                label={t("workers.form.workDir")}
                value={workDir}
                onChange={(e) => setWorkDir(e.target.value)}
                placeholder={t("workers.form.workDirPlaceholder")}
                autoComplete="off"
                spellCheck={false}
                required
                disabled={isPending}
                aria-describedby="ewis-workdir-hint"
              />
              <p id="ewis-workdir-hint" className="pt-2 text-xs text-muted">
                {t("workers.form.workDirEditHelper")}
              </p>
            </div>

            <Select
              id="ewis-engine"
              label={t("workers.form.engine")}
              description={t("workers.form.engineHelper")}
              options={engineOptions}
              value={engine}
              onValueChange={(v) => v && setEngine(v as Engine)}
              disabled={isPending}
            />

            <EngineArgsSection
              engines={[engine]}
              value={engineArgs}
              onChange={setEngineArgs}
              idPrefix="ewis-engine-args"
              disabled={isPending}
            />
          </div>

          {flatDepts.length > 0 && (
            <section className="grid gap-3 border-t border-border-subtle pt-6" aria-label={t("workers.form.sectionDepartment")}>
              <div className="grid gap-1">
                <SectionHeading text={t("workers.form.sectionDepartment")} badge={selectedDeptIds.size} />
                <p className="text-sm text-secondary">{t("workers.form.departmentHelper")}</p>
              </div>
              <WorkerDepartmentPicker
                selected={selectedDeptIds}
                onChange={setSelectedDeptIds}
                disabled={isPending}
              />
            </section>
          )}

          {submitError && (
            <Alert tone="danger" title={t("workers.form.saveError")}>
              {submitError}
            </Alert>
          )}

          <div className="sticky -bottom-5 -mx-5 -mb-5 flex justify-end gap-2 border-t border-border-subtle bg-surface-raised px-5 py-4 sm:-bottom-6 sm:-mx-6 sm:-mb-6 sm:px-6">
            <Button
              type="button"
              variant="secondary"
              onClick={() => onOpenChange(false)}
              disabled={isPending}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              loading={isPending}
              disabled={!name.trim() || !trimmedWorkDir}
            >
              {t("workerDetail.saveInfo")}
            </Button>
          </div>
        </form>
      </DrawerContent>
    </Drawer>
  )
}
