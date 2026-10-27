import { Suspense, useEffect, useMemo } from "react"
import { Outlet, useMatch, useNavigate, useOutletContext } from "react-router-dom"
import { ConversationList } from "@/components/chat/conversation-list"
import { useWorkers } from "@/hooks/use-workers"
import { BEE_CONVERSATION, conversationPath } from "@/lib/conversations"
import { resolveConversationRoute } from "@/lib/routes"
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
    </div>
  )
}
