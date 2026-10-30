import { useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useQueryClient } from "@tanstack/react-query"
import { useActiveConversation, useChatEventSubscription } from "@/hooks/use-chat-events"
import { useConversations } from "@/hooks/use-conversations"
import { markNotificationsUnsupported, useNotificationSettings } from "@/hooks/use-notifications"
import { fetchWorkers, useWorkers } from "@/hooks/use-workers"
import { hasUnknownWorker } from "@/lib/chat-events"
import { conversationPath } from "@/lib/conversations"
import { isRoutedConversation } from "@/lib/routes"
import { notificationContent, shouldNotify, titleWithUnread, totalUnread, type PageAttention } from "@/lib/notifications"
import type { ChatMessage, ConversationId, Worker } from "@/lib/types"

const ICON_URL = "/notification-icon.png"
const FRONT_TAB_LOCK = "openbee-front-tab"

function pageAttention(): PageAttention {
  return { visible: document.visibilityState === "visible", focused: document.hasFocus() }
}

function inFront(): boolean {
  const { visible, focused } = pageAttention()
  return visible && focused
}

function workerMap(list: Worker[]): Map<string, Worker> {
  return new Map(list.map((item) => [item.id, item]))
}

function anotherTabInFront(): Promise<boolean> {
  if (!navigator.locks) return Promise.resolve(false)
  return navigator.locks.query().then(
    ({ held = [] }) => held.some((lock) => lock.name === FRONT_TAB_LOCK),
    () => false,
  )
}

function useFrontTabLock() {
  useEffect(() => {
    const locks = navigator.locks
    if (!locks) return
    let stop: (() => void) | undefined
    const leave = () => {
      stop?.()
      stop = undefined
    }
    const sync = () => {
      if (!inFront()) {
        leave()
        return
      }
      if (stop) return
      const controller = new AbortController()
      let release: (() => void) | undefined
      locks
        .request(FRONT_TAB_LOCK, { mode: "shared", signal: controller.signal }, () => new Promise<void>((resolve) => (release = resolve)))
        .catch(() => undefined)
      stop = () => {
        controller.abort()
        release?.()
      }
    }
    sync()
    window.addEventListener("focus", sync)
    window.addEventListener("blur", leave)
    document.addEventListener("visibilitychange", sync)
    return () => {
      window.removeEventListener("focus", sync)
      window.removeEventListener("blur", leave)
      document.removeEventListener("visibilitychange", sync)
      leave()
    }
  }, [])
}

export function useMessageNotifications() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data: workerList } = useWorkers()
  const workers = useMemo(() => workerMap(workerList ?? []), [workerList])
  const { active: enabled } = useNotificationSettings()
  const { active: activeConversation } = useActiveConversation()
  const shown = useRef(new Map<ConversationId, Set<Notification>>())
  useFrontTabLock()

  useEffect(() => {
    const closeActive = () => {
      if (!activeConversation || !inFront() || !isRoutedConversation(activeConversation)) return
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

  const resolveWorkers = async (conversation: ConversationId, message: ChatMessage) => {
    const event = { type: "message", seq: 0, conversation_id: conversation, message } as const
    if (!hasUnknownWorker(event, new Set(workers.keys()))) return workers
    return fetchWorkers(queryClient).then(workerMap, () => workers)
  }

  const show = (conversation: ConversationId, message: ChatMessage, known: Map<string, Worker>) => {
    const content = notificationContent(conversation, message, known, t)
    let notification: Notification
    try {
      notification = new Notification(content.title, { body: content.body, tag: content.tag, icon: ICON_URL })
    } catch (error) {
      if (error instanceof TypeError) markNotificationsUnsupported()
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
  }

  useChatEventSubscription({
    onMessage: (conversation, message) => {
      if (!enabled || !shouldNotify(conversation, message, pageAttention())) return
      void Promise.all([anotherTabInFront(), resolveWorkers(conversation, message)]).then(([elsewhere, known]) => {
        if (!elsewhere && !inFront()) show(conversation, message, known)
      })
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
