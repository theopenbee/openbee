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
import { useQueryClient, type QueryClient } from "@tanstack/react-query"
import { CONVERSATIONS_KEY } from "@/hooks/use-conversations"
import { tokenParam } from "@/lib/auth"
import {
  applyEventToConversations,
  applyWorkerStatus,
  detectGap,
  type ChatEvent,
  type ChatMessageEvent,
  type ConversationView,
  type TaskEvent,
} from "@/lib/chat-events"
import { config } from "@/lib/config"
import type { ChatMessage, ConversationId, ConversationSummary, Worker } from "@/lib/types"

const MAX_RECONNECT_ATTEMPTS = 5
const RECONNECT_BASE_DELAY_MS = 2000
const RECONNECT_MAX_DELAY_MS = 30_000

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

function updateWorkerCache(data: Worker | Worker[] | undefined, event: ChatEvent) {
  if (!data) return data
  return Array.isArray(data) ? applyWorkerStatus(data, event) : applyWorkerStatus([data], event)[0]
}

function updateConversationCache(
  queryClient: QueryClient,
  event: ChatMessageEvent | TaskEvent,
  view: ConversationView,
) {
  const list = queryClient.getQueryData<ConversationSummary[]>(CONVERSATIONS_KEY)
  if (!list) return
  if (!list.some((item) => item.id === event.conversation_id)) {
    queryClient.invalidateQueries({ queryKey: CONVERSATIONS_KEY })
    return
  }
  queryClient.setQueryData<ConversationSummary[]>(CONVERSATIONS_KEY, applyEventToConversations(list, event, view))
}

function applyToCaches(queryClient: QueryClient, event: ChatEvent, view: ConversationView) {
  switch (event.type) {
    case "message":
    case "task":
      updateConversationCache(queryClient, event, view)
      return
    case "worker_status":
      queryClient.setQueriesData<Worker | Worker[]>(
        { predicate: (query) => query.queryKey[0] === "workers" },
        (data) => updateWorkerCache(data, event),
      )
      return
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
      queryClient.invalidateQueries({ queryKey: CONVERSATIONS_KEY })
      for (const handler of handlersRef.current) handler.onResync?.()
    }

    const handle = (event: ChatEvent) => {
      if (detectGap(lastSeq, event.seq)) resync()
      lastSeq = event.seq
      applyToCaches(queryClient, event, {
        active: activeRef.current,
        visible: document.visibilityState === "visible",
      })
      notify(handlersRef.current, event)
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
        if (attempts >= MAX_RECONNECT_ATTEMPTS) return
        attempts += 1
        const delay = Math.min(RECONNECT_BASE_DELAY_MS * 2 ** (attempts - 1), RECONNECT_MAX_DELAY_MS)
        reconnectTimer = setTimeout(connect, delay)
      }
    }

    connect()

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
