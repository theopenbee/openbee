import { LoaderCircleIcon } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { useConnection } from "@/hooks/use-chat-events"
import { CHAT_COLUMN } from "@/lib/styles"
import { cn } from "@/lib/utils"

export function ConnectionBanner() {
  const { t } = useTranslation()
  const { online, retry, reconnect } = useConnection()
  const retrying = retry === "retrying"

  return (
    <div className={cn(CHAT_COLUMN, "shrink-0")}>
      <Alert open={!online} tone="warning" title={t("chat.connectionLost")} className="mb-3">
        {t(retry === "failed" ? "chat.reconnectFailed" : "chat.connectionLostDescription")}{" "}
        <button
          type="button"
          onClick={reconnect}
          disabled={retrying}
          aria-busy={retrying}
          className={cn(
            "inline-flex items-center gap-1 align-bottom font-medium transition-colors",
            retrying
              ? "cursor-default text-secondary"
              : "text-foreground underline decoration-border-strong underline-offset-2 hover:decoration-foreground",
          )}
        >
          {retrying && <LoaderCircleIcon size={12} strokeWidth={2} className="animate-spin" aria-hidden="true" />}
          {t(retrying ? "chat.reconnecting" : "chat.reconnectNow")}
        </button>
      </Alert>
    </div>
  )
}
