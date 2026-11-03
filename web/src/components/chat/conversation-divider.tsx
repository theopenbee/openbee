import { memo } from "react"
import { useTranslation } from "react-i18next"
import { formatMessageTimestamp } from "@/lib/chat-format"

export const ConversationDivider = memo(function ConversationDivider({ ts }: { ts: number }) {
  const { t, i18n } = useTranslation()
  const timestamp = formatMessageTimestamp(ts, i18n.language)

  return (
    <div className="mt-8 mb-2 flex items-center gap-3 text-xs text-muted first:mt-0">
      <span aria-hidden="true" className="h-px flex-1 bg-border" />
      <p className="shrink-0">
        {t("chat.newConversation")}
        {timestamp && (
          <>
            {" · "}
            <time className="tabular">{timestamp}</time>
          </>
        )}
      </p>
      <span aria-hidden="true" className="h-px flex-1 bg-border" />
    </div>
  )
})
