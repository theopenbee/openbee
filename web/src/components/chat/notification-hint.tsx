import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { useNotificationSettings } from "@/hooks/use-notifications"
import type { NotificationHint as Hint } from "@/lib/notifications"
import { CHAT_COLUMN } from "@/lib/styles"
import { cn } from "@/lib/utils"

export function NotificationHint() {
  const { t } = useTranslation()
  const { hint, enable, dismiss } = useNotificationSettings()
  const [shown, setShown] = useState<Hint>(hint ?? "prompt")
  if (hint !== null && hint !== shown) setShown(hint)
  const blocked = shown === "blocked"

  return (
    <div className={cn(CHAT_COLUMN, "shrink-0")}>
      <Alert
        open={hint !== null}
        tone={blocked ? "warning" : "info"}
        title={t(blocked ? "notifications.blockedTitle" : "notifications.promptTitle")}
        onDismiss={dismiss}
        className="mt-3"
      >
        {blocked ? (
          t("notifications.blockedDescription")
        ) : (
          <>
            {t("notifications.promptDescription")}{" "}
            <button
              type="button"
              onClick={() => void enable()}
              className="font-medium text-foreground underline decoration-border-strong underline-offset-2 transition-colors hover:decoration-foreground"
            >
              {t("notifications.enable")}
            </button>
          </>
        )}
      </Alert>
    </div>
  )
}
