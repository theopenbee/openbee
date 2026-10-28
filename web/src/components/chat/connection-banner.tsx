import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { useConnection } from "@/hooks/use-chat-events"
import { CHAT_COLUMN } from "@/lib/styles"
import { cn } from "@/lib/utils"

export function ConnectionBanner() {
  const { t } = useTranslation()
  const { online, reconnect } = useConnection()

  return (
    <div className={cn(CHAT_COLUMN, "shrink-0")}>
      <Alert open={!online} tone="warning" title={t("chat.connectionLost")} className="mb-3">
        {t("chat.connectionLostDescription")}{" "}
        <button
          type="button"
          onClick={reconnect}
          className="font-medium text-foreground underline decoration-border-strong underline-offset-2 transition-colors hover:decoration-foreground"
        >
          {t("chat.reconnectNow")}
        </button>
      </Alert>
    </div>
  )
}
