import { useCallback, useRef, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { CONVERSATIONS_KEY } from "@/lib/chat-events"
import type { ConversationId, ConversationMessagesResponse } from "@/lib/types"

function useInvalidateConversations() {
  const queryClient = useQueryClient()
  return useCallback(() => {
    queryClient.invalidateQueries({ queryKey: CONVERSATIONS_KEY })
  }, [queryClient])
}

export function useConversations() {
  return useQuery({
    queryKey: CONVERSATIONS_KEY,
    queryFn: () => api.chat.conversations(),
  })
}

export function useConversationMessages(id: ConversationId) {
  return useQuery({
    queryKey: ["conversation-messages", id],
    queryFn: () => api.chat.messages(id),
    staleTime: 0,
  })
}

export function useLoadOlder(id: ConversationId, onLoaded: (page: ConversationMessagesResponse) => void) {
  const isLoadingRef = useRef(false)
  const [isLoadingMore, setIsLoadingMore] = useState(false)
  const [loadError, setLoadError] = useState(false)

  const loadMore = useCallback(async (earliestTs: number) => {
    if (isLoadingRef.current) return
    isLoadingRef.current = true
    setIsLoadingMore(true)
    setLoadError(false)
    try {
      onLoaded(await api.chat.messages(id, earliestTs))
    } catch {
      setLoadError(true)
    } finally {
      isLoadingRef.current = false
      setIsLoadingMore(false)
    }
  }, [id, onLoaded])

  return { loadMore, isLoadingMore, loadError }
}

export function useSendToConversation(conversationId: ConversationId) {
  return useMutation({
    mutationFn: ({ id, content, mediaPaths }: { id: string; content: string; mediaPaths?: string[] }) =>
      api.chat.send(conversationId, id, content, mediaPaths),
  })
}

export function useMarkRead(id: ConversationId) {
  const invalidate = useInvalidateConversations()
  return useMutation({
    mutationFn: (ts?: number) => api.chat.markRead(id, ts),
    onSuccess: invalidate,
  })
}

export function useClearConversation(id: ConversationId) {
  const queryClient = useQueryClient()
  const invalidate = useInvalidateConversations()
  return useMutation({
    mutationFn: (force?: boolean) => api.chat.clear(id, force),
    onSuccess: () => {
      invalidate()
      queryClient.invalidateQueries({ queryKey: ["conversation-messages", id] })
    },
  })
}

export function useStopConversation(id: ConversationId) {
  const invalidate = useInvalidateConversations()
  return useMutation({
    mutationFn: () => api.chat.stop(id),
    onSuccess: invalidate,
  })
}
