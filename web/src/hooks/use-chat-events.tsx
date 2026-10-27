import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { useQueryClient } from "@tanstack/react-query"
import { ensureFreshToken, tokenParam } from "@/lib/auth"
import { ingestChatEvent, reconnectDelay, resyncCaches, type ChatEvent, type TaskEvent } from "@/lib/chat-events"
import { config } from "@/lib/config"
import type { ChatMessage, ConversationId } from "@/lib/types"

export interface ChatEventHandler {
  onMessage?: (conversationId: ConversationId, message: ChatMessage) => void
  onTask?: (event: TaskEvent) => void
  onResync?: () => void
}

interface ActiveConversation {
  active: ConversationId | null
  setActive: (id: ConversationId | null) => void
}

type Subscribe = (handler: ChatEventHandler) => () => void

const SubscribeContext = createContext<Subscribe | null>(null)
const ActiveConversationContext = createContext<ActiveConversation | null>(null)

function parseEvent(data: string): ChatEvent | null {
  try {
    const event = JSON.parse(data)
    return event && typeof event.seq === "number" ? event : null
  } catch {
    return null
  }
}

function notify(handlers: Iterable<ChatEventHandler>, event: ChatEvent) {
  for (const handler of handlers) {
    if (event.type === "message") handler.onMessage?.(event.conversation_id, event.message)
    else if (event.type === "task") handler.onTask?.(event)
  }
}

export function ChatEventsProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const handlersRef = useRef(new Set<ChatEventHandler>())
  const activeRef = useRef<ConversationId | null>(null)
  const [active, setActiveState] = useState<ConversationId | null>(null)

  const subscribe = useCallback<Subscribe>((handler) => {
    handlersRef.current.add(handler)
    return () => {
      handlersRef.current.delete(handler)
    }
  }, [])

  const setActive = useCallback((id: ConversationId | null) => {
    activeRef.current = id
    setActiveState(id)
  }, [])

  const activeValue = useMemo(() => ({ active, setActive }), [active, setActive])

  useEffect(() => {
    let source: EventSource | undefined
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined
    let stopped = false
    let attempts = 0
    let lastSeq: number | null = null

    const resync = () => {
      lastSeq = null
      resyncCaches(queryClient)
      for (const handler of handlersRef.current) handler.onResync?.()
    }

    const handle = (event: ChatEvent) => {
      lastSeq = ingestChatEvent(
        queryClient,
        event,
        lastSeq,
        { active: activeRef.current, visible: document.visibilityState === "visible" },
        resync,
      )
      notify(handlersRef.current, event)
    }

    const open = () => {
      ensureFreshToken().then(connect, connect)
    }

    const connect = () => {
      if (stopped) return
      const es = new EventSource(`${config.apiUrl}/local/stream${tokenParam()}`)
      source = es

      es.onopen = () => {
        attempts = 0
        resync()
      }

      es.onmessage = (message) => {
        const event = parseEvent(message.data)
        if (event) handle(event)
      }

      es.onerror = () => {
        es.close()
        clearTimeout(reconnectTimer)
        attempts += 1
        reconnectTimer = setTimeout(open, reconnectDelay(attempts))
      }
    }

    open()

    return () => {
      stopped = true
      clearTimeout(reconnectTimer)
      source?.close()
    }
  }, [queryClient])

  return (
    <SubscribeContext.Provider value={subscribe}>
      <ActiveConversationContext.Provider value={activeValue}>{children}</ActiveConversationContext.Provider>
    </SubscribeContext.Provider>
  )
}

export function useChatEventSubscription(handler: ChatEventHandler) {
  const subscribe = useContext(SubscribeContext)
  if (!subscribe) throw new Error("useChatEventSubscription must be used within ChatEventsProvider")
  const handlerRef = useRef(handler)

  useEffect(() => {
    handlerRef.current = handler
  })

  useEffect(
    () =>
      subscribe({
        onMessage: (conversationId, message) => handlerRef.current.onMessage?.(conversationId, message),
        onTask: (event) => handlerRef.current.onTask?.(event),
        onResync: () => handlerRef.current.onResync?.(),
      }),
    [subscribe],
  )
}

export function useActiveConversation(): ActiveConversation {
  const value = useContext(ActiveConversationContext)
  if (!value) throw new Error("useActiveConversation must be used within ChatEventsProvider")
  return value
}
