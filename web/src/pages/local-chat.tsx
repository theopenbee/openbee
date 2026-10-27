import {
  memo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useReducer,
  useRef,
  useState,
} from "react"
import { useTranslation } from "react-i18next"
import {
  ArrowUp,
  ArrowUpRight,
  File,
  FileArchive,
  FileAudio,
  FileCode,
  FileImage,
  FileText,
  FileVideo,
  Paperclip,
  X,
} from "lucide-react"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import {
  useLocalMessages,
  useLocalChatStream,
  useLoadMoreMessages,
  useSendMessage,
} from "@/hooks/use-local-chat"
import { EmptyState } from "@/components/empty-state"
import { CopyButton } from "@/components/copy-button"
import { ExpandableContent } from "@/components/expandable-content"
import { ImageLightbox } from "@/components/image-lightbox"
import { ChatMarkdown } from "@/components/chat-markdown"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import type { ChatMessage, LocalMessagesResponse, Worker } from "@/lib/types"
import { basename, cn, getFileCategory, isImage, isImeComposing } from "@/lib/utils"
import { isSameDay } from "@/lib/format"
import { observeResize } from "@/lib/resize-observer"
import { chatReducer, createMessageId, EMPTY_CHAT } from "@/lib/chat-messages"
import { useWorkers } from "@/hooks/use-workers"
import { useMediaQuery } from "@/hooks/use-media-query"
import { MentionTextarea } from "@/components/mention-textarea"
import composerStyles from "@/components/chat-composer.module.css"

const EMPTY_WORKERS: Worker[] = []

const TOUCH_ONLY_QUERY = "(hover: none) and (pointer: coarse)"

const NEAR_BOTTOM_PX = 48

const COLLAPSE_HEIGHT = 320

const COMPOSER_MAX_HEIGHT = 160

const COLUMN = "mx-auto w-full max-w-3xl px-4 sm:px-6"

type ScrollAnchor = { el: Element; top: number }

function isNearBottom(container: HTMLElement) {
  return container.scrollHeight - container.scrollTop - container.clientHeight < NEAR_BOTTOM_PX
}

function findScrollAnchor(container: HTMLElement, log: HTMLElement | null): ScrollAnchor | null {
  if (!log) return null
  const containerTop = container.getBoundingClientRect().top
  const items = log.children
  let lo = 0
  let hi = items.length - 1
  let anchor: Element | null = null
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (items[mid].getBoundingClientRect().bottom > containerTop) {
      anchor = items[mid]
      hi = mid - 1
    } else {
      lo = mid + 1
    }
  }
  return anchor ? { el: anchor, top: anchor.getBoundingClientRect().top - containerTop } : null
}

function formatMessageTimestamp(timestamp: number | null | undefined, language: string) {
  if (!timestamp) return ""
  return new Intl.DateTimeFormat(language, isSameDay(timestamp, Date.now())
    ? {
        hour: "numeric",
        minute: "2-digit",
      }
    : {
        month: "short",
        day: "numeric",
        hour: "numeric",
        minute: "2-digit",
      }).format(new Date(timestamp))
}

function FileCategoryIcon({ filePath, className }: { filePath: string; className?: string }) {
  const category = getFileCategory(filePath)
  const props = { size: 16, strokeWidth: 1.75, "aria-hidden": true, className: cn("shrink-0", className) }
  switch (category) {
    case "image": return <FileImage {...props} />
    case "audio": return <FileAudio {...props} />
    case "video": return <FileVideo {...props} />
    case "document": return <FileText {...props} />
    case "code": return <FileCode {...props} />
    case "archive": return <FileArchive {...props} />
    default: return <File {...props} />
  }
}

function mediaUrl(mediaPath: string) {
  const filename = basename(mediaPath)
  return `${config.apiUrl}/local/media/${encodeURIComponent(filename)}${tokenParam()}`
}

const AttachmentPreview = memo(function AttachmentPreview({
  mediaPath,
}: {
  mediaPath: string
}) {
  const filename = basename(mediaPath)
  const url = mediaUrl(mediaPath)

  if (isImage(mediaPath)) {
    return <ImageLightbox src={url} alt={filename} />
  }

  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className="inline-flex min-h-11 max-w-full items-center gap-2 rounded-control border border-border bg-surface px-3 text-sm text-foreground transition-colors hover:bg-surface-muted"
    >
      <FileCategoryIcon filePath={mediaPath} className="text-secondary" />
      <span className="min-w-0 truncate">{filename}</span>
      <ArrowUpRight size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-muted" />
    </a>
  )
})

