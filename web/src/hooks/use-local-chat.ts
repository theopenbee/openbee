import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef, useState, useCallback } from "react"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import type { ChatMessage } from "@/lib/types"

// useLocalWorkers lists the digital employees a user can chat with 1:1.
// Scoped by chat:write (not contacts:read), so any chat user gets the list.
export function useLocalWorkers() {
  return useQuery({
    queryKey: ["local-chat-workers"],
    queryFn: () => api.localChat.listWorkers(),
  })
}

// useLocalMessages fetches history for the active conversation. Pass a workerId
// to scope to a 1:1 digital-employee conversation; omit it for the bee conversation.
export function useLocalMessages(workerId = "") {
  return useQuery({
    queryKey: ["local-messages", workerId],
    queryFn: () => api.localChat.getMessages(undefined, 50, workerId),
  })
}

/**
 * useLoadMoreMessages manages incremental loading of older messages.
 * Pass `initialHasMore` from the first query response to initialize the button state.
 * Call `loadMore(earliestTs)` with the timestamp of the oldest message currently shown.
 */
export function useLoadMoreMessages(
  onLoaded: (older: ChatMessage[]) => void,
  initialHasMore = false,
  workerId = "",
) {
  const isLoadingRef = useRef(false)
  const [isLoadingMore, setIsLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(initialHasMore)

  useEffect(() => {
    setHasMore(initialHasMore)
  }, [initialHasMore])

  const loadMore = useCallback(async (earliestTs: number) => {
    if (isLoadingRef.current) return
    isLoadingRef.current = true
    setIsLoadingMore(true)
    try {
      const res = await api.localChat.getMessages(earliestTs, 50, workerId)
      setHasMore(res.has_more)
      onLoaded(res.messages)
    } finally {
      isLoadingRef.current = false
      setIsLoadingMore(false)
    }
  }, [onLoaded, workerId])

  return { loadMore, hasMore, isLoadingMore }
}

export function useSendMessage() {
  return useMutation({
    mutationFn: ({ content, mediaPaths, workerId }: { content: string; mediaPaths?: string[]; workerId?: string }) =>
      api.localChat.sendMessage(content, mediaPaths, workerId),
  })
}

// Stop reconnecting after this many consecutive failures so an unrecoverable
// error (revoked permission, server down) can't spin a reconnect loop forever.
const MAX_RECONNECT_ATTEMPTS = 5

// useLocalChatStream subscribes to SSE for the active conversation (bee when
// workerId is empty, otherwise the 1:1 digital-employee session). Calls onReply
// on each new reply event and re-fetches that conversation's history on reconnect.
export function useLocalChatStream(onReply: (msg: ChatMessage) => void, workerId = "") {
  const queryClient = useQueryClient()
  const onReplyRef = useRef(onReply)
  onReplyRef.current = onReply

  useEffect(() => {
    let es: EventSource
    let reconnectTimer: ReturnType<typeof setTimeout>
    let mounted = true
    let attempts = 0
    let everConnected = false

    const connect = () => {
      if (!mounted) return
      const workerParam = workerId ? `&worker_id=${encodeURIComponent(workerId)}` : ""
      es = new EventSource(`${config.apiUrl}/local/stream${tokenParam()}${workerParam}`)

      es.onopen = () => {
        attempts = 0
        everConnected = true
      }

      es.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data)
          onReplyRef.current({
            role: "bee",
            content: data.content,
            ts: data.created_at,
          })
        } catch {
          // ignore malformed events
        }
      }

      es.onerror = () => {
        es.close()
        clearTimeout(reconnectTimer)
        // A connection that never opened (e.g. a 403) has no gap to backfill and
        // would only spawn another failing request — only re-fetch history after
        // a real disconnect from a previously-open stream.
        if (everConnected) {
          queryClient.invalidateQueries({ queryKey: ["local-messages", workerId] })
        }
        if (attempts >= MAX_RECONNECT_ATTEMPTS) return
        attempts += 1
        // Exponential backoff (2s, 4s, 8s … capped at 30s) instead of a fixed
        // 2s loop, so repeated failures don't hammer the server.
        const delay = Math.min(2000 * 2 ** (attempts - 1), 30_000)
        reconnectTimer = setTimeout(connect, delay)
      }
    }

    connect()

    return () => {
      mounted = false
      clearTimeout(reconnectTimer)
      es?.close()
    }
  }, [queryClient, workerId])
}
