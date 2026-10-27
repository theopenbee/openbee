import { memo, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { Badge } from "@/components/arc/badge/badge"
import { AttachmentPreview } from "@/components/chat/attachment-preview"
import { ChatMarkdown } from "@/components/chat-markdown"
import { CopyButton } from "@/components/copy-button"
import { ExpandableContent } from "@/components/expandable-content"
import { formatMessageTimestamp } from "@/lib/chat-format"
import type { ChatMessage } from "@/lib/types"
import { cn } from "@/lib/utils"

const COLLAPSE_HEIGHT = 320

const COPY_REVEAL =
  "transition-opacity pointer-fine:opacity-0 pointer-fine:group-hover:opacity-100 pointer-fine:focus-within:opacity-100"

export interface MessageSender {
  name: string
  avatar: ReactNode
  via?: string
}

interface MessageBubbleProps {
  message: ChatMessage
  sender: MessageSender | null
  isGroupStart: boolean
  onExpandToggle: () => void
  body?: ReactNode
}

function Attachments({ paths, hasContent, isUser }: { paths: string[]; hasContent: boolean; isUser: boolean }) {
  if (paths.length === 0) return null
  return (
    <div
      className={cn(
        "flex max-w-full flex-col gap-2",
        hasContent && "mb-2",
        isUser ? "items-end sm:max-w-[min(85%,32rem)]" : "items-start sm:max-w-md"
      )}
    >
      {paths.map((path) => (
        <AttachmentPreview key={path} mediaPath={path} />
      ))}
    </div>
  )
}

export const MessageBubble = memo(function MessageBubble({
  message,
  sender,
  isGroupStart,
  onExpandToggle,
  body,
}: MessageBubbleProps) {
  const { t, i18n } = useTranslation()
  const hasContent = message.content.trim().length > 0
  const mediaPaths = message.media_paths ?? []
  const timestamp = formatMessageTimestamp(message.ts, i18n.language)

  if (!sender) {
    return (
      <div className={cn("group flex min-w-0 flex-col items-end first:mt-0", isGroupStart ? "mt-6" : "mt-2")}>
        {isGroupStart && (
          <div className="mb-2 flex items-baseline gap-2 text-xs">
            <span className="sr-only">{t("chat.you")}</span>
            {timestamp && <time className="text-muted tabular">{timestamp}</time>}
          </div>
        )}

        <Attachments paths={mediaPaths} hasContent={hasContent} isUser />

        {hasContent && (
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
            <span className={cn("absolute top-1 right-full mr-1 inline-flex", COPY_REVEAL)}>
              <CopyButton value={message.content} label={t("chat.copyMessage")} />
            </span>
          </div>
        )}
      </div>
    )
  }

  return (
    <div className={cn("group flex min-w-0 gap-3 first:mt-0", isGroupStart ? "mt-6" : "mt-2")}>
      <div className="w-7 shrink-0">{isGroupStart && sender.avatar}</div>
      <div className="flex min-w-0 flex-1 flex-col items-start">
        {isGroupStart && (
          <div className="mb-1.5 flex min-h-7 min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs">
            <span className="truncate font-medium text-foreground">{sender.name}</span>
            {sender.via && (
              <Badge tone="neutral" size="sm">
                {sender.via}
              </Badge>
            )}
            {timestamp && <time className="text-muted tabular">{timestamp}</time>}
          </div>
        )}

        {body ?? (
          <>
            <Attachments paths={mediaPaths} hasContent={hasContent} isUser={false} />

            {hasContent && (
              <div className="relative w-full min-w-0">
                <ExpandableContent maxHeight={COLLAPSE_HEIGHT} onToggle={onExpandToggle}>
                  <ChatMarkdown breaks>{message.content}</ChatMarkdown>
                </ExpandableContent>
                <span className={cn("absolute -top-1.5 right-full mr-0.5 inline-flex", COPY_REVEAL)}>
                  <CopyButton value={message.content} label={t("chat.copyMessage")} />
                </span>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
})
