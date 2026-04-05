import {
  memo,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react"
import { Streamdown } from "streamdown"
import { useParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  ArrowUpRight,
  Clock3,
  MessageSquareText,
  Paperclip,
  Send,
  X,
} from "lucide-react"
import {
  useLocalMessages,
  useLocalSessions,
  useLocalChatStream,
  useSendMessage,
} from "@/hooks/use-local-chat"
import { DetailField, DetailHero, DetailOverviewStat, DetailSection } from "@/components/detail-primitives"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import { tokenParam } from "@/lib/auth"
import { config } from "@/lib/config"
import type { ChatMessage } from "@/lib/types"
import { basename, cn, isImage } from "@/lib/utils"

function isSameDay(left: number, right: number) {
  const leftDate = new Date(left)
  const rightDate = new Date(right)
  return (
    leftDate.getFullYear() === rightDate.getFullYear()
    && leftDate.getMonth() === rightDate.getMonth()
    && leftDate.getDate() === rightDate.getDate()
  )
}

function formatSessionTimestamp(timestamp: number | null | undefined, language: string) {
  if (!timestamp) return "—"
  return new Intl.DateTimeFormat(language, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(timestamp))
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

function getLastMessageByRole(messages: ChatMessage[], role: ChatMessage["role"]) {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index].role === role) {
      return messages[index]
    }
  }
  return undefined
}

function mediaUrl(sessionId: string, mediaPath: string) {
  const filename = basename(mediaPath)
  return `${config.apiUrl}/local/sessions/${sessionId}/media/${encodeURIComponent(filename)}${tokenParam()}`
}

const AttachmentPreview = memo(function AttachmentPreview({
  sessionId,
  mediaPath,
  tone,
}: {
  sessionId: string
  mediaPath: string
  tone: "user" | "bee"
}) {
  const filename = basename(mediaPath)
  const url = mediaUrl(sessionId, mediaPath)
  const frameClass = tone === "user"
    ? "border-primary-foreground/15 bg-primary-foreground/10"
    : "border-border/70 bg-background/70"

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
        tone === "user"
          ? "text-primary-foreground/90 hover:bg-primary-foreground/10"
          : "text-foreground hover:bg-muted/40"
      )}
    >
      <Paperclip className="size-4 shrink-0" />
      <span className="break-all">{filename}</span>
      <ArrowUpRight className="size-4 shrink-0 opacity-70" />
    </a>
  )
})

const QueuedMediaItem = memo(function QueuedMediaItem({
  sessionId,
  mediaPath,
  onRemove,
  removeLabel,
  openLabel,
}: {
  sessionId: string
  mediaPath: string
  onRemove: () => void
  removeLabel: string
  openLabel: string
}) {
  const filename = basename(mediaPath)
  const url = mediaUrl(sessionId, mediaPath)

  return (
    <div className="flex items-center gap-3 rounded-2xl border border-border/70 bg-background/80 p-3">
      <a
        href={url}
        target="_blank"
        rel="noreferrer"
        className="flex min-w-0 flex-1 items-center gap-3"
      >
        {isImage(mediaPath) ? (
          <img
            src={url}
            alt={filename}
            className="size-12 rounded-xl border border-border/70 bg-muted/40 object-cover"
          />
        ) : (
          <div className="flex size-12 items-center justify-center rounded-xl border border-border/70 bg-muted/35 text-muted-foreground">
            <Paperclip className="size-4" />
          </div>
        )}

        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">{filename}</p>
          <p className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
            <span>{openLabel}</span>
            <ArrowUpRight className="size-3.5" />
          </p>
        </div>
      </a>

      <button
        type="button"
        className="inline-flex size-8 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        aria-label={`${removeLabel}: ${filename}`}
        onClick={onRemove}
      >
        <X className="size-4" />
      </button>
    </div>
  )
})

