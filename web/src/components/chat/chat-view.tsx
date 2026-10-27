import { useCallback, useEffect, useMemo, useReducer, useRef, useState, type ReactNode } from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { InfoIcon } from "lucide-react"
import { Avatar } from "@/components/arc/avatar/avatar"
import { Button } from "@/components/arc/button/button"
import { ActivityIndicator } from "@/components/chat/activity-indicator"
import { BeeAvatar } from "@/components/chat/bee-avatar"
import { ChatHeader } from "@/components/chat/chat-header"
import { ChatSkeleton } from "@/components/chat/chat-skeleton"
import { Composer } from "@/components/chat/composer"
import { HandoffCard } from "@/components/chat/handoff-card"
import { MessageBubble, type MessageSender } from "@/components/chat/message-bubble"
import { useScrollAnchor } from "@/components/chat/use-scroll-anchor"
import { EmptyState } from "@/components/empty-state"
import { WorkerAvatar } from "@/components/worker-avatar"
import { useActiveConversation, useChatEventSubscription } from "@/hooks/use-chat-events"
import {
  useConversationMessages,
  useConversations,
  useLoadOlder,
  useMarkRead,
  useSendToConversation,
} from "@/hooks/use-conversations"
import { useWorkers } from "@/hooks/use-workers"
import { chatReducer, createMessageId, EMPTY_CHAT } from "@/lib/chat-messages"
import { composerDisabled, groupStarts, senderDisplay, statusText, type SenderDisplay } from "@/lib/chat-view"
import { BEE_CONVERSATION, conversationPath, workerIdOf } from "@/lib/conversations"
import { CHAT_COLUMN } from "@/lib/styles"
import type {
  ChatMessage,
  ConversationActivity,
  ConversationId,
  ConversationMessagesResponse,
  Worker,
} from "@/lib/types"
import { cn, getErrorMessage } from "@/lib/utils"

const IDLE: ConversationActivity = { state: "idle" }
const BEE_BUSY: ConversationActivity = { state: "running" }
const EMPTY_WORKERS: Worker[] = []

function isVisible() {
  return document.visibilityState === "visible"
}

function senderAvatar(display: SenderDisplay, workers: Map<string, Worker>): ReactNode {
  if (display.kind === "bee") return <BeeAvatar size="sm" />
  if (display.kind === "worker") {
    const worker = display.workerId ? workers.get(display.workerId) : undefined
    return worker ? (
      <WorkerAvatar workerId={worker.id} name={worker.name} status={worker.status} size="sm" />
    ) : (
      <Avatar name="?" size="sm" aria-hidden="true" />
    )
  }
  return (
    <span aria-hidden="true" className="grid size-7 place-items-center rounded-full bg-surface-muted text-secondary">
      <InfoIcon size={14} strokeWidth={1.75} />
    </span>
  )
}