const MessageBubble = memo(function MessageBubble({
  message,
  isGroupStart,
  onExpandToggle,
}: {
  message: ChatMessage
  isGroupStart: boolean
  onExpandToggle: () => void
}) {
  const { t, i18n } = useTranslation()
  const isUser = message.role === "user"
  const hasContent = message.content.trim().length > 0
  const mediaPaths = message.media_paths ?? []
  const timestamp = formatMessageTimestamp(message.ts, i18n.language)

  const copyReveal =
    "transition-opacity pointer-fine:opacity-0 pointer-fine:group-hover:opacity-100 pointer-fine:focus-within:opacity-100"

  return (
    <div
      className={cn(
        "group flex min-w-0 flex-col first:mt-0",
        isUser ? "items-end" : "items-start",
        isGroupStart ? "mt-6" : "mt-2"
      )}
    >
      {isGroupStart && (
        <div className={cn("mb-2 flex items-baseline gap-2 text-xs", isUser && "flex-row-reverse")}>
          <span className="font-medium text-foreground">
            {isUser ? t("localChat.operatorLabel") : t("localChat.beeLabel")}
          </span>
          {timestamp && <time className="text-muted tabular">{timestamp}</time>}
        </div>
      )}

      {mediaPaths.length > 0 && (
        <div
          className={cn(
            "flex max-w-full flex-col gap-2",
            hasContent && "mb-2",
            isUser ? "items-end sm:max-w-[min(85%,32rem)]" : "items-start sm:max-w-md"
          )}
        >
          {mediaPaths.map((path) => (
            <AttachmentPreview key={path} mediaPath={path} />
          ))}
        </div>
      )}

      {hasContent && isUser && (
        <div className="relative min-w-0 max-w-[min(85%,36rem)]">
          <div className="rounded-panel bg-surface-muted px-4 py-2.5">
            <ExpandableContent
              maxHeight={COLLAPSE_HEIGHT}
              onToggle={onExpandToggle}
              fadeClassName="h-16 from-surface-muted"
            >
              <ChatMarkdown breaks>{message.content}</ChatMarkdown>
            </ExpandableContent>
          </div>
          <span className={cn("absolute top-1 right-full mr-1 inline-flex", copyReveal)}>
            <CopyButton value={message.content} label={t("localChat.copyMessage")} />
          </span>
        </div>
      )}

      {hasContent && !isUser && (
        <>
          <div className="w-full min-w-0">
            <ExpandableContent maxHeight={COLLAPSE_HEIGHT} onToggle={onExpandToggle}>
              <ChatMarkdown breaks>{message.content}</ChatMarkdown>
            </ExpandableContent>
          </div>
          <div className={cn("-mb-1 -ml-2.5 flex", copyReveal)}>
            <CopyButton value={message.content} label={t("localChat.copyMessage")} />
          </div>
        </>
      )}
    </div>
  )
})

function ChatSkeleton() {
  return (
    <div className="flex flex-col gap-8" aria-busy="true">
      <div className="flex justify-end">
        <div className="skeleton h-11 w-2/5" />
      </div>
      <div className="flex flex-col gap-2">
        <div className="skeleton h-3 w-12" />
        <div className="skeleton h-4 w-full" />
        <div className="skeleton h-4 w-4/5" />
        <div className="skeleton h-4 w-3/5" />
      </div>
      <div className="flex justify-end">
        <div className="skeleton h-11 w-1/3" />
      </div>
      <div className="flex flex-col gap-2">
        <div className="skeleton h-3 w-12" />
        <div className="skeleton h-4 w-11/12" />
        <div className="skeleton h-4 w-2/3" />
      </div>
    </div>
  )
}