export function LocalChatDetail() {
  const { t, i18n } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const sessionId = id!

  const { data: history = [], isLoading } = useLocalMessages(sessionId)
  const { data: sessions = [] } = useLocalSessions()
  const sendMessage = useSendMessage(sessionId)

  const [localMessages, setLocalMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState("")
  const [isProcessing, setIsProcessing] = useState(false)
  const [pendingMediaPaths, setPendingMediaPaths] = useState<string[]>([])
  const [uploadError, setUploadError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)

  const currentSession = sessions.find((session) => session.id === sessionId)

  useEffect(() => {
    setLocalMessages(history)
  }, [history])

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
  useLocalChatStream(sessionId, handleReply)

  useEffect(() => {
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

  const handleFileChange = useCallback(async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? [])
    if (files.length === 0) return

    event.target.value = ""
    setUploadError(null)

    const results = await Promise.allSettled(
      files.map((file) => api.localChat.uploadMedia(sessionId, file))
    )
    const succeeded = results.filter(
      (result): result is PromiseFulfilledResult<{ path: string }> => result.status === "fulfilled"
    )
    const failedCount = results.length - succeeded.length

    if (failedCount > 0) {
      setUploadError(t("localChat.uploadError", { count: failedCount }))
    }

    setPendingMediaPaths((prev) => [...prev, ...succeeded.map((result) => result.value.path)])
  }, [sessionId, t])

  const messageCount = localMessages.length
  const beeReplyCount = localMessages.filter((message) => message.role === "bee").length
  const latestBeeMessage = getLastMessageByRole(localMessages, "bee")
  const latestTimestamp = currentSession?.updated_at ?? localMessages[localMessages.length - 1]?.ts
  const canSend = input.trim().length > 0 || pendingMediaPaths.length > 0
  const isEmpty = !isLoading && messageCount === 0
  const openAttachmentLabel = t("localChat.openAttachment")

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader
          title={currentSession?.name ?? sessionId}
          subtitle={t("localChat.detailDescription")}
          actions={
            <div className="flex flex-wrap items-center justify-end gap-2">
              <span className="inline-flex items-center rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                {isProcessing ? t("localChat.processing") : t("localChat.idleStatus")}
              </span>
              <span className="inline-flex items-center rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                {t("localChat.queuedUploadsLabel")}: {pendingMediaPaths.length}
              </span>
            </div>
          }
        />

        <DetailHero>
          <div className="grid gap-6 p-5 sm:p-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(19rem,0.95fr)]">
            <div className="space-y-5">
              <div className="space-y-3">
                <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">
                  {t("localChat.detailLabel")}
                </p>

                <div className="space-y-2">
                  <h2 className="max-w-4xl break-all font-mono text-sm leading-7 text-foreground sm:text-base">
                    {sessionId}
                  </h2>
                  <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
                    {t("localChat.timelineHint")}
                  </p>
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-2">
                <span className="inline-flex max-w-full items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                  <span>{t("localChat.sessionIdLabel")}</span>
                  <span className="truncate font-mono text-foreground">{sessionId}</span>
                </span>
                <span className="inline-flex items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                  <Clock3 className="size-3.5" />
                  {t("localChat.updatedAt", {
                    time: formatSessionTimestamp(latestTimestamp, i18n.language),
                  })}
                </span>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-2">
              <DetailOverviewStat
                label={t("localChat.messagesLabel")}
                value={<span className="font-mono text-sm sm:text-base">{messageCount}</span>}
                hint={t("localChat.createdAt", {
                  time: formatSessionTimestamp(currentSession?.created_at, i18n.language),
                })}
              />
              <DetailOverviewStat
                label={t("localChat.repliesLabel")}
                value={<span className="font-mono text-sm sm:text-base">{beeReplyCount}</span>}
                hint={isProcessing ? t("localChat.processing") : t("localChat.idleStatus")}
              />
              <DetailOverviewStat
                label={t("localChat.queuedUploadsLabel")}
                value={<span className="font-mono text-sm sm:text-base">{pendingMediaPaths.length}</span>}
                hint={uploadError || t("localChat.queuePanelHint")}
              />
              <DetailOverviewStat
                label={t("localChat.latestResponseLabel")}
                valueClassName="font-mono text-sm leading-6"
                value={
                  latestBeeMessage
                    ? formatSessionTimestamp(latestBeeMessage.ts, i18n.language)
                    : t("localChat.latestResponseEmpty")
                }
                hint={t("localChat.timelineHint")}
              />
            </div>
          </div>
        </DetailHero>

        <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_20rem]">
          <DetailSection className="flex min-h-[34rem] flex-col xl:h-[calc(100vh-16rem)]">
            <div className="border-b border-border/70 px-5 py-4 sm:px-6">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
                <div>
                  <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                    {t("localChat.timelineLabel")}
                  </p>
                  <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                    {t("localChat.timelineHint")}
                  </p>
                </div>

                <div className="flex flex-wrap items-center gap-2 text-xs">
                  <span className="inline-flex items-center rounded-full border border-border/70 bg-background/80 px-3 py-1 text-muted-foreground">
                    {isProcessing ? t("localChat.processing") : t("localChat.idleStatus")}
                  </span>
                  <span className="inline-flex items-center rounded-full border border-border/70 bg-background/80 px-3 py-1 text-muted-foreground">
                    {t("localChat.queuedUploadsLabel")}: {pendingMediaPaths.length}
                  </span>
                </div>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto px-4 py-5 sm:px-6 sm:py-6">
              {isLoading ? (
                <div className="space-y-4">
                  {Array.from({ length: 3 }).map((_, index) => (
                    <div
                      key={index}
                      className="rounded-[1.6rem] border border-border/70 bg-background/80 px-4 py-4"
                    >
                      <div className="h-4 w-28 rounded bg-muted" />
                      <div className="mt-4 h-4 w-full rounded bg-muted" />
                      <div className="mt-2 h-4 w-4/5 rounded bg-muted" />
                    </div>
                  ))}
                </div>
              ) : isEmpty ? (
                <div className="flex h-full min-h-[18rem] items-center justify-center">
                  <div className="max-w-md rounded-[1.75rem] border border-dashed border-border/80 bg-background/78 px-6 py-8 text-left">
                    <div className="inline-flex size-12 items-center justify-center rounded-2xl border border-border/70 bg-muted/35">
                      <MessageSquareText className="size-5 text-muted-foreground" />
                    </div>
                    <h2 className="mt-5 text-lg font-semibold tracking-tight text-foreground">
                      {t("localChat.noMessagesTitle")}
                    </h2>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                      {t("localChat.noMessagesDescription")}
                    </p>
                  </div>
                </div>
              ) : (
                <div className="space-y-4">
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
                            "w-full rounded-[1.6rem] border px-4 py-4 sm:px-5",
                            isUser
                              ? "max-w-[min(100%,44rem)] border-primary/15 bg-primary text-primary-foreground"
                              : "max-w-[min(100%,52rem)] border-border/70 bg-background/82"
                          )}
                        >
                          <div className="flex flex-wrap items-center justify-between gap-3">
                            <div className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.18em]">
                              <span className={cn(
                                "size-2 rounded-full",
                                isUser ? "bg-primary-foreground/70" : "bg-primary/70"
                              )}
                              />
                              <span className={isUser ? "text-primary-foreground/78" : "text-muted-foreground"}>
                                {isUser ? t("localChat.operatorLabel") : t("localChat.beeLabel")}
                              </span>
                            </div>

                            <time className={cn(
                              "text-xs",
                              isUser ? "text-primary-foreground/72" : "text-muted-foreground"
                            )}
                            >
                              {formatMessageTimestamp(message.ts, i18n.language)}
                            </time>
                          </div>

                          {message.media_paths && message.media_paths.length > 0 && (
                            <div className="mt-4 space-y-3">
                              {message.media_paths.map((path) => (
                                <AttachmentPreview
                                  key={path}
                                  sessionId={sessionId}
                                  mediaPath={path}
                                  tone={message.role}
                                />
                              ))}
                            </div>
                          )}

                          {hasContent && (
                            message.role === "bee" ? (
                              <div className="prose prose-sm mt-4 max-w-none dark:prose-invert prose-p:my-3 prose-pre:rounded-2xl prose-pre:border prose-pre:border-border/70 prose-pre:bg-muted/35 prose-pre:px-4 prose-pre:py-3">
                                <Streamdown mode="static">{message.content}</Streamdown>
                              </div>
                            ) : (
                              <p className="mt-4 whitespace-pre-wrap text-sm leading-7">
                                {message.content}
                              </p>
                            )
                          )}
                        </article>
                      </div>
                    )
                  })}

                  {isProcessing && (
                    <div className="flex justify-start">
                      <div className="w-full max-w-[38rem] rounded-[1.6rem] border border-border/70 bg-background/82 px-4 py-4 sm:px-5">
                        <div className="flex items-center justify-between gap-3">
                          <div className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                            <span className="size-2 rounded-full bg-primary/70" />
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
                <div className="mb-4 rounded-2xl border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
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
                      <Paperclip className="size-3.5 text-muted-foreground" />
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

              <div className="rounded-[1.6rem] border border-border/70 bg-background/82 p-3">
                <textarea
                  ref={textareaRef}
                  className="max-h-[220px] min-h-[120px] w-full resize-none bg-transparent px-3 py-2 text-sm leading-7 placeholder:text-muted-foreground focus:outline-none"
                  placeholder={t("localChat.inputPlaceholder")}
                  value={input}
                  onChange={(event) => setInput(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" && !event.shiftKey) {
                      event.preventDefault()
                      void handleSend()
                    }
                  }}
                />

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

          <aside className="space-y-4 xl:sticky xl:top-6 xl:self-start">
            <DetailSection className="p-5">
              <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                {t("localChat.detailsPanelLabel")}
              </p>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                {t("localChat.detailsPanelHint")}
              </p>

              <div className="mt-5 space-y-4 border-t border-border/70 pt-4">
                <DetailField label={t("localChat.sessionIdLabel")} value={sessionId} mono />
                <DetailField
                  label={t("localChat.createdLabel")}
                  value={formatSessionTimestamp(currentSession?.created_at, i18n.language)}
                />
                <DetailField
                  label={t("localChat.updatedLabel")}
                  value={formatSessionTimestamp(latestTimestamp, i18n.language)}
                />
                <DetailField
                  label={t("localChat.latestResponseLabel")}
                  value={
                    latestBeeMessage
                      ? formatMessageTimestamp(latestBeeMessage.ts, i18n.language)
                      : t("localChat.latestResponseEmpty")
                  }
                />
                <DetailField
                  label={t("localChat.statusLabel")}
                  value={isProcessing ? t("localChat.processing") : t("localChat.idleStatus")}
                />
              </div>
            </DetailSection>

            <DetailSection className="p-5">
              <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                {t("localChat.queuePanelLabel")}
              </p>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                {t("localChat.queuePanelHint")}
              </p>

              {pendingMediaPaths.length > 0 ? (
                <div className="mt-5 space-y-3">
                  {pendingMediaPaths.map((path) => (
                    <QueuedMediaItem
                      key={path}
                      sessionId={sessionId}
                      mediaPath={path}
                      removeLabel={t("localChat.removeAttachment")}
                      openLabel={openAttachmentLabel}
                      onRemove={() =>
                        setPendingMediaPaths((prev) => prev.filter((entry) => entry !== path))
                      }
                    />
                  ))}
                </div>
              ) : (
                <div className="mt-5 rounded-2xl border border-dashed border-border/80 bg-background/75 px-4 py-4 text-sm leading-6 text-muted-foreground">
                  {t("localChat.queueEmpty")}
                </div>
              )}
            </DetailSection>
          </aside>
        </div>
      </div>
    </FadeIn>
  )
}
