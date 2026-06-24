import {
  memo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react"
import { Streamdown } from "streamdown"
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
import { DetailSection } from "@/components/detail-primitives"
import { EmptyState } from "@/components/empty-state"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import type { ChatMessage, Worker } from "@/lib/types"
import { basename, cn, getFileCategory, isImage } from "@/lib/utils"
import { isSameDay } from "@/lib/format"
import { useWorkers } from "@/hooks/use-workers"
import { MentionTextarea } from "@/components/mention-textarea"

const EMPTY_WORKERS: Worker[] = []

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
  tone,
}: {
  mediaPath: string
  tone: "user" | "bee"
}) {
  const filename = basename(mediaPath)
  const url = mediaUrl(mediaPath)
  const frameClass = "border-border/70 bg-background/70"

  if (isImage(mediaPath)) {
    return (
      <a
        href={url}
        target="_blank"
        rel="noreferrer"
        className={cn("block overflow-hidden rounded-2xl border", frameClass)}
      >
        <img
          src={url}
          alt={filename}
          className="max-h-80 w-full object-contain"
        />
      </a>
    )
  }

  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className={cn(
        "inline-flex items-center gap-2 rounded-2xl border px-3 py-2 text-sm transition-colors",
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
        className={cn("overflow-hidden transition-[max-height] duration-300", collapsed && overflows ? "relative" : "")}
        style={{ maxHeight: collapsed && overflows ? COLLAPSE_HEIGHT : undefined }}
      >
        <div ref={innerRef}>{children}</div>
        {collapsed && overflows && (
          <div className="absolute inset-x-0 bottom-0 h-16 bg-gradient-to-t from-background/95 to-transparent" />
        )}
      </div>
      {overflows && (
        <button
          type="button"
          className="mt-2 text-xs font-medium text-primary/80 hover:text-primary transition-colors"
          onClick={() => setCollapsed((prev) => !prev)}
        >
          {collapsed ? t("localChat.showMore") : t("localChat.showLess")}
        </button>
      )}
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
    textarea.style.height = `${Math.min(textarea.scrollHeight, 220)}px`
  }, [input])

  const handleReply = useCallback((message: ChatMessage) => {
    setLocalMessages((prev) => [...prev, message])
    setIsProcessing(false)
  }, [])
  useLocalChatStream(handleReply)

  const { data: workersData } = useWorkers()

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

  const handleQuickCommand = useCallback(async (text: string) => {
    const userMessage: ChatMessage = {
      role: "user",
      content: text,
      ts: Date.now(),
    }
    setLocalMessages((prev) => [...prev, userMessage])
    setIsProcessing(true)
    try {
      await sendMessage.mutateAsync({ content: text })
    } catch {
      setLocalMessages((prev) => prev.filter((m) => m !== userMessage))
      setIsProcessing(false)
    }
  }, [sendMessage])

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
    <FadeIn>
      <PageHeader title={t("localChat.title")} />
      <DetailSection className="flex min-h-[34rem] flex-col xl:h-[calc(100vh-12rem)]">
        <div ref={scrollContainerRef} className="flex-1 overflow-y-auto overflow-x-hidden px-4 py-5 sm:px-6 sm:py-6">
            {isLoading ? (
              <div className="space-y-4">
                {Array.from({ length: 3 }).map((_, index) => (
                  <div
                    key={index}
                    className="rounded-3xl border border-border/70 bg-background/80 px-4 py-4"
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
              <div className="space-y-4" role="log" aria-live="polite" aria-relevant="additions">
                {hasMore && (
                  <div className="flex justify-center pb-4">
                    <button
                      type="button"
                      disabled={isLoadingMore}
                      onClick={handleLoadMore}
                      className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/80 px-4 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
                    >
                      {isLoadingMore ? t("localChat.loadingMore") : t("localChat.loadMore")}
                    </button>
                  </div>
                )}
                {localMessages.map((message, index) => {
                  const isUser = message.role === "user"
                  const hasContent = message.content.trim().length > 0

                  return (
                    <div
                      key={`${message.role}-${message.ts}-${index}`}
                      className={cn("flex", isUser ? "justify-end" : "justify-start")}
                    >
                      <article
                        className={cn(
                          "w-full overflow-hidden rounded-3xl border border-border/70 bg-background/80 px-4 py-4 sm:px-5",
                          isUser
                            ? "max-w-[min(100%,44rem)]"
                            : "max-w-[min(100%,52rem)]"
                        )}
                      >
                        <div className="flex flex-wrap items-center justify-between gap-3">
                          <div className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.18em]">
                            <span
                              className={cn(
                                "size-2 rounded-full",
                                isUser ? "bg-muted-foreground/40" : "bg-primary"
                              )}
                            />
                            <span className="text-muted-foreground">
                              {isUser ? t("localChat.operatorLabel") : t("localChat.beeLabel")}
                            </span>
                          </div>

                          <time className="text-xs text-muted-foreground">
                            {formatMessageTimestamp(message.ts, i18n.language)}
                          </time>
                        </div>

                        {message.media_paths && message.media_paths.length > 0 && (
                          <div className="mt-4 space-y-3">
                            {message.media_paths.map((path) => (
                              <AttachmentPreview
                                key={path}
                                mediaPath={path}
                                tone={message.role}
                              />
                            ))}
                          </div>
                        )}

                        {hasContent && (
                          <CollapsibleContent>
                            <div className="prose prose-sm mt-4 max-w-none dark:prose-invert prose-p:my-3 prose-pre:overflow-x-auto prose-pre:rounded-2xl prose-pre:border prose-pre:border-border/70 prose-pre:bg-muted/35 prose-pre:px-4 prose-pre:py-3 prose-code:break-words">
                              <Streamdown mode="static">{normalizeBeeContent(message.content)}</Streamdown>
                            </div>
                          </CollapsibleContent>
                        )}
                      </article>
                    </div>
                  )
                })}

                {isProcessing && (
                  <div className="flex justify-start">
                    <div className="w-full max-w-[min(100%,52rem)] rounded-3xl border border-border/70 bg-background/80 px-4 py-4 sm:px-5">
                      <div className="flex items-center justify-between gap-3">
                        <div className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                          <span className="size-2 rounded-full bg-primary" />
                          <span>{t("localChat.beeLabel")}</span>
                        </div>
                        <span className="text-xs text-muted-foreground">
                          {t("localChat.processing")}
                        </span>
                      </div>
                      <div className="mt-4 flex gap-1.5">
                        <span className="h-2 w-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "0ms" }} />
                        <span className="h-2 w-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "300ms" }} />
                        <span className="h-2 w-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "600ms" }} />
                      </div>
                    </div>
                  </div>
                )}

                <div ref={bottomRef} />
              </div>
            )}
          </div>

          <div className="border-t border-border/70 bg-card p-4 sm:p-5">
            {uploadError && (
              <div role="alert" className="mb-4 rounded-2xl border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                {uploadError}
              </div>
            )}

            {pendingMediaPaths.length > 0 && (
              <div className="mb-4 flex flex-wrap gap-2">
                {pendingMediaPaths.map((path) => (
                  <span
                    key={path}
                    className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/80 px-3 py-1.5 text-xs text-foreground"
                  >
                    <FileCategoryIcon filePath={path} className="size-3.5 shrink-0 text-muted-foreground" />
                    <span className="max-w-52 truncate">{basename(path)}</span>
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

            <div className="rounded-3xl border border-border/70 bg-background/80 p-3">
              <MentionTextarea
                textareaRef={textareaRef}
                className="max-h-[220px] min-h-[120px] w-full resize-none bg-transparent px-3 py-2 text-sm leading-7 placeholder:text-muted-foreground focus:outline-none"
                placeholder={t("localChat.inputPlaceholder")}
                value={input}
                onChange={setInput}
                onKeyDown={handleComposerKeyDown}
                onPaste={handlePaste}
                workers={workersData ?? EMPTY_WORKERS}
                disabled={isProcessing}
              />

              <div className="mt-3 flex flex-wrap gap-1.5 px-3">
                {[
                  t("localChat.quickCommandClear"),
                  t("localChat.quickCommandConfirm"),
                ].map((cmd) => (
                  <button
                    key={cmd}
                    type="button"
                    className="inline-flex items-center rounded-full border border-border/70 bg-muted/40 px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:border-border hover:bg-muted hover:text-foreground"
                    onClick={() => void handleQuickCommand(cmd)}
                  >
                    {cmd}
                  </button>
                ))}
              </div>

              <div className="mt-3 flex flex-col gap-3 border-t border-border/70 pt-3 sm:flex-row sm:items-center sm:justify-between">
                <p className="text-xs leading-5 text-muted-foreground">
                  {t("localChat.composerHint")}
                </p>

                <div className="flex flex-wrap items-center gap-2">
                  <input
                    ref={fileInputRef}
                    type="file"
                    multiple
                    className="hidden"
                    onChange={handleFileChange}
                  />
                  <Button
                    variant="outline"
                    className="h-10 rounded-xl"
                    onClick={() => fileInputRef.current?.click()}
                    aria-label={t("localChat.uploadFile")}
                  >
                    <Paperclip className="size-4" />
                    <span className="hidden sm:inline">{t("localChat.uploadFile")}</span>
                  </Button>
                  <Button
                    className="h-10 rounded-xl"
                    onClick={() => void handleSend()}
                    disabled={!canSend || sendMessage.isPending}
                    aria-label={t("localChat.send")}
                  >
                    <Send className="size-4" />
                    <span>{t("localChat.send")}</span>
                  </Button>
                </div>
              </div>
            </div>
          </div>
      </DetailSection>
    </FadeIn>
  )
}
