import { useTranslation } from "react-i18next"
import { ArrowRightIcon, CheckIcon, ChevronRightIcon, CircleAlertIcon, LoaderCircleIcon } from "lucide-react"
import { Avatar } from "@/components/arc/avatar/avatar"
import { Badge } from "@/components/arc/badge/badge"
import { WorkerAvatar } from "@/components/worker-avatar"
import { instructionPreview, taskTypeLabelKey } from "@/lib/chat-view"
import { handoffRunLabel } from "@/lib/execution"
import type { ConversationId, ExecutionSummary, HandoffInfo, Worker } from "@/lib/types"
import { cn } from "@/lib/utils"

interface HandoffCardProps {
  info: HandoffInfo
  instruction: string
  workers: Map<string, Worker>
  onOpen: (conversation: ConversationId) => void
  onShowExecution?: (executionId: string) => void
}

export function HandoffCard({ info, instruction, workers, onOpen, onShowExecution }: HandoffCardProps) {
  const { t } = useTranslation()
  const worker = workers.get(info.worker_id)
  const name = worker?.name ?? t("chat.deletedWorker")
  const preview = instructionPreview(instruction)
  const typeKey = taskTypeLabelKey(info.task_type)

  return (
    <div className="w-full max-w-md min-w-0 overflow-hidden rounded-panel border border-border bg-surface">
      <button
        type="button"
        disabled={!worker}
        onClick={() => onOpen(info.conversation_id)}
        className="group/handoff flex w-full min-w-0 items-start gap-3 px-4 py-3.5 text-left transition-colors duration-150 enabled:hover:bg-surface-muted disabled:cursor-not-allowed disabled:opacity-60"
      >
        {worker ? (
          <WorkerAvatar workerId={worker.id} name={worker.name} />
        ) : (
          <Avatar name="?" aria-hidden="true" />
        )}
        <span className="grid min-w-0 flex-1 gap-1">
          <span className="flex min-w-0 items-center gap-1.5 text-sm font-medium text-foreground">
            <span className="truncate">{t("chat.handedOffTo", { name })}</span>
            {worker && (
              <ArrowRightIcon
                size={16}
                strokeWidth={1.75}
                aria-hidden="true"
                className="shrink-0 text-secondary transition-transform duration-150 group-hover/handoff:translate-x-0.5"
              />
            )}
          </span>
          {preview && <span className="line-clamp-3 whitespace-pre-line text-sm text-secondary">{preview}</span>}
          {typeKey && (
            <span className="mt-1 flex min-w-0 flex-wrap items-center gap-2">
              <Badge tone="neutral" size="sm">
                {t(typeKey)}
              </Badge>
              {info.cron_expr && (
                <code className="truncate font-mono text-xs text-secondary">{info.cron_expr}</code>
              )}
            </span>
          )}
        </span>
      </button>
      {info.run && onShowExecution && (
        <RunRow run={info.run} latest={info.task_type === "scheduled"} onShowExecution={onShowExecution} />
      )}
    </div>
  )
}

function RunStatusIcon({ status }: { status: ExecutionSummary["status"] }) {
  if (status === "running") {
    return <LoaderCircleIcon size={14} strokeWidth={1.75} className="shrink-0 animate-spin" aria-hidden="true" />
  }
  if (status === "completed") {
    return <CheckIcon size={14} strokeWidth={2} className="shrink-0 text-success" aria-hidden="true" />
  }
  return <CircleAlertIcon size={14} strokeWidth={1.75} className="shrink-0 text-danger" aria-hidden="true" />
}

function RunRow({
  run,
  latest,
  onShowExecution,
}: {
  run: ExecutionSummary
  latest: boolean
  onShowExecution: (executionId: string) => void
}) {
  const { t } = useTranslation()
  return (
    <button
      type="button"
      onClick={() => onShowExecution(run.id)}
      className="flex h-11 w-full min-w-0 items-center gap-2 border-t border-border px-4 text-left text-xs text-secondary transition-colors duration-150 hover:bg-surface-muted hover:text-foreground"
    >
      <RunStatusIcon status={run.status} />
      <span className={cn("min-w-0 truncate", run.status === "failed" && "text-danger")}>
        {handoffRunLabel(run, latest, t)}
      </span>
      <span className="ml-auto inline-flex shrink-0 items-center gap-0.5 font-medium text-foreground">
        {t("execution.show")}
        <ChevronRightIcon size={14} strokeWidth={1.75} aria-hidden="true" />
      </span>
    </button>
  )
}
