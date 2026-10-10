import {
  memo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useReducer,
  useRef,
  useState,
} from "react"
import { Streamdown } from "streamdown"
import { code } from "@streamdown/code"
import { useTranslation } from "react-i18next"
import {
  ArrowUpRight,
  File,
  FileArchive,
  FileAudio,
  FileCode,
  FileImage,
  FileText,
  FileVideo,
  Paperclip,
  Send,
  X,
} from "lucide-react"
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
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import type { ChatMessage, LocalMessagesResponse, Worker } from "@/lib/types"
import { basename, cn, getFileCategory, isImage, isImeComposing } from "@/lib/utils"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
import { isSameDay } from "@/lib/format"
import { observeResize } from "@/lib/resize-observer"
import { chatReducer, createMessageId, EMPTY_CHAT } from "@/lib/chat-messages"
import { CHAT_REMARK_PLUGINS } from "@/lib/markdown"
import { useWorkers } from "@/hooks/use-workers"
import { useMe } from "@/hooks/use-me"
import { useMediaQuery } from "@/hooks/use-media-query"
import { hasPermission, Perm } from "@/lib/permissions"
import { MentionTextarea } from "@/components/mention-textarea"

const EMPTY_WORKERS: Worker[] = []

// Enable Shiki syntax highlighting for fenced code blocks. Kept as a stable
// module-level reference so Streamdown's memoization isn't defeated by a new
// object on every render.
const STREAMDOWN_PLUGINS = { code }

const TOUCH_ONLY_QUERY = "(hover: none) and (pointer: coarse)"

const NEAR_BOTTOM_PX = 48

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
  if (!timestamp) return "—"
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
  const props = { className: className ?? "size-4 shrink-0" }
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
  const frameClass = "border-border/70 bg-background/70"

  if (isImage(mediaPath)) {
    return <ImageLightbox src={url} alt={filename} className={frameClass} />
  }

  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className={cn(
        "inline-flex items-center gap-2 rounded-sm border px-3 py-2 text-sm transition-colors",
        frameClass,
        "text-foreground hover:bg-muted/40"
      )}
    >
      <FileCategoryIcon filePath={mediaPath} />
      <span className="break-all">{filename}</span>
      <ArrowUpRight className="size-4 shrink-0 opacity-70" />
    </a>
  )
})

const COLLAPSE_HEIGHT = 320

// A single chat message row (sent or received). Extracted as a memo'd component
// so a long transcript doesn't re-render every bubble on each state change, and
// so each row can own its hover state for the inline copy affordance.
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
  const hasMedia = Boolean(message.media_paths && message.media_paths.length > 0)

  return (
    <div
      className={cn(
        "group flex flex-col first:mt-0",
        isUser ? "items-end" : "items-start",
        isGroupStart ? "mt-4" : "mt-1"
      )}
    >
      {isGroupStart && (
        <div
          className={cn(
            "mb-1 flex items-center gap-2 px-0.5 text-xs",
            isUser && "flex-row-reverse"
          )}
        >
          <span
            className={cn(
              "size-1.5 rounded-full",
              isUser ? "bg-muted-foreground/40" : "bg-primary"
            )}
          />
          <span className="font-medium text-muted-foreground">
            {isUser ? t("localChat.operatorLabel") : t("localChat.beeLabel")}
          </span>
          <time className="text-muted-foreground/60">
            {formatMessageTimestamp(message.ts, i18n.language)}
          </time>
        </div>
      )}

      <div
        className={cn(
          "relative flex min-w-0 flex-col",
          isUser ? "max-w-[min(85%,42rem)] items-end" : "max-w-[min(100%,52rem)] items-start"
        )}
      >
        <div
          className={cn(
            "max-w-full overflow-hidden",
            isUser
              ? "rounded-sm bg-muted/50 px-3.5 py-2"
              : "rounded-sm border border-border/60 bg-card px-3.5 py-2"
          )}
        >
          {hasMedia && (
            <div className="space-y-2">
              {message.media_paths!.map((path) => (
                <AttachmentPreview key={path} mediaPath={path} />
              ))}
            </div>
          )}

          {hasContent && (
            <ExpandableContent maxHeight={COLLAPSE_HEIGHT} onToggle={onExpandToggle}>
              <div
                className={cn(
                  "prose prose-sm max-w-none dark:prose-invert prose-p:my-2 prose-pre:rounded-sm prose-pre:border prose-pre:border-border/70 prose-pre:bg-muted/35 prose-pre:px-3 prose-pre:py-2 prose-code:break-words",
                  hasMedia && "mt-2"
                )}
              >
                <Streamdown mode="static" plugins={STREAMDOWN_PLUGINS} remarkPlugins={CHAT_REMARK_PLUGINS}>
                  {message.content}
                </Streamdown>
              </div>
            </ExpandableContent>
          )}
        </div>

        {hasContent && (
          <CopyButton
            value={message.content}
            className={cn(
              "-mx-2 p-2 pointer-fine:absolute pointer-fine:top-1 pointer-fine:m-0 pointer-fine:p-0 pointer-fine:opacity-0 pointer-fine:transition-opacity pointer-fine:group-hover:opacity-100 pointer-fine:focus-visible:opacity-100",
              isUser ? "pointer-fine:-left-7" : "pointer-fine:-right-7"
            )}
          />
        )}
      </div>
    </div>
  )
})

