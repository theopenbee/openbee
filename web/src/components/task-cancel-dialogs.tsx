import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import type { Task } from "@/lib/types"

interface CancelTaskDialogProps {
  open: boolean
  task: Task | null
  pending: boolean
  error: Error | null
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}

export function CancelTaskDialog({ open, task, pending, error, onOpenChange, onConfirm }: CancelTaskDialogProps) {
  const { t } = useTranslation()

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!pending) onOpenChange(next) }}>
      <DialogContent
        title={t("tasks.cancelConfirmTitle")}
        description={t("tasks.cancelConfirmDescription")}
      >
        {task && (
          <p className="line-clamp-4 rounded-inset bg-surface-muted px-4 py-3 text-sm break-words whitespace-pre-wrap text-foreground">
            {task.instruction}
          </p>
        )}
        {error && (
          <div className="mt-4">
            <Alert tone="danger" title={t("tasks.cancelError")}>{error.message}</Alert>
          </div>
        )}
        <div className="mt-6 flex flex-wrap justify-end gap-2">
          <Button variant="secondary" disabled={pending} onClick={() => onOpenChange(false)}>
            {t("tasks.keepTask")}
          </Button>
          <Button variant="danger" loading={pending} onClick={onConfirm}>
            {t("tasks.cancelTask")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

interface CancelAllTasksDialogProps {
  open: boolean
  workerName: string
  count: number
  pending: boolean
  error: Error | null
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}

export function CancelAllTasksDialog({ open, workerName, count, pending, error, onOpenChange, onConfirm }: CancelAllTasksDialogProps) {
  const { t } = useTranslation()

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!pending) onOpenChange(next) }}>
      <DialogContent
        title={t("tasks.cancelAllConfirmTitle", { name: workerName })}
        description={t("tasks.cancelAllConfirmDescription", { count })}
      >
        {error && (
          <div className="mb-6">
            <Alert tone="danger" title={t("tasks.cancelAllError")}>{error.message}</Alert>
          </div>
        )}
        <div className="flex flex-wrap justify-end gap-2">
          <Button variant="secondary" disabled={pending} onClick={() => onOpenChange(false)}>
            {t("tasks.keepTasks")}
          </Button>
          <Button variant="danger" loading={pending} onClick={onConfirm}>
            {t("tasks.cancelAll")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
