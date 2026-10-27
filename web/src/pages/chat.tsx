import { Link, Navigate, useLocation } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { ChevronLeftIcon } from "lucide-react"
import { BeeAvatar } from "@/components/chat/bee-avatar"
import { useChatRoute } from "@/components/chat/chat-shell"
import { WorkerAvatar } from "@/components/worker-avatar"
import { useMediaQuery } from "@/hooks/use-media-query"
import { useWorkers } from "@/hooks/use-workers"
import { BEE_CONVERSATION, conversationPath, workerIdOf } from "@/lib/conversations"

const DESKTOP_QUERY = "(min-width: 48rem)"

export function ChatIndex() {
  const desktop = useMediaQuery(DESKTOP_QUERY)
  const { search } = useLocation()
  if (!desktop) return null
  return <Navigate to={{ pathname: conversationPath(BEE_CONVERSATION), search }} replace />
}

export function Chat() {
  const { t } = useTranslation()
  const { conversation } = useChatRoute()
  const { data: workers } = useWorkers()
  const workerId = conversation ? workerIdOf(conversation) : null
  const worker = workerId ? workers?.find((item) => item.id === workerId) : undefined

  return (
    <header className="flex h-16 shrink-0 items-center gap-3 border-b border-border px-3 md:px-5">
      <Link
        to="/chat"
        aria-label={t("chat.back")}
        className="grid size-10 shrink-0 place-items-center rounded-control text-secondary transition-colors hover:bg-surface-muted hover:text-foreground md:hidden"
      >
        <ChevronLeftIcon size={20} strokeWidth={1.75} aria-hidden="true" />
      </Link>
      {conversation && (
        <>
          {worker ? <WorkerAvatar workerId={worker.id} name={worker.name} status={worker.status} /> : <BeeAvatar />}
          <h1 className="truncate text-base font-medium text-foreground">
            {worker ? worker.name : t("localChat.beeLabel")}
          </h1>
        </>
      )}
    </header>
  )
}
