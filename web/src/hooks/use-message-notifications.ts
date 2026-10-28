import { useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useActiveConversation, useChatEventSubscription } from "@/hooks/use-chat-events"
import { useConversations } from "@/hooks/use-conversations"
import { useNotificationSettings } from "@/hooks/use-notifications"
import { useWorkers } from "@/hooks/use-workers"
import { createMessageId } from "@/lib/chat-messages"
import { conversationPath } from "@/lib/conversations"
import {
  clearTabFocused,
  markTabFocused,
  notificationContent,
  otherTabFocused,
  shouldNotify,
  titleWithUnread,
  totalUnread,
  type PageAttention,
} from "@/lib/notifications"
import type { ConversationId, Worker } from "@/lib/types"

const ICON_URL = "/logo-mark.svg"

function pageAttention(): PageAttention {
  return { visible: document.visibilityState === "visible", focused: document.hasFocus() }
}

function inFront(): boolean {
  const { visible, focused } = pageAttention()
  return visible && focused
}

function useFocusedTab(): string {
  const [tabId] = useState(createMessageId)

  useEffect(() => {
    const sync = () => (inFront() ? markTabFocused(tabId) : clearTabFocused(tabId))
    const leave = () => clearTabFocused(tabId)
    sync()
    window.addEventListener("focus", sync)
    window.addEventListener("blur", leave)
    window.addEventListener("pagehide", leave)
    document.addEventListener("visibilitychange", sync)
    return () => {
      window.removeEventListener("focus", sync)
      window.removeEventListener("blur", leave)
      window.removeEventListener("pagehide", leave)
      document.removeEventListener("visibilitychange", sync)
      leave()
    }
  }, [tabId])

  return tabId
}

export function useMessageNotifications() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: workerList } = useWorkers()
  const workers = useMemo(() => new Map<string, Worker>((workerList ?? []).map((item) => [item.id, item])), [workerList])
  const { active: enabled } = useNotificationSettings()
  const { active: activeConversation } = useActiveConversation()
  const tabId = useFocusedTab()
  const shown = useRef(new Map<ConversationId, Set<Notification>>())

  useEffect(() => {
    const closeActive = () => {
      if (!activeConversation || !inFront()) return
      shown.current.get(activeConversation)?.forEach((notification) => notification.close())
      shown.current.delete(activeConversation)
    }
    closeActive()
    window.addEventListener("focus", closeActive)
    document.addEventListener("visibilitychange", closeActive)
    return () => {
      window.removeEventListener("focus", closeActive)
      document.removeEventListener("visibilitychange", closeActive)
    }
  }, [activeConversation])

  useChatEventSubscription({
    onMessage: (conversation, message) => {
      if (!enabled || !shouldNotify(conversation, message, pageAttention(), otherTabFocused(tabId))) return
      const content = notificationContent(conversation, message, workers, t)
      let notification: Notification
      try {
        notification = new Notification(content.title, { body: content.body, tag: content.tag, icon: ICON_URL })
      } catch {
        return
      }
      const open = shown.current.get(conversation) ?? new Set<Notification>()
      open.add(notification)
      shown.current.set(conversation, open)
      notification.onclose = () => open.delete(notification)
      notification.onclick = () => {
        window.focus()
        navigate(conversationPath(conversation))
        notification.close()
      }
    },
  })
}

export function useUnreadTitle() {
  const [base] = useState(() => document.title)
  const { data } = useConversations()
  const unread = totalUnread(data)

  useEffect(() => {
    document.title = titleWithUnread(base, unread)
  }, [base, unread])

  useEffect(
    () => () => {
      document.title = base
    },
    [base],
  )
}