export function LocalChat() {
  const { t } = useTranslation()

  const { data, isLoading } = useLocalMessages()
  const sendMessage = useSendMessage()

  const [chat, dispatch] = useReducer(chatReducer, EMPTY_CHAT)
  const localMessages = chat.messages
  const [input, setInput] = useState("")
  const [isProcessing, setIsProcessing] = useState(false)
  const [pendingMediaPaths, setPendingMediaPaths] = useState<string[]>([])
  const [uploadError, setUploadError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  const scrollContentRef = useRef<HTMLDivElement>(null)
  const logRef = useRef<HTMLDivElement>(null)
  const stickToBottomRef = useRef(true)
  const scrollAnchorRef = useRef<ScrollAnchor | null>(null)
  const lastScrollTopRef = useRef(0)

  const captureScrollAnchor = useCallback(() => {
    const container = scrollContainerRef.current
    scrollAnchorRef.current =
      container && !stickToBottomRef.current ? findScrollAnchor(container, logRef.current) : null
  }, [])

  const syncScroll = useCallback(() => {
    const container = scrollContainerRef.current
    if (!container) return
    if (stickToBottomRef.current) {
      container.scrollTop = container.scrollHeight
      lastScrollTopRef.current = container.scrollTop
      return
    }
    const anchor = scrollAnchorRef.current
    if (anchor?.el.isConnected) {
      const delta = anchor.el.getBoundingClientRect().top - container.getBoundingClientRect().top - anchor.top
      if (Math.abs(delta) >= 1) container.scrollTop += delta
    }
    lastScrollTopRef.current = container.scrollTop
    captureScrollAnchor()
  }, [captureScrollAnchor])

  const handleExpandToggle = useCallback(() => {
    stickToBottomRef.current = false
    captureScrollAnchor()
  }, [captureScrollAnchor])

  const handleOlderLoaded = useCallback((page: LocalMessagesResponse) => {
    if (page.messages.length > 0) captureScrollAnchor()
    dispatch({ type: "older", messages: page.messages, hasMore: page.has_more })
  }, [captureScrollAnchor])

  const { loadMore, isLoadingMore, loadError } = useLoadMoreMessages(handleOlderLoaded)
  const hasMore = chat.hasMore

  useEffect(() => {
    if (!data) return
    dispatch({ type: "latest", messages: data.messages, hasMore: data.has_more })
  }, [data])

  useEffect(() => {
    const textarea = textareaRef.current
    if (!textarea) return
    textarea.style.height = "0px"
    textarea.style.height = `${Math.min(textarea.scrollHeight, COMPOSER_MAX_HEIGHT)}px`
  }, [input])

  const handleReply = useCallback((message: ChatMessage) => {
    dispatch({ type: "add", message })
    setIsProcessing(false)
  }, [])
  useLocalChatStream(handleReply)

  const { data: workersData } = useWorkers()

  useEffect(() => {
    const container = scrollContainerRef.current
    const content = scrollContentRef.current
    if (!container || !content) return
    const stopContainer = observeResize(container, syncScroll)
    const stopContent = observeResize(content, syncScroll)
    return () => {
      stopContainer()
      stopContent()
    }
  }, [syncScroll])

  useLayoutEffect(() => {
    syncScroll()
  }, [localMessages, syncScroll])

  const handleScroll = useCallback(() => {
    const container = scrollContainerRef.current
    if (!container) return
    const top = container.scrollTop
    if (Math.abs(top - lastScrollTopRef.current) < 1) {
      syncScroll()
      return
    }
    const movedUp = top < lastScrollTopRef.current
    lastScrollTopRef.current = top
    if (isNearBottom(container)) stickToBottomRef.current = true
    else if (movedUp) stickToBottomRef.current = false
    captureScrollAnchor()
  }, [captureScrollAnchor, syncScroll])

  const handleSend = useCallback(async () => {
    const content = input.trim()
    if (!content && pendingMediaPaths.length === 0) return

    const paths = [...pendingMediaPaths]
    const id = createMessageId()
    const userMessage: ChatMessage = {
      id,
      role: "user",
      content,
      media_paths: paths.length > 0 ? paths : undefined,
      ts: Date.now(),
      pending: true,
    }

    stickToBottomRef.current = true
    dispatch({ type: "add", message: userMessage })
    setInput("")
    setPendingMediaPaths([])
    setUploadError(null)
    setIsProcessing(true)

    try {
      const sent = await sendMessage.mutateAsync({
        id,
        content: content || " ",
        mediaPaths: paths.length > 0 ? paths : undefined,
      })
      dispatch({ type: "sent", id, ts: sent.ts })
    } catch {
      dispatch({ type: "remove", id })
      setPendingMediaPaths((prev) => [...paths, ...prev])
      setIsProcessing(false)
    }
  }, [input, pendingMediaPaths, sendMessage])

  const handleLoadMore = useCallback(() => {
    void loadMore(localMessages[0]?.ts ?? Date.now())
  }, [loadMore, localMessages])

  const uploadFiles = useCallback(async (files: File[]) => {
    if (files.length === 0) return

    setUploadError(null)

    const results = await Promise.allSettled(
      files.map((file) => api.localChat.uploadMedia(file))
    )
    const succeeded = results.filter(
      (result): result is PromiseFulfilledResult<{ path: string }> => result.status === "fulfilled"
    )
    const failedCount = results.length - succeeded.length

    if (failedCount > 0) {
      setUploadError(t("localChat.uploadError", { count: failedCount }))
    }

    setPendingMediaPaths((prev) => [...prev, ...succeeded.map((result) => result.value.path)])
  }, [t])

  const handleFileChange = useCallback(async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? [])
    event.target.value = ""
    await uploadFiles(files)
  }, [uploadFiles])

  const handlePaste = useCallback(async (event: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const files = Array.from(event.clipboardData.items)
      .map((item) => (item.type.startsWith("image/") ? item.getAsFile() : null))
      .filter((file): file is File => file !== null)
    if (files.length === 0) return

    event.preventDefault()
    await uploadFiles(files)
  }, [uploadFiles])

  const isTouchOnly = useMediaQuery(TOUCH_ONLY_QUERY)

  const handleComposerKeyDown = useCallback((event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== "Enter" || event.shiftKey || isImeComposing(event)) return
    if (isTouchOnly && !event.metaKey && !event.ctrlKey) return
    event.preventDefault()
    void handleSend()
  }, [handleSend, isTouchOnly])

  const messageCount = localMessages.length
  const canSend = input.trim().length > 0 || pendingMediaPaths.length > 0
  const isEmpty = !isLoading && messageCount === 0
  const controlSize = isTouchOnly ? "md" : "sm"

  return (
    <div className="flex h-full min-h-0 flex-col animate-fade-in">
      <h1 className="sr-only">{t("localChat.title")}</h1>

      <div
        ref={scrollContainerRef}
        onScroll={handleScroll}
        className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden overscroll-contain [overflow-anchor:none]"
      >
        <div ref={scrollContentRef} className={cn(COLUMN, "flex min-h-full flex-col pt-6 pb-8 md:pt-10")}>
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

      <div className="shrink-0 bg-background">
        <div className={cn(COLUMN, "pb-3 sm:pb-6")}>
          {uploadError && (
            <div className="mb-3">
              <Alert tone="danger" title={uploadError} open onDismiss={() => setUploadError(null)} />
            </div>
          )}

          <div
            className={cn(
              composerStyles.composer,
              "rounded-surface border border-border bg-surface p-2 transition-colors focus-within:border-border-strong"
            )}
          >
            {pendingMediaPaths.length > 0 && (
              <ul className={cn(composerStyles.files, "flex flex-wrap gap-2 px-2 pt-1 pb-2")} aria-label={t("localChat.attachments")}>
                {pendingMediaPaths.map((path) => (
                  <li
                    key={path}
                    className="inline-flex min-h-8 max-w-full items-center gap-2 rounded-full border border-border bg-surface-muted py-1 pr-1 pl-3 text-xs text-foreground"
                  >
                    <FileCategoryIcon filePath={path} className="text-secondary" />
                    <span className="min-w-0 max-w-40 truncate sm:max-w-56">{basename(path)}</span>
                    <button
                      type="button"
                      className="inline-grid size-6 shrink-0 place-items-center rounded-full text-secondary transition-colors hover:bg-surface hover:text-foreground"
                      aria-label={t("localChat.removeNamedAttachment", { name: basename(path) })}
                      onClick={() =>
                        setPendingMediaPaths((prev) => prev.filter((entry) => entry !== path))
                      }
                    >
                      <X size={16} strokeWidth={1.75} aria-hidden="true" />
                    </button>
                  </li>
                ))}
              </ul>
            )}

            <div className={composerStyles.input}>
              <MentionTextarea
                textareaRef={textareaRef}
                ariaLabel={t("localChat.messageLabel")}
                className="block max-h-40 min-h-11 w-full resize-none bg-transparent px-2 py-2.5 text-base leading-6 text-foreground placeholder:text-muted disabled:cursor-not-allowed disabled:opacity-60 sm:min-h-12 sm:px-3 sm:py-3 sm:text-sm"
                placeholder={t("localChat.inputPlaceholder")}
                value={input}
                onChange={setInput}
                onKeyDown={handleComposerKeyDown}
                onPaste={handlePaste}
                workers={workersData ?? EMPTY_WORKERS}
                disabled={isProcessing}
              />
            </div>

            <input
              ref={fileInputRef}
              type="file"
              multiple
              className="hidden"
              onChange={handleFileChange}
            />
            <span className={composerStyles.attach}>
              <Button
                variant="ghost"
                size={controlSize}
                onClick={() => fileInputRef.current?.click()}
                aria-label={t("localChat.uploadFile")}
              >
                <Paperclip size={16} strokeWidth={1.75} aria-hidden="true" />
              </Button>
            </span>
            <span className={cn(composerStyles.hint, "truncate text-xs text-muted")}>
              {isTouchOnly ? t("localChat.composerHintTouch") : t("localChat.composerHint")}
            </span>
            <span className={composerStyles.send}>
              <Button
                size={controlSize}
                onClick={() => void handleSend()}
                disabled={!canSend || sendMessage.isPending}
                aria-label={t("localChat.send")}
              >
                <ArrowUp size={16} strokeWidth={1.75} aria-hidden="true" />
              </Button>
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
