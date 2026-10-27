import { useMemo, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { PlusIcon } from "lucide-react"
import { Button } from "@/components/arc/button/button"
import { SearchField } from "@/components/arc/search-field/search-field"
import { Tooltip } from "@/components/arc/tooltip/tooltip"
import { LogoFull } from "@/components/brand/logo"
import { ConversationRow } from "@/components/chat/conversation-row"
import { NavUser } from "@/components/nav-user"
import { useNow } from "@/components/plugin-ui"
import { SettingsButton } from "@/components/settings/settings-button"
import { useConversations } from "@/hooks/use-conversations"
import { useWorkers } from "@/hooks/use-workers"
import { BEE_CONVERSATION, filterConversations, sortConversations } from "@/lib/conversations"
import { panelToSearch } from "@/lib/routes"
import type { ConversationId, Worker } from "@/lib/types"
import { cn } from "@/lib/utils"

const CLOCK_INTERVAL_MS = 60_000
const SKELETON_ROWS = 4

interface ConversationListProps {
  active: ConversationId | null
  className?: string
}

export function ConversationList({ active, className }: ConversationListProps) {
  const { t } = useTranslation()
  const [, setSearchParams] = useSearchParams()
  const [query, setQuery] = useState("")
  const now = useNow(true, CLOCK_INTERVAL_MS)
  const conversations = useConversations()
  const workers = useWorkers()

  const workerMap = useMemo(
    () => new Map<string, Worker>((workers.data ?? []).map((worker) => [worker.id, worker])),
    [workers.data],
  )

  const rows = useMemo(() => {
    const names = new Map(Array.from(workerMap.values(), (worker) => [worker.id, worker.name]))
    const listed = (conversations.data ?? []).filter(
      (item) => item.id === BEE_CONVERSATION || (item.worker_id !== undefined && workerMap.has(item.worker_id)),
    )
    return filterConversations(sortConversations(listed, names), query, workerMap)
  }, [conversations.data, workerMap, query])

  const openNewWorker = () => setSearchParams((current) => panelToSearch({ mode: "new" }, current))
  const loading = conversations.isPending || workers.isPending
  const failed = (conversations.isError && !conversations.data) || (workers.isError && !workers.data)
  const retry = () => {
    if (conversations.isError) conversations.refetch()
    if (workers.isError) workers.refetch()
  }
  const newWorkerLabel = t("chat.newWorker")

  return (
    <aside className={cn("h-full min-h-0 flex-col bg-background", className)}>
      <div className="flex h-16 shrink-0 items-center justify-between gap-2 pr-3 pl-5">
        <Link to="/" aria-label="OpenBee" className="flex items-center rounded-control">
          <LogoFull className="h-7 w-auto" />
        </Link>
        <Tooltip content={newWorkerLabel}>
          <Button variant="ghost" size="sm" aria-label={newWorkerLabel} onClick={openNewWorker}>
            <PlusIcon size={18} strokeWidth={1.75} aria-hidden="true" />
          </Button>
        </Tooltip>
      </div>
      <div className="shrink-0 px-3 pb-3 [&_label]:sr-only">
        <SearchField label={t("chat.search")} placeholder={t("chat.search")} value={query} onValueChange={setQuery} />
      </div>
      <nav aria-label={t("chat.title")} className="relative min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 pb-3">
        {loading ? (
          <ListSkeleton />
        ) : failed ? (
          <div className="grid justify-items-start gap-3 px-3 py-4">
            <p className="text-sm text-secondary">{t("common.loadError")}</p>
            <Button variant="secondary" size="sm" onClick={retry}>
              {t("common.retry")}
            </Button>
          </div>
        ) : (
          <>
            <ul className="grid gap-0.5">
              {rows.map((item) => (
                <li key={item.id}>
                  <ConversationRow conversation={item} workers={workerMap} active={item.id === active} now={now} />
                </li>
              ))}
            </ul>
            {workerMap.size === 0 && (
              <div className="grid justify-items-start gap-3 px-3 py-4">
                <p className="text-sm text-secondary">{t("chat.noWorkersHint")}</p>
                <Button variant="secondary" size="sm" onClick={openNewWorker}>
                  <PlusIcon size={16} strokeWidth={1.75} aria-hidden="true" />
                  {newWorkerLabel}
                </Button>
              </div>
            )}
          </>
        )}
      </nav>
      <div className="flex shrink-0 items-center justify-between gap-2 border-t border-border-subtle px-3 py-3">
        <NavUser showName align="start" />
        <SettingsButton />
      </div>
    </aside>
  )
}

function ListSkeleton() {
  return (
    <div aria-busy="true" className="grid gap-0.5">
      {Array.from({ length: SKELETON_ROWS }, (_, index) => (
        <div key={index} className="flex items-center gap-3 px-3 py-2.5">
          <div className="skeleton size-9 shrink-0 rounded-full" />
          <div className="grid min-w-0 flex-1 gap-2">
            <div className="skeleton h-3.5 w-24" />
            <div className="skeleton h-3 w-40 max-w-full" />
          </div>
        </div>
      ))}
    </div>
  )
}
