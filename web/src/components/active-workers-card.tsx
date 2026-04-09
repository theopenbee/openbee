import { useTranslation } from "react-i18next"
import { TrendingUp, TrendingDown, Minus } from "lucide-react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { formatChange } from "@/lib/format"

interface ActiveWorkersCardProps {
  today: number
  yesterday: number
  change: number | null
  loading?: boolean
}

export function ActiveWorkersCard({ today, yesterday, change, loading }: ActiveWorkersCardProps) {
  const { t } = useTranslation()

  const changeLabel = formatChange(change)

  const ChangeIcon = change === null ? null : change > 0 ? TrendingUp : change < 0 ? TrendingDown : Minus
  const changeColor = change === null ? "" : change > 0 ? "text-status-idle" : change < 0 ? "text-status-error" : "text-muted-foreground"

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("dashboard.activeWorkers")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="flex flex-wrap gap-6">
            <Skeleton className="h-8 w-16" />
            <Skeleton className="h-8 w-16" />
            <Skeleton className="h-8 w-16" />
          </div>
        ) : (
          <div className="flex flex-wrap items-end gap-6">
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.today")}</p>
              <p className="text-3xl font-bold" aria-live="polite">{today}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.yesterday")}</p>
              <p className="text-3xl font-bold text-muted-foreground">{yesterday}</p>
            </div>
            {changeLabel !== null && ChangeIcon && (
              <div
                className={`flex items-center gap-1 pb-1 ${changeColor}`}
                aria-label={changeLabel ?? undefined}
              >
                <ChangeIcon className="h-4 w-4" aria-hidden="true" />
                <span className="text-sm font-medium" aria-live="polite">{changeLabel}</span>
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
