import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Checkbox } from "@/components/arc/checkbox/checkbox"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { Input } from "@/components/arc/input/input"
import { useDeleteWorker } from "@/hooks/use-workers"
import type { Worker } from "@/lib/types"
import { getErrorMessage } from "@/lib/utils"

export type DeleteTarget = Pick<Worker, "id" | "name" | "work_dir">

interface DeleteWorkerDialogProps {
  target: DeleteTarget | null
  onClose: () => void
  onDeleted: () => void
}

export function DeleteWorkerDialog({ target, onClose, onDeleted }: DeleteWorkerDialogProps) {
  const { t } = useTranslation()
  const deleteWorker = useDeleteWorker()
  const { reset } = deleteWorker
  const inputRef = useRef<HTMLInputElement>(null)
  const [confirmation, setConfirmation] = useState("")
  const [deleteWorkDir, setDeleteWorkDir] = useState(false)
  const [lastTarget, setLastTarget] = useState(target)
  if (target && target !== lastTarget) {
    setLastTarget(target)
    setConfirmation("")
    setDeleteWorkDir(false)
  }
  useEffect(() => {
    if (target) reset()
  }, [target, reset])
  const shown = target ?? lastTarget
  const confirmed = !!shown && confirmation === shown.name

  const handleConfirm = async () => {
    if (!shown || !confirmed) return
    try {
      await deleteWorker.mutateAsync({ id: shown.id, deleteWorkDir })
      onDeleted()
    } catch {
      return
    }
  }

  return (
    <Dialog
      open={!!target}
      onOpenChange={(open) => {
        if (!open && !deleteWorker.isPending) onClose()
      }}
    >
      <DialogContent
        title={t("workers.deleteDialog.title", { name: shown?.name ?? "" })}
        description={t("workers.deleteDialog.description")}
        onOpenAutoFocus={(event) => {
          event.preventDefault()
          inputRef.current?.focus()
        }}
      >
        <form
          className="grid gap-4"
          onSubmit={(event) => {
            event.preventDefault()
            void handleConfirm()
          }}
        >
          <Alert tone="warning" title={t("workers.deleteDialog.chatWarning")} />
          <Input
            ref={inputRef}
            label={t("workers.deleteDialog.confirmNameLabel", { name: shown?.name ?? "" })}
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            placeholder={t("workers.deleteDialog.confirmNamePlaceholder")}
            autoComplete="off"
            spellCheck={false}
            disabled={deleteWorker.isPending}
          />
          <Checkbox
            label={t("workers.deleteDialog.deleteWorkDir")}
            description={
              shown?.work_dir ? t("workers.deleteDialog.deleteWorkDirHint", { path: shown.work_dir }) : undefined
            }
            checked={deleteWorkDir}
            onCheckedChange={(state) => setDeleteWorkDir(state === true)}
            disabled={deleteWorker.isPending}
          />
          {deleteWorker.error && (
            <Alert tone="danger" title={t("workers.deleteDialog.error")}>
              {getErrorMessage(deleteWorker.error)}
            </Alert>
          )}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={onClose} disabled={deleteWorker.isPending}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="danger" loading={deleteWorker.isPending} disabled={!confirmed}>
              {t("workers.deleteDialog.confirm")}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}
