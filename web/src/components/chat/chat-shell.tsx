import { Suspense, useEffect, useMemo } from "react"
import { Outlet, useMatch, useNavigate, useOutletContext, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { ConversationList } from "@/components/chat/conversation-list"
import { SplitPanel } from "@/components/split-panel"
import { WorkerCreateForm } from "@/components/worker-panel/worker-create-form"
import { WorkerPanel } from "@/components/worker-panel/worker-panel"
import { useMessageNotifications, useUnreadTitle } from "@/hooks/use-message-notifications"
import { useWorkers } from "@/hooks/use-workers"
import { BEE_CONVERSATION, conversationPath } from "@/lib/conversations"
import {
  panelFromSearch,
  panelReplacesHistory,
  panelToSearch,
  panelView,
  resolveConversationRoute,
  workerPanelPath,
  type PanelState,
} from "@/lib/routes"
import type { ConversationId } from "@/lib/types"
import { cn } from "@/lib/utils"

export interface ChatRouteContext {
  conversation: ConversationId | null
}

export function useChatRoute() {
  return useOutletContext<ChatRouteContext>()
}

function useConversationRoute(param: string | undefined): ConversationId | null {
  const navigate = useNavigate()
  const { data: workers, refetch } = useWorkers()
  const workerIds = useMemo(() => new Set((workers ?? []).map((worker) => worker.id)), [workers])
  const conversation = resolveConversationRoute(param, workerIds)
  const unknownParam = param !== undefined && workers !== undefined && conversation === null ? param : null

  useEffect(() => {
    if (unknownParam === null) return
    let current = true
    refetch().then((result) => {
      if (!current || result.isError) return
      const ids = new Set((result.data ?? []).map((worker) => worker.id))
      if (resolveConversationRoute(unknownParam, ids) === null) {
        navigate(conversationPath(BEE_CONVERSATION), { replace: true })
      }
    })
    return () => {
      current = false
    }
  }, [unknownParam, refetch, navigate])

  return conversation
}

export function ChatShell() {
  const listRoute = useMatch("/chat") !== null
  const param = useMatch("/chat/:conversation")?.params.conversation
  const conversation = useConversationRoute(param)
  const context = useMemo<ChatRouteContext>(() => ({ conversation }), [conversation])
  useMessageNotifications()
  useUnreadTitle()

  return (
    <div className="flex h-dvh bg-background">
      <ConversationList
        active={conversation}
        className={cn("w-full shrink-0 md:flex md:w-76 md:border-r md:border-border", listRoute ? "flex" : "hidden")}
      />
      <main className={cn("min-h-0 min-w-0 flex-1 flex-col", listRoute ? "hidden md:flex" : "flex")}>
        <Suspense fallback={null}>
          <Outlet context={context} />
        </Suspense>
      </main>
      <ChatPanel param={param} />
    </div>
  )
}

function ChatPanel({ param }: { param: string | undefined }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const panel = panelFromSearch(searchParams)
  const view = panelView(param, panel)

  const changePanel = (next: PanelState) =>
    setSearchParams((current) => panelToSearch(next, current), { replace: panelReplacesHistory(panel, next) })

  const title =
    view?.mode === "new" ? t(view.copyFrom ? "workers.copyWorker" : "chat.newWorker") : t("chat.panelTitle")

  return (
    <SplitPanel
      open={view !== null}
      onOpenChange={(open) => {
        if (!open) changePanel(null)
      }}
      title={title}
    >
      {view?.mode === "worker" && (
        <WorkerPanel key={view.workerId} workerId={view.workerId} panel={panel} onPanelChange={changePanel} />
      )}
      {view?.mode === "new" && (
        <WorkerCreateForm
          key={view.copyFrom ?? ""}
          copyFrom={view.copyFrom}
          onCreated={(worker) => navigate(workerPanelPath(worker.id), { replace: true })}
          onCancel={() => changePanel(null)}
        />
      )}
    </SplitPanel>
  )
}
