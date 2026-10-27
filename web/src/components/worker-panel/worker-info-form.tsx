import { useId, useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Input } from "@/components/arc/input/input"
import { Select } from "@/components/arc/select/select"
import { Textarea } from "@/components/arc/textarea/textarea"
import { EngineArgsSection } from "@/components/engine-args-section"
import { useEngineOptions } from "@/components/engine-select-items"
import { SectionHeading } from "@/components/section-heading"
import { WorkerDepartmentPicker } from "@/components/worker-department-picker"
import { WorkerNameField } from "@/components/worker-name-field"
import { useEnabledEngines } from "@/hooks/use-config"
import { useFlatDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { useUpdateWorker } from "@/hooks/use-workers"
import { engineArgsEqual, stripEmptyEngineArgs } from "@/lib/engine-args"
import { pickDefaultEngine, type Engine, type Worker } from "@/lib/types"
import { getErrorMessage } from "@/lib/utils"

interface WorkerInfoFormProps {
  worker: Worker
  onDone: () => void
}

export function WorkerInfoForm({ worker, onDone }: WorkerInfoFormProps) {
  const { t } = useTranslation()
  const headingId = useId()
  const updateWorker = useUpdateWorker()
  const setWorkerDepts = useSetWorkerDepartments()
  const flatDepts = useFlatDepartments()
  const enabledEngines = useEnabledEngines()
  const engineOptions = useEngineOptions(enabledEngines)

  const [name, setName] = useState(worker.name ?? "")
  const [description, setDescription] = useState(worker.description ?? "")
  const [workDir, setWorkDir] = useState(worker.work_dir ?? "")
  const [engine, setEngine] = useState<Engine>(() => pickDefaultEngine(worker.engine, enabledEngines))
  const [selectedDeptIds, setSelectedDeptIds] = useState<Set<string>>(
    () => new Set(worker.departments?.map((d) => d.id) ?? []),
  )
  const [engineArgs, setEngineArgs] = useState<Record<string, string>>(worker.engine_args ?? {})
  const [submitError, setSubmitError] = useState("")

  const trimmedWorkDir = workDir.trim()

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setSubmitError("")
    try {
      const originalDeptIds = worker.departments?.map((d) => d.id).sort().join(",") ?? ""
      const newDeptIds = [...selectedDeptIds].sort().join(",")
      const engineArgsChanged = !engineArgsEqual(stripEmptyEngineArgs(engineArgs), worker.engine_args ?? {})
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
      onDone()
    } catch (err) {
      setSubmitError(getErrorMessage(err))
    }
  }

  const isPending = updateWorker.isPending || setWorkerDepts.isPending

  return (
    <form onSubmit={handleSubmit} className="grid grid-cols-[minmax(0,1fr)] gap-8" aria-labelledby={headingId}>
      <div className="grid gap-5">
        <h4 id={headingId} className="text-base font-medium text-foreground">
          {t("workerDetail.editInfoTitle")}
        </h4>
        <WorkerNameField
          id="ewis-name"
          open
          value={name}
          onChange={setName}
          onError={setSubmitError}
          autoFocus
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
          <WorkerDepartmentPicker selected={selectedDeptIds} onChange={setSelectedDeptIds} disabled={isPending} />
        </section>
      )}

      {submitError && (
        <Alert tone="danger" title={t("workers.form.saveError")}>
          {submitError}
        </Alert>
      )}

      <div className="flex justify-end gap-2 border-t border-border-subtle pt-6">
        <Button type="button" variant="secondary" onClick={onDone} disabled={isPending}>
          {t("common.cancel")}
        </Button>
        <Button type="submit" loading={isPending} disabled={!name.trim() || !trimmedWorkDir}>
          {t("workerDetail.saveInfo")}
        </Button>
      </div>
    </form>
  )
}
