import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef } from "react"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { getAccessToken } from "@/lib/auth"
import type { ChatMessage } from "@/lib/types"

export function useLocalSessions() {
  return useQuery({
    queryKey: ["local-sessions"],
    queryFn: () => api.localChat.listSessions(),
  })
}

export function useLocalMessages(sessionId: string) {
  return useQuery({
    queryKey: ["local-messages", sessionId],
    queryFn: () => api.localChat.getMessages(sessionId),
    enabled: !!sessionId,
  })
}

export function useCreateSession() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => api.localChat.createSession(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["local-sessions"] }),
  })
}

export function useDeleteSession() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.localChat.deleteSession(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["local-sessions"] }),
  })
}

export function useSendMessage(sessionId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ content, mediaPath }: { content: string; mediaPath?: string }) =>
      api.localChat.sendMessage(sessionId, content, mediaPath),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["local-sessions"] })
    },
  })
}

// useLocalChatStream subscribes to SSE for a session.
// Calls onReply on each new reply event and re-fetches history on reconnect.
export function useLocalChatStream(
  sessionId: string,
  onReply: (msg: ChatMessage) => void
) {
  const queryClient = useQueryClient()
  const onReplyRef = useRef(onReply)
  onReplyRef.current = onReply

  useEffect(() => {
    if (!sessionId) return
    let es: EventSource
    let reconnectTimer: ReturnType<typeof setTimeout>

    const connect = () => {
      const token = getAccessToken()
      const tokenParam = token ? `?token=${encodeURIComponent(token)}` : ""
      es = new EventSource(`${config.apiUrl}/local/sessions/${sessionId}/stream${tokenParam}`)

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
        // Re-fetch full history to fill any gap, then reconnect
        queryClient.invalidateQueries({ queryKey: ["local-messages", sessionId] })
        reconnectTimer = setTimeout(connect, 2000)
      }
    }

    connect()

    return () => {
      clearTimeout(reconnectTimer)
      es?.close()
    }
  }, [sessionId, queryClient])
}
