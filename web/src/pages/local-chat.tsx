import {
  memo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
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
  SendHorizontal,
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
import { ImageLightbox } from "@/components/image-lightbox"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import type { ChatMessage, Worker } from "@/lib/types"
import { basename, cn, getFileCategory, isImage } from "@/lib/utils"
import { ALERT_DESTRUCTIVE, STREAMDOWN_BLOCKS } from "@/lib/styles"
import { isSameDay } from "@/lib/format"
import { useWorkers } from "@/hooks/use-workers"
import { useMe } from "@/hooks/use-me"
import { hasPermission, Perm } from "@/lib/permissions"
import { MentionTextarea } from "@/components/mention-textarea"
import { LogoMark } from "@/components/brand/logo"

const EMPTY_WORKERS: Worker[] = []

// Enable Shiki syntax highlighting for fenced code blocks. Kept as a stable
// module-level reference so Streamdown's memoization isn't defeated by a new
// object on every render.
const STREAMDOWN_PLUGINS = { code }

// The Bee's 24px identity chip: the OpenBee mark on a white disc.
function BeeAvatar() {
  return (
    <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-background ring-1 ring-border" aria-hidden="true">
      <LogoMark className="size-4.5" />
    </span>
  )
}

// Convert isolated single newlines to double newlines so Markdown renders them
// as paragraph breaks. Fenced code blocks are left untouched.
function normalizeBeeContent(content: string): string {
  const parts = content.split(/(```[\s\S]*?```)/g)
  return parts
    .map((part, index) => {
      if (index % 2 === 1) return part
      return part.replace(/(?<!\n)\n(?!\n)/g, "\n\n")
    })
    .join("")
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

  if (isImage(mediaPath)) {
    return <ImageLightbox src={url} alt={filename} />
  }

  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className="inline-flex items-center gap-2 rounded-sm bg-background px-3 py-2 text-sm text-foreground ring-1 ring-border transition-colors outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring"
    >
      <FileCategoryIcon filePath={mediaPath} className="size-4 shrink-0 text-muted-foreground" />
      <span className="break-all">{filename}</span>
      <ArrowUpRight className="size-4 shrink-0 text-muted-foreground" />
    </a>
  )
})

const COLLAPSE_HEIGHT = 320

const CollapsibleContent = memo(function CollapsibleContent({
  children,
}: {
  children: React.ReactNode
}) {
  const { t } = useTranslation()
  const innerRef = useRef<HTMLDivElement>(null)
  const [collapsed, setCollapsed] = useState(true)
  const [overflows, setOverflows] = useState(false)

  useLayoutEffect(() => {
    const el = innerRef.current
    if (!el) return
    setOverflows(el.scrollHeight > COLLAPSE_HEIGHT)
  }, [children])

  return (
    <div>
      <div
        className={cn(
          "overflow-hidden transition-[max-height] duration-300",
          collapsed && overflows && "border-b border-dashed border-border"
        )}
        style={{ maxHeight: collapsed && overflows ? COLLAPSE_HEIGHT : undefined }}
      >
        <div ref={innerRef}>{children}</div>
      </div>
      {overflows && (
        <button
          type="button"
          className="mt-2 rounded-sm text-[13px] font-medium text-link underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
          onClick={() => setCollapsed((prev) => !prev)}
        >
          {collapsed ? t("localChat.showMore") : t("localChat.showLess")}
        </button>
      )}
    </div>
  )
})