export function ChatView({ conversation }: { conversation: ConversationId }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isBee = conversation === BEE_CONVERSATION
  const workerId = workerIdOf(conversation)

  const { data: workersData, isPending: workersPending } = useWorkers()
  const workerList = workersData ?? EMPTY_WORKERS
  const workers = useMemo(() => new Map(workerList.map((item) => [item.id, item])), [workerList])
  const worker = workerId ? workers.get(workerId) : undefined
  const name = isBee ? t("localChat.beeLabel") : (worker?.name ?? t("chat.deletedWorker"))

  const { data: conversations } = useConversations()
  const [beeBusy, setBeeBusy] = useState(false)
  const summaryActivity = conversations?.find((item) => item.id === conversation)?.activity ?? IDLE
  const activity = isBee ? (beeBusy ? BEE_BUSY : IDLE) : summaryActivity
  const status = statusText(activity, worker, t)
  const busy = activity.state !== "idle"

  const { data, isPending, isError, refetch } = useConversationMessages(conversation)
  const { mutateAsync: sendMessage } = useSendToConversation(conversation)
  const { mutate: markRead } = useMarkRead(conversation)
  const { setActive } = useActiveConversation()

  const [chat, dispatch] = useReducer(chatReducer, EMPTY_CHAT)
  const messages = chat.messages
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  const scrollContentRef = useRef<HTMLDivElement>(null)
  const logRef = useRef<HTMLDivElement>(null)

  const { handleScroll, captureAnchor, stickToBottom } = useScrollAnchor(
    { containerRef: scrollContainerRef, contentRef: scrollContentRef, logRef },
    messages,
  )

  useEffect(() => {
    setActive(conversation)
    return () => setActive(null)
  }, [conversation, setActive])

  useEffect(() => {
    if (data) dispatch({ type: "latest", messages: data.messages, hasMore: data.has_more })
  }, [data])

  useEffect(() => {
    if (isVisible()) markRead(undefined)
    const handleVisibility = () => {
      if (isVisible()) markRead(undefined)
    }
    document.addEventListener("visibilitychange", handleVisibility)
    return () => document.removeEventListener("visibilitychange", handleVisibility)
  }, [markRead])

  useChatEventSubscription({
    onMessage: (id, message) => {
      if (id !== conversation) return
      dispatch({ type: "add", message: { ...message, pending: true } })
      if (message.sender.type === "user") return
      if (isBee) setBeeBusy(false)
      if (isVisible()) markRead(message.ts)
    },
    onResync: () => {
      void refetch()
    },
  })

  const handleExpandToggle = useCallback(() => {
    stickToBottom.current = false
    captureAnchor()
  }, [captureAnchor, stickToBottom])

  const handleOlderLoaded = useCallback(
    (page: ConversationMessagesResponse) => {
      if (page.messages.length > 0) captureAnchor()
      dispatch({ type: "older", messages: page.messages, hasMore: page.has_more })
    },
    [captureAnchor],
  )

  const { loadMore, isLoadingMore, loadError } = useLoadOlder(conversation, handleOlderLoaded)

  const handleLoadMore = useCallback(() => {
    void loadMore(messages[0]?.ts ?? Date.now())
  }, [loadMore, messages])

  const handleSend = useCallback(
    async (content: string, paths: string[]) => {
      const id = createMessageId()
      const mediaPaths = paths.length > 0 ? paths : undefined
      stickToBottom.current = true
      dispatch({
        type: "add",
        message: { id, sender: { type: "user" }, kind: "message", content, media_paths: mediaPaths, ts: Date.now(), pending: true },
      })
      if (isBee) setBeeBusy(true)
      try {
        const sent = await sendMessage({ id, content: content || " ", mediaPaths })
        dispatch({ type: "sent", id, ts: sent.ts })
      } catch (error) {
        dispatch({ type: "remove", id })
        if (isBee) setBeeBusy(false)
        toast.error(t("chat.sendFailed"), { description: getErrorMessage(error) })
        throw error
      }
    },
    [isBee, sendMessage, stickToBottom, t],
  )

  const openConversation = useCallback((id: ConversationId) => navigate(conversationPath(id)), [navigate])

  const senderCache = useMemo(() => new Map<string, MessageSender | null>(), [conversation, workers, t])
  const senderOf = (message: ChatMessage): MessageSender | null => {
    const key = `${message.sender.type}:${message.sender.id ?? ""}`
    const cached = senderCache.get(key)
    if (cached !== undefined) return cached
    const display = senderDisplay(message, conversation, workers, t)
    const sender =
      display.kind === "user"
        ? null
        : { name: display.name, avatar: senderAvatar(display, workers), via: display.via ? t("chat.fromBee") : undefined }
    senderCache.set(key, sender)
    return sender
  }

  const starts = useMemo(() => groupStarts(messages), [messages])
  const loading = (isPending || workersPending) && messages.length === 0
  const failed = isError && !data && messages.length === 0
  const avatar = isBee ? (
    <BeeAvatar size="sm" />
  ) : (
    worker && <WorkerAvatar workerId={worker.id} name={worker.name} status={worker.status} size="sm" />
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col animate-fade-in">
      <ChatHeader conversation={conversation} worker={worker} name={name} status={status} busy={busy} />

      <div
        ref={scrollContainerRef}
        onScroll={handleScroll}
        className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden overscroll-contain [overflow-anchor:none]"
      >
        <div ref={scrollContentRef} className={cn(CHAT_COLUMN, "flex min-h-full flex-col pt-6 pb-8 md:pt-10")}>
          {loading ? (
            <ChatSkeleton />
          ) : failed ? (
            <div className="flex flex-1 flex-col items-center justify-center gap-3">
              <p className="text-sm text-secondary">{t("common.loadError")}</p>
              <Button variant="secondary" size="sm" onClick={() => void refetch()}>
                {t("common.retry")}
              </Button>
            </div>
          ) : messages.length === 0 ? (
            worker ? (
              <WorkerIntro worker={worker} />
            ) : (
              <div className="flex flex-1 items-center justify-center">
                <EmptyState title={t("localChat.noMessagesTitle")} description={t("localChat.noMessagesDescription")} />
              </div>
            )
          ) : (
            <>
              {chat.hasMore && (
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
                {messages.map((message, index) => (
                  <MessageBubble
                    key={message.id}
                    message={message}
                    sender={senderOf(message)}
                    isGroupStart={starts[index]}
                    onExpandToggle={handleExpandToggle}
                    body={
                      message.kind === "handoff" && message.handoff ? (
                        <HandoffCard
                          info={message.handoff}
                          instruction={message.content}
                          workers={workers}
                          onOpen={openConversation}
                        />
                      ) : undefined
                    }
                  />
                ))}
              </div>
            </>
          )}
        </div>
      </div>

      <ActivityIndicator label={busy ? status : null} avatar={avatar} />

      <Composer
        placeholder={t("chat.messagePlaceholder", { name })}
        disabled={composerDisabled(activity)}
        mentions={isBee ? workerList : undefined}
        onSend={handleSend}
      />
    </div>
  )
}

function WorkerIntro({ worker }: { worker: Worker }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-5 py-10 text-center">
      <WorkerAvatar workerId={worker.id} name={worker.name} status={worker.status} size="xl" />
      <div className="grid max-w-md gap-2">
        <h2 className="text-xl font-medium text-foreground">{worker.name}</h2>
        {worker.description && <p className="line-clamp-4 text-sm text-secondary">{worker.description}</p>}
      </div>
      <p className="max-w-md text-sm text-muted">{t("chat.emptyWorker", { name: worker.name })}</p>
    </div>
  )
}
