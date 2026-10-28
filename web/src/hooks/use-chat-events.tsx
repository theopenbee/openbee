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
import { ingestChatEvent, reconnectDelay, resyncCaches, type ChatEvent } from "@/lib/chat-events"
import { config } from "@/lib/config"
import type { ChatMessage, ConversationId } from "@/lib/types"

export interface ChatEventHandler {
  onMessage?: (conversationId: ConversationId, message: ChatMessage) => void
  onResync?: () => void
}

interface ActiveConversation {
  active: ConversationId | null
  setActive: (id: ConversationId | null) => void
}

interface Connection {
  online: boolean
  reconnect: () => void
}

type Subscribe = (handler: ChatEventHandler) => () => void

const SubscribeContext = createContext<Subscribe | null>(null)
const ActiveConversationContext = createContext<ActiveConversation | null>(null)
const ConnectionContext = createContext<Connection | null>(null)

const OFFLINE_GRACE_MS = 4000

function parseEvent(data: string): ChatEvent | null {
  try {
    const event = JSON.parse(data)
    return event && typeof event.seq === "number" ? event : null
  } catch {
    return null
  }
}

function notify(handlers: Iterable<ChatEventHandler>, event: ChatEvent) {
  if (event.type !== "message") return
  for (const handler of handlers) handler.onMessage?.(event.conversation_id, event.message)
}

export function ChatEventsProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const handlersRef = useRef(new Set<ChatEventHandler>())
  const activeRef = useRef<ConversationId | null>(null)
  const [active, setActiveState] = useState<ConversationId | null>(null)
  const [online, setOnline] = useState(true)
  const reconnectRef = useRef<() => void>(() => {})

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

  const reconnect = useCallback(() => reconnectRef.current(), [])
  const connectionValue = useMemo(() => ({ online, reconnect }), [online, reconnect])

  useEffect(() => {
    let source: EventSource | undefined
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined
    let offlineTimer: ReturnType<typeof setTimeout> | undefined
    let opening = false
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

    const markOnline = () => {
      clearTimeout(offlineTimer)
      offlineTimer = undefined
      setOnline(true)
    }

    const markLost = () => {
      if (offlineTimer !== undefined) return
      offlineTimer = setTimeout(() => setOnline(false), OFFLINE_GRACE_MS)
    }

    const open = () => {
      if (opening) return
      opening = true
      const start = () => {
        opening = false
        connect()
      }
      ensureFreshToken().then(start, start)
    }

    const connect = () => {
      if (stopped) return
      const es = new EventSource(`${config.apiUrl}/local/stream${tokenParam()}`)
      source = es

      es.onopen = () => {
        attempts = 0
        markOnline()
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
        markLost()
        reconnectTimer = setTimeout(open, reconnectDelay(attempts))
      }
    }

    const retryNow = () => {
      if (stopped || opening || source?.readyState !== EventSource.CLOSED) return
      clearTimeout(reconnectTimer)
      open()
    }

    reconnectRef.current = retryNow
    window.addEventListener("online", retryNow)
    window.addEventListener("focus", retryNow)
    open()

    return () => {
      stopped = true
      reconnectRef.current = () => {}
      window.removeEventListener("online", retryNow)
      window.removeEventListener("focus", retryNow)
      clearTimeout(reconnectTimer)
      clearTimeout(offlineTimer)
      source?.close()
    }
  }, [queryClient])

  return (
    <SubscribeContext.Provider value={subscribe}>
      <ActiveConversationContext.Provider value={activeValue}>
        <ConnectionContext.Provider value={connectionValue}>{children}</ConnectionContext.Provider>
      </ActiveConversationContext.Provider>
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

export function useConnection(): Connection {
  const value = useContext(ConnectionContext)
  if (!value) throw new Error("useConnection must be used within ChatEventsProvider")
  return value
}
