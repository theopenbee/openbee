import { memo } from "react"
import { useTranslation } from "react-i18next"
import { AttachmentPreview } from "@/components/chat/attachment-preview"
import { ChatMarkdown } from "@/components/chat-markdown"
import { CopyButton } from "@/components/copy-button"
import { ExpandableContent } from "@/components/expandable-content"
import { formatMessageTimestamp } from "@/lib/chat-format"
import type { ChatMessage } from "@/lib/types"
import { cn } from "@/lib/utils"

const COLLAPSE_HEIGHT = 320

export const MessageBubble = memo(function MessageBubble({
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
