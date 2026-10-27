import {
  useCallback,
  useEffect,
  useReducer,
  useRef,
  useState,
} from "react"
import { useTranslation } from "react-i18next"
import { Button } from "@/components/arc/button/button"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { ChatSkeleton } from "@/components/chat/chat-skeleton"
import { Composer } from "@/components/chat/composer"
import { MessageBubble } from "@/components/chat/message-bubble"
import { useScrollAnchor } from "@/components/chat/use-scroll-anchor"
import {
  useLocalMessages,
  useLocalChatStream,
  useLoadMoreMessages,
  useSendMessage,
} from "@/hooks/use-local-chat"
import { EmptyState } from "@/components/empty-state"
import type { ChatMessage, LocalMessagesResponse, Worker } from "@/lib/types"
import { cn } from "@/lib/utils"
import { CHAT_COLUMN } from "@/lib/styles"
import { chatReducer, createMessageId, EMPTY_CHAT } from "@/lib/chat-messages"
import { useWorkers } from "@/hooks/use-workers"

const EMPTY_WORKERS: Worker[] = []

export function LocalChat() {
  const { t } = useTranslation()

  const { data, isLoading } = useLocalMessages()
  const sendMessage = useSendMessage()

  const [chat, dispatch] = useReducer(chatReducer, EMPTY_CHAT)
  const localMessages = chat.messages
  const [isProcessing, setIsProcessing] = useState(false)
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  const scrollContentRef = useRef<HTMLDivElement>(null)
  const logRef = useRef<HTMLDivElement>(null)

  const { handleScroll, captureAnchor, stickToBottom } = useScrollAnchor(
    { containerRef: scrollContainerRef, contentRef: scrollContentRef, logRef },
    localMessages
  )

  const handleExpandToggle = useCallback(() => {
    stickToBottom.current = false
    captureAnchor()
  }, [captureAnchor, stickToBottom])

  const handleOlderLoaded = useCallback((page: LocalMessagesResponse) => {
    if (page.messages.length > 0) captureAnchor()
    dispatch({ type: "older", messages: page.messages, hasMore: page.has_more })
  }, [captureAnchor])

  const { loadMore, isLoadingMore, loadError } = useLoadMoreMessages(handleOlderLoaded)
  const hasMore = chat.hasMore

  useEffect(() => {
    if (!data) return
    dispatch({ type: "latest", messages: data.messages, hasMore: data.has_more })
  }, [data])

  const handleReply = useCallback((message: ChatMessage) => {
    dispatch({ type: "add", message })
    setIsProcessing(false)
  }, [])
  useLocalChatStream(handleReply)

  const { data: workersData } = useWorkers()

  const handleSend = useCallback(async (content: string, paths: string[]) => {
    const id = createMessageId()
    const userMessage: ChatMessage = {
      id,
      role: "user",
      content,
      media_paths: paths.length > 0 ? paths : undefined,
      ts: Date.now(),
      pending: true,
    }

    stickToBottom.current = true
    dispatch({ type: "add", message: userMessage })
    setIsProcessing(true)

    try {
      const sent = await sendMessage.mutateAsync({
        id,
        content: content || " ",
        mediaPaths: paths.length > 0 ? paths : undefined,
      })
      dispatch({ type: "sent", id, ts: sent.ts })
    } catch (error) {
      dispatch({ type: "remove", id })
      setIsProcessing(false)
      throw error
    }
  }, [sendMessage, stickToBottom])

  const handleLoadMore = useCallback(() => {
    void loadMore(localMessages[0]?.ts ?? Date.now())
  }, [loadMore, localMessages])

  const messageCount = localMessages.length
  const isEmpty = !isLoading && messageCount === 0

  return (
    <div className="flex h-full min-h-0 flex-col animate-fade-in">
      <h1 className="sr-only">{t("localChat.title")}</h1>

      <div
        ref={scrollContainerRef}
        onScroll={handleScroll}
        className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden overscroll-contain [overflow-anchor:none]"
      >
        <div ref={scrollContentRef} className={cn(CHAT_COLUMN, "flex min-h-full flex-col pt-6 pb-8 md:pt-10")}>
          {isLoading ? (
            <ChatSkeleton />
          ) : isEmpty ? (
            <div className="flex flex-1 items-center justify-center">
              <EmptyState
                title={t("localChat.noMessagesTitle")}
                description={t("localChat.noMessagesDescription")}
              />
            </div>
          ) : (
            <>
              {hasMore && (
                <div className="mb-8 flex flex-col items-center gap-2">
                  {loadError && !isLoadingMore && (
                    <p role="alert" className="text-sm text-danger">
                      {t("localChat.loadMoreFailed")}
                    </p>
                  )}
                  <Button variant="secondary" size="sm" loading={isLoadingMore} onClick={handleLoadMore}>
                    {loadError ? t("common.retry") : t("localChat.loadMore")}
                  </Button>
                </div>
              )}
              <div ref={logRef} role="log" aria-live="polite" aria-relevant="additions">
                {localMessages.map((message, index) => {
                  const prev = localMessages[index - 1]
                  const isGroupStart = !prev || prev.role !== message.role

                  return (
                    <MessageBubble
                      key={message.id}
                      message={message}
                      isGroupStart={isGroupStart}
                      onExpandToggle={handleExpandToggle}
                    />
                  )
                })}

                {isProcessing && (
                  <div className="mt-6 flex flex-col items-start">
                    <span className="mb-2 text-xs font-medium text-foreground">{t("localChat.beeLabel")}</span>
                    <TextShimmer className="text-sm text-secondary">{t("localChat.processing")}</TextShimmer>
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>

      <Composer
        placeholder={t("localChat.inputPlaceholder")}
        disabled={isProcessing}
        mentions={workersData ?? EMPTY_WORKERS}
        onSend={handleSend}
      />
    </div>
  )
}
