import { useTranslation } from "react-i18next"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

interface MessagesCardProps {
  received: number
  sent: number
  loading?: boolean
}

export function MessagesCard({ received, sent, loading }: MessagesCardProps) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("dashboard.messages")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="flex gap-6">
            <Skeleton className="h-8 w-16" />
            <Skeleton className="h-8 w-16" />
          </div>
        ) : (
          <div className="flex gap-6">
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.messagesReceived")}</p>
              <p className="text-3xl font-bold" aria-live="polite">{received}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.messagesSent")}</p>
              <p className="text-3xl font-bold" aria-live="polite">{sent}</p>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
