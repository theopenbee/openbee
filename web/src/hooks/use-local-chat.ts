import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef } from "react"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import type { ChatMessage } from "@/lib/types"

export function useLocalMessages() {
  return useQuery({
    queryKey: ["local-messages"],
    queryFn: () => api.localChat.getMessages(),
  })
}

export function useSendMessage() {
  return useMutation({
    mutationFn: ({ content, mediaPaths }: { content: string; mediaPaths?: string[] }) =>
      api.localChat.sendMessage(content, mediaPaths),
  })
}

// useLocalChatStream subscribes to SSE for the default local session.
// Calls onReply on each new reply event and re-fetches history on reconnect.
export function useLocalChatStream(onReply: (msg: ChatMessage) => void) {
  const queryClient = useQueryClient()
  const onReplyRef = useRef(onReply)
  onReplyRef.current = onReply

  useEffect(() => {
    let es: EventSource
    let reconnectTimer: ReturnType<typeof setTimeout>
    let mounted = true

    const connect = () => {
      if (!mounted) return
      es = new EventSource(`${config.apiUrl}/local/stream${tokenParam()}`)

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
        // Re-fetch full history to fill any gap created by the disconnect.
        queryClient.invalidateQueries({ queryKey: ["local-messages"] })
        reconnectTimer = setTimeout(connect, 2000)
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