export function LocalChat() {
  const { t, i18n } = useTranslation()

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
    textarea.style.height = `${Math.min(textarea.scrollHeight, 160)}px`
  }, [input])

  const handleReply = useCallback((message: ChatMessage) => {
    dispatch({ type: "add", message })
    setIsProcessing(false)
  }, [])
  useLocalChatStream(handleReply)

  // @mention autocomplete is an enhancement gated on workers:read. Users who can
  // chat but cannot browse the worker directory simply get no mention list — we
  // skip the request entirely so it never 403s and tears down the whole page.
  const { data: me } = useMe()
  const canMention = hasPermission(me?.permissions, Perm.ContactsRead)
  const { data: workersData } = useWorkers(undefined, { enabled: canMention })

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

  return (
    <div className="flex h-full min-h-0 flex-col animate-fade-in">
      <div
        ref={scrollContainerRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto overflow-x-hidden overscroll-contain [overflow-anchor:none]"
      >
        <div ref={scrollContentRef} className="mx-auto w-full max-w-4xl px-3 py-4 sm:px-6 sm:py-5">
            {isLoading ? (
              <div className="space-y-4">
                {Array.from({ length: 3 }).map((_, index) => (
                  <div
                    key={index}
                    className="rounded-sm border border-border/70 bg-background/80 px-4 py-4"
                  >
                    <div className="skeleton h-4 w-28" />
                    <div className="skeleton mt-4 h-4 w-full" />
                    <div className="skeleton mt-2 h-4 w-4/5" />
                  </div>
                ))}
              </div>
            ) : isEmpty ? (
              <EmptyState
                title={t("localChat.noMessagesTitle")}
                description={t("localChat.noMessagesDescription")}
              />
            ) : (
              <>
                {hasMore && (
                  <div className="flex justify-center pb-8">
                    <button
                      type="button"
                      disabled={isLoadingMore}
                      onClick={handleLoadMore}
                      className={cn(
                        "inline-flex items-center gap-2 rounded-full border bg-background/80 px-4 py-1.5 text-xs transition-colors disabled:opacity-50",
                        loadError
                          ? "border-destructive/40 text-destructive hover:bg-destructive/10"
                          : "border-border/70 text-muted-foreground hover:bg-muted hover:text-foreground"
                      )}
                    >
                      {isLoadingMore
                        ? t("localChat.loadingMore")
                        : loadError
                          ? t("localChat.loadMoreError")
                          : t("localChat.loadMore")}
                    </button>
                    {loadError && !isLoadingMore && (
                      <span role="alert" className="sr-only">
                        {t("localChat.loadMoreError")}
                      </span>
                    )}
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
                    <div className="mt-4 flex flex-col items-start">
                      <div className="mb-1 flex items-center gap-2 px-0.5 text-xs text-muted-foreground">
                        <span className="size-1.5 rounded-full bg-primary" />
                        <span className="font-medium">{t("localChat.beeLabel")}</span>
                        <span className="text-muted-foreground/60">{t("localChat.processing")}</span>
                      </div>
                      <div className="flex gap-1.5 px-0.5 py-1">
                        <span className="h-2 w-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "0ms" }} />
                        <span className="h-2 w-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "300ms" }} />
                        <span className="h-2 w-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "600ms" }} />
                      </div>
                    </div>
                  )}
                </div>
              </>
            )}
          </div>
      </div>

      <div className="border-t border-border/70 bg-card">
        <div className="mx-auto w-full max-w-4xl px-3 py-2 sm:px-6 sm:py-3">
            {uploadError && (
              <div role="alert" className={cn(ALERT_DESTRUCTIVE, "mb-2 px-3 py-2")}>
                {uploadError}
              </div>
            )}

            {pendingMediaPaths.length > 0 && (
              <div className="mb-2 flex flex-wrap gap-2">
                {pendingMediaPaths.map((path) => (
                  <span
                    key={path}
                    className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/80 px-3 py-1.5 text-xs text-foreground"
                  >
                    <FileCategoryIcon filePath={path} className="size-3.5 shrink-0 text-muted-foreground" />
                    <span className="max-w-40 truncate sm:max-w-52">{basename(path)}</span>
                    <button
                      type="button"
                      className="inline-flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                      aria-label={`${t("localChat.removeAttachment")}: ${basename(path)}`}
                      onClick={() =>
                        setPendingMediaPaths((prev) => prev.filter((entry) => entry !== path))
                      }
                    >
                      <X className="size-3.5" />
                    </button>
                  </span>
                ))}
              </div>
            )}

            <div className="flex items-end gap-1 rounded-sm border border-border/70 bg-background/80 p-1 sm:block sm:p-2">
              <div className="min-w-0 flex-1">
                <MentionTextarea
                  textareaRef={textareaRef}
                  className="block max-h-[160px] min-h-10 w-full resize-none bg-transparent px-2 py-2 text-base leading-6 placeholder:text-muted-foreground focus:outline-none sm:min-h-[2.75rem] sm:py-1.5 sm:text-sm"
                  placeholder={t("localChat.inputPlaceholder")}
                  value={input}
                  onChange={setInput}
                  onKeyDown={handleComposerKeyDown}
                  onPaste={handlePaste}
                  workers={workersData ?? EMPTY_WORKERS}
                  disabled={isProcessing}
                />
              </div>

              <div className="contents sm:mt-2 sm:flex sm:flex-wrap sm:items-center sm:justify-between sm:gap-2 sm:px-1">
                <span className="hidden text-xs text-muted-foreground sm:inline">
                  {isTouchOnly ? t("localChat.composerHintTouch") : t("localChat.composerHint")}
                </span>

                <div className="contents sm:ml-auto sm:flex sm:items-center sm:gap-2">
                  <input
                    ref={fileInputRef}
                    type="file"
                    multiple
                    className="hidden"
                    onChange={handleFileChange}
                  />
                  <Button
                    variant="outline"
                    className="order-first size-10 rounded-sm border-transparent bg-transparent text-muted-foreground dark:border-transparent dark:bg-transparent sm:order-none sm:h-9 sm:w-auto sm:border-border sm:bg-background sm:text-foreground dark:sm:border-input dark:sm:bg-input/30"
                    onClick={() => fileInputRef.current?.click()}
                    aria-label={t("localChat.uploadFile")}
                  >
                    <Paperclip className="size-5 sm:size-4" />
                    <span className="hidden sm:inline">{t("localChat.uploadFile")}</span>
                  </Button>
                  <Button
                    className="size-10 rounded-sm sm:h-9 sm:w-auto"
                    onClick={() => void handleSend()}
                    disabled={!canSend || sendMessage.isPending}
                    aria-label={t("localChat.send")}
                  >
                    <Send className="size-4" />
                    <span className="hidden sm:inline">{t("localChat.send")}</span>
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
  )
}
