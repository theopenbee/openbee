import { useTranslation } from "react-i18next"
import { ArrowRightIcon } from "lucide-react"
import { Avatar } from "@/components/arc/avatar/avatar"
import { Badge } from "@/components/arc/badge/badge"
import { WorkerAvatar } from "@/components/worker-avatar"
import { instructionPreview, taskTypeLabelKey } from "@/lib/chat-view"
import type { ConversationId, HandoffInfo, Worker } from "@/lib/types"

interface HandoffCardProps {
  info: HandoffInfo
  instruction: string
  workers: Map<string, Worker>
  onOpen: (conversation: ConversationId) => void
}

export function HandoffCard({ info, instruction, workers, onOpen }: HandoffCardProps) {
  const { t } = useTranslation()
  const worker = workers.get(info.worker_id)
  const name = worker?.name ?? t("chat.deletedWorker")
  const preview = instructionPreview(instruction)
  const typeKey = taskTypeLabelKey(info.task_type)

  return (
    <button
      type="button"
      disabled={!worker}
      onClick={() => onOpen(info.conversation_id)}
      className="group/handoff flex w-full max-w-md min-w-0 items-start gap-3 rounded-panel border border-border bg-surface px-4 py-3.5 text-left transition-colors duration-150 enabled:hover:border-border-strong enabled:hover:bg-surface-muted disabled:cursor-not-allowed disabled:opacity-60"
    >
      {worker ? (
        <WorkerAvatar workerId={worker.id} name={worker.name} status={worker.status} />
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
  )
}
