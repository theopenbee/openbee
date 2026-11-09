import { Suspense } from "react"
import { Outlet } from "react-router-dom"
import { ChatEventsProvider } from "@/hooks/use-chat-events"
import { useMessageNotifications, useUnreadTitle } from "@/hooks/use-message-notifications"

function ChatAttention() {
  useMessageNotifications()
  useUnreadTitle()
  return null
}

export function Layout() {
  return (
    <ChatEventsProvider>
      <Suspense fallback={null}>
        <Outlet />
      </Suspense>
      <ChatAttention />
    </ChatEventsProvider>
  )
}
