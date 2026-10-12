import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef, useState, useCallback } from "react"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import type { ChatMessage, LocalMessagesResponse } from "@/lib/types"

export function useLocalMessages() {
  return useQuery({
    queryKey: ["local-messages"],
    queryFn: () => api.localChat.getMessages(),
  })
}

/**
 * useLoadMoreMessages manages incremental loading of older messages.
 * Call `loadMore(earliestTs)` with the timestamp of the oldest message currently shown.
 */
export function useLoadMoreMessages(onLoaded: (page: LocalMessagesResponse) => void) {
  const isLoadingRef = useRef(false)
  const [isLoadingMore, setIsLoadingMore] = useState(false)
  const [loadError, setLoadError] = useState(false)

  const loadMore = useCallback(async (earliestTs: number) => {
    if (isLoadingRef.current) return
    isLoadingRef.current = true
    setIsLoadingMore(true)
    setLoadError(false)
    try {
      onLoaded(await api.localChat.getMessages(earliestTs))
    } catch {
      setLoadError(true)
    } finally {
      isLoadingRef.current = false
      setIsLoadingMore(false)
    }
  }, [onLoaded])

  return { loadMore, isLoadingMore, loadError }
}

export function useSendMessage() {
  return useMutation({
    mutationFn: ({ id, content, mediaPaths }: { id: string; content: string; mediaPaths?: string[] }) =>
      api.localChat.sendMessage(id, content, mediaPaths),
  })
}

// Stop reconnecting after this many consecutive failures so an unrecoverable
// error (revoked permission, server down) can't spin a reconnect loop forever.
const MAX_RECONNECT_ATTEMPTS = 5

// Calls onReply on each new reply event and re-fetches history on reconnect.
export function useLocalChatStream(onReply: (msg: ChatMessage) => void) {
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
      es = new EventSource(`${config.apiUrl}/local/stream${tokenParam()}`)

      es.onopen = () => {
        attempts = 0
        everConnected = true
      }

      es.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data)
          onReplyRef.current({
            id: data.id,
            role: "bee",
            content: data.content,
            media_paths: data.media_paths,
            ts: data.created_at,
            pending: true,
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
          queryClient.invalidateQueries({ queryKey: ["local-messages"] })
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
  }, [queryClient])
}