// A single chat message row (sent or received). Extracted as a memo'd component
// so a long transcript doesn't re-render every bubble on each state change, and
// so each row can own its hover state for the inline copy affordance.
const MessageBubble = memo(function MessageBubble({
  message,
  isGroupStart,
}: {
  message: ChatMessage
  isGroupStart: boolean
}) {
  const { t, i18n } = useTranslation()
  const isUser = message.role === "user"
  const hasContent = message.content.trim().length > 0
  const hasMedia = Boolean(message.media_paths && message.media_paths.length > 0)
  const normalizedContent = useMemo(
    () => normalizeBeeContent(message.content),
    [message.content]
  )

  const body = (
    <>
      {hasMedia && (
        <div className="space-y-2">
          {message.media_paths!.map((path) => (
            <AttachmentPreview key={path} mediaPath={path} />
          ))}
        </div>
      )}

      {hasContent && (
        <CollapsibleContent>
          <div className={cn("min-w-0 break-words", STREAMDOWN_BLOCKS, hasMedia && "mt-2")}>
            <Streamdown mode="static" plugins={STREAMDOWN_PLUGINS}>{normalizedContent}</Streamdown>
          </div>
        </CollapsibleContent>
      )}
    </>
  )

  const timestamp = (
    <time className="text-xs text-muted-foreground tabular-nums">
      {formatMessageTimestamp(message.ts, i18n.language)}
    </time>
  )

  if (isUser) {
    return (
      <div className={cn("group flex flex-col items-end first:mt-0", isGroupStart ? "mt-5" : "mt-1.5")}>
        {isGroupStart && (
          <div className="mb-1.5 flex items-center gap-2">
            {timestamp}
            <span className="text-[13px] font-medium text-muted-foreground">{t("localChat.operatorLabel")}</span>
          </div>
        )}

        <div className="relative max-w-[min(100%,40rem)] min-w-0">
          <div className="overflow-hidden rounded-sm bg-background px-3.5 py-2 ring-1 ring-border">
            {body}
          </div>

          {hasContent && (
            <CopyButton
              value={message.content}
              className="absolute top-1.5 -left-7 p-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
            />
          )}
        </div>
      </div>
    )
  }

  return (
    <div className={cn("group flex flex-col items-start first:mt-0", isGroupStart ? "mt-5" : "mt-1.5")}>
      {isGroupStart && (
        <div className="mb-1 flex items-center gap-2">
          <BeeAvatar />
          <span className="text-[13px] font-semibold text-strong">{t("localChat.beeLabel")}</span>
          {timestamp}
        </div>
      )}

      <div className="relative w-full max-w-[52rem] min-w-0 pl-8">
        {body}

        {hasContent && (
          <CopyButton
            value={message.content}
            className="absolute top-0.5 -right-7 p-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
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

  const [localMessages, setLocalMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState("")
  const [isProcessing, setIsProcessing] = useState(false)
  const [pendingMediaPaths, setPendingMediaPaths] = useState<string[]>([])
  const [uploadError, setUploadError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  const suppressScrollRef = useRef(false)

  const handleOlderLoaded = useCallback((older: ChatMessage[]) => {
    suppressScrollRef.current = true
    setLocalMessages((prev) => [...older, ...prev])
  }, [])

  const { loadMore, hasMore, isLoadingMore } = useLoadMoreMessages(handleOlderLoaded, data?.has_more ?? false)

  useEffect(() => {
    if (!data) return
    setLocalMessages(data.messages)
  }, [data])

  useEffect(() => {
    const textarea = textareaRef.current
    if (!textarea) return
    textarea.style.height = "0px"
    textarea.style.height = `${Math.min(textarea.scrollHeight, 160)}px`
  }, [input])

  const handleReply = useCallback((message: ChatMessage) => {
    setLocalMessages((prev) => [...prev, message])
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
    if (suppressScrollRef.current) {
      suppressScrollRef.current = false
      return
    }
    bottomRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [localMessages, isProcessing])

  const handleSend = useCallback(async () => {
    const content = input.trim()
    if (!content && pendingMediaPaths.length === 0) return

    const paths = [...pendingMediaPaths]
    const userMessage: ChatMessage = {
      role: "user",
      content,
      media_paths: paths.length > 0 ? paths : undefined,
      ts: Date.now(),
    }

    setLocalMessages((prev) => [...prev, userMessage])
    setInput("")
    setPendingMediaPaths([])
    setUploadError(null)
    setIsProcessing(true)

    try {
      await sendMessage.mutateAsync({
        content: content || " ",
        mediaPaths: paths.length > 0 ? paths : undefined,
      })
    } catch {
      setLocalMessages((prev) => prev.filter((message) => message !== userMessage))
      setPendingMediaPaths((prev) => [...paths, ...prev])
      setIsProcessing(false)
    }
  }, [input, pendingMediaPaths, sendMessage])

  const handleLoadMore = useCallback(() => {
    const container = scrollContainerRef.current
    const prevScrollHeight = container?.scrollHeight ?? 0
    const earliestTs = localMessages[0]?.ts ?? Date.now()
    loadMore(earliestTs).then(() => {
      if (container) {
        container.scrollTop += container.scrollHeight - prevScrollHeight
      }
    }).catch(() => {
      // scroll restoration skipped on error; hasMore remains true so user can retry
    })
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

  const handleComposerKeyDown = useCallback((event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault()
      void handleSend()
    }
  }, [handleSend])

  const messageCount = localMessages.length
  const canSend = input.trim().length > 0 || pendingMediaPaths.length > 0
  const isEmpty = !isLoading && messageCount === 0

  return (
    <div className="flex h-full min-h-0 animate-fade-in flex-col bg-canvas">
      <div ref={scrollContainerRef} className="flex-1 overflow-x-hidden overflow-y-auto">
        <div className="mx-auto w-full max-w-4xl px-4 py-6 sm:px-6">
            {isLoading ? (
              <div className="space-y-6">
                {Array.from({ length: 3 }).map((_, index) => (
                  <div key={index} className="flex gap-2">
                    <div className="skeleton size-6 shrink-0 rounded-full" />
                    <div className="flex-1 space-y-2 pt-1">
                      <div className="skeleton h-3.5 w-24" />
                      <div className="skeleton h-3.5 w-full" />
                      <div className="skeleton h-3.5 w-4/5" />
                    </div>
                  </div>
                ))}
              </div>
            ) : isEmpty ? (
              <EmptyState
                title={t("localChat.noMessagesTitle")}
                description={t("localChat.noMessagesDescription")}
              />
            ) : (
              <div role="log" aria-live="polite" aria-relevant="additions">
                {hasMore && (
                  <div className="flex justify-center pb-4">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={isLoadingMore}
                      onClick={handleLoadMore}
                    >
                      {isLoadingMore ? t("localChat.loadingMore") : t("localChat.loadMore")}
                    </Button>
                  </div>
                )}
                {localMessages.map((message, index) => {
                  const prev = localMessages[index - 1]
                  const isGroupStart = !prev || prev.role !== message.role

                  return (
                    <MessageBubble
                      key={`${message.role}-${message.ts}-${index}`}
                      message={message}
                      isGroupStart={isGroupStart}
                    />
                  )
                })}

                {isProcessing && (
                  <div className="mt-5 flex flex-col items-start">
                    <div className="mb-1 flex items-center gap-2">
                      <BeeAvatar />
                      <span className="text-[13px] font-semibold text-strong">{t("localChat.beeLabel")}</span>
                      <span className="text-xs text-muted-foreground">{t("localChat.processing")}</span>
                    </div>
                    <div className="flex gap-1.5 py-1.5 pl-8" aria-hidden="true">
                      <span className="size-1.5 animate-pulse-amber rounded-full bg-status-working" style={{ animationDelay: "0ms" }} />
                      <span className="size-1.5 animate-pulse-amber rounded-full bg-status-working" style={{ animationDelay: "300ms" }} />
                      <span className="size-1.5 animate-pulse-amber rounded-full bg-status-working" style={{ animationDelay: "600ms" }} />
                    </div>
                  </div>
                )}

                <div ref={bottomRef} />
              </div>
            )}
          </div>
      </div>

      <div className="shrink-0">
        <div className="mx-auto w-full max-w-4xl px-4 pt-2 pb-4 sm:px-6">
            {uploadError && (
              <div role="alert" className={cn(ALERT_DESTRUCTIVE, "mb-2 px-3 py-2")}>
                {uploadError}
              </div>
            )}

            <div className="rounded-sm bg-background shadow-xs ring-1 ring-border transition-shadow focus-within:ring-[1.5px] focus-within:ring-focus/50">
              {pendingMediaPaths.length > 0 && (
                <div className="flex flex-wrap gap-1.5 border-b border-hairline px-2.5 py-2">
                  {pendingMediaPaths.map((path) => (
                    <span
                      key={path}
                      className="inline-flex h-7 items-center gap-1.5 rounded-sm bg-recessed pr-1 pl-2 text-xs text-foreground"
                    >
                      <FileCategoryIcon filePath={path} className="size-3.5 shrink-0 text-muted-foreground" />
                      <span className="max-w-52 truncate">{basename(path)}</span>
                      <button
                        type="button"
                        className="inline-flex size-5 items-center justify-center rounded-sm text-muted-foreground transition-colors outline-none hover:bg-background hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
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

              <MentionTextarea
                textareaRef={textareaRef}
                className="block max-h-[160px] min-h-[2.75rem] w-full resize-none bg-transparent px-3 pt-2.5 pb-1 text-sm leading-6 text-foreground placeholder:text-muted-foreground focus:outline-none disabled:opacity-60"
                placeholder={t("localChat.inputPlaceholder")}
                value={input}
                onChange={setInput}
                onKeyDown={handleComposerKeyDown}
                onPaste={handlePaste}
                workers={workersData ?? EMPTY_WORKERS}
                disabled={isProcessing}
              />

              <div className="flex items-center justify-between gap-2 px-2 pb-2">
                <div className="flex min-w-0 items-center gap-2">
                  <input
                    ref={fileInputRef}
                    type="file"
                    multiple
                    className="hidden"
                    onChange={handleFileChange}
                  />
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="text-muted-foreground"
                    onClick={() => fileInputRef.current?.click()}
                    aria-label={t("localChat.uploadFile")}
                    title={t("localChat.uploadFile")}
                  >
                    <Paperclip />
                  </Button>
                  <span className="hidden truncate text-xs text-muted-foreground sm:inline">
                    {t("localChat.composerHint")}
                  </span>
                </div>

                <Button
                  size="icon-sm"
                  onClick={() => void handleSend()}
                  disabled={!canSend || sendMessage.isPending}
                  aria-label={t("localChat.send")}
                  title={t("localChat.send")}
                >
                  <SendHorizontal />
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
  )
}
