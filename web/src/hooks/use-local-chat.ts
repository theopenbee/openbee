import { useQuery, useMutation } from "@tanstack/react-query"
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
// Calls onReply on each new reply event.
export function useLocalChatStream(onReply: (msg: ChatMessage) => void) {
  const onReplyRef = useRef(onReply)
  onReplyRef.current = onReply

  useEffect(() => {
    let es: EventSource
    let reconnectTimer: ReturnType<typeof setTimeout>

    const connect = () => {
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
        reconnectTimer = setTimeout(connect, 2000)
      }
    }

    connect()

    return () => {
      clearTimeout(reconnectTimer)
      es?.close()
    }
  }, [])
}
