import { Navigate, useLocation } from "react-router-dom"
import { ChatHeaderPlaceholder } from "@/components/chat/chat-header"
import { useChatRoute } from "@/components/chat/chat-shell"
import { ChatSkeleton } from "@/components/chat/chat-skeleton"
import { ChatView } from "@/components/chat/chat-view"
import { useMediaQuery } from "@/hooks/use-media-query"
import { BEE_CONVERSATION, conversationPath } from "@/lib/conversations"
import { CHAT_COLUMN } from "@/lib/styles"
import { cn } from "@/lib/utils"

const DESKTOP_QUERY = "(min-width: 48rem)"

export function ChatIndex() {
  const desktop = useMediaQuery(DESKTOP_QUERY)
  const { search } = useLocation()
  if (!desktop) return null
  return <Navigate to={{ pathname: conversationPath(BEE_CONVERSATION), search }} replace />
}

export function Chat() {
  const { conversation } = useChatRoute()

  if (!conversation) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <ChatHeaderPlaceholder />
        <div className={cn(CHAT_COLUMN, "pt-6 md:pt-10")}>
          <ChatSkeleton />
        </div>
      </div>
    )
  }

  return <ChatView key={conversation} conversation={conversation} />
}
