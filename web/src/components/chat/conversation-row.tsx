import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Badge } from "@/components/arc/badge/badge"
import { BeeAvatar } from "@/components/chat/bee-avatar"
import { WorkerAvatar } from "@/components/worker-avatar"
import { conversationPath } from "@/lib/conversations"
import { formatListTime } from "@/lib/format"
import type { ConversationSummary, Worker } from "@/lib/types"
import { cn } from "@/lib/utils"

const MAX_UNREAD = 99

interface ConversationRowProps {
  conversation: ConversationSummary
  worker?: Worker
  active: boolean
  now: number
}

export function ConversationRow({ conversation, worker, active, now }: ConversationRowProps) {
  const { t, i18n } = useTranslation()
  const last = conversation.last_message
  const name = worker ? worker.name : t("localChat.beeLabel")
  const preview = last
    ? `${last.sender.type === "user" ? t("chat.youPrefix") : ""}${last.preview}`
    : (worker?.description ?? "")
  const unread = conversation.unread > MAX_UNREAD ? `${MAX_UNREAD}+` : String(conversation.unread)

  return (
    <Link
      to={conversationPath(conversation.id)}
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex min-w-0 items-center gap-3 rounded-control px-3 py-2.5 transition-colors duration-150 hover:bg-surface-muted",
        active && "bg-accent-subtle hover:bg-accent-subtle",
      )}
    >
      {worker ? <WorkerAvatar workerId={worker.id} name={worker.name} status={worker.status} /> : <BeeAvatar />}
      <span className="grid min-w-0 flex-1 gap-0.5">
        <span className="flex min-w-0 items-baseline gap-2">
          <span className="truncate text-sm font-medium text-foreground">{name}</span>
          {last && (
            <time
              dateTime={new Date(last.ts).toISOString()}
              className="ml-auto shrink-0 text-xs text-muted tabular"
            >
              {formatListTime(last.ts, now, i18n.language, t("chat.yesterday"))}
            </time>
          )}
        </span>
        <span className="flex h-5 min-w-0 items-center gap-2">
          <span className={cn("truncate text-sm", last ? "text-secondary" : "text-muted")}>{preview}</span>
          {conversation.unread > 0 && (
            <Badge tone="info" size="sm" className="ml-auto shrink-0 tabular">
              {unread}
            </Badge>
          )}
        </span>
      </span>
    </Link>
  )
}
