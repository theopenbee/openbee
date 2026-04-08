import { useTranslation } from "react-i18next"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import type { ExecStats } from "@/lib/types"

interface ExecutionsCardProps {
  stats: ExecStats
  loading?: boolean
}

export function ExecutionsCard({ stats, loading }: ExecutionsCardProps) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("dashboard.executions")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="flex gap-4">
            <Skeleton className="h-8 w-12" />
            <Skeleton className="h-8 w-12" />
            <Skeleton className="h-8 w-12" />
          </div>
        ) : (
          <div className="flex gap-6">
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.executionsTotal")}</p>
              <p className="text-3xl font-bold" aria-live="polite">{stats.total}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.executionsSuccess")}</p>
              <p className="text-3xl font-bold text-status-idle" aria-live="polite">
                <span aria-label={`${t("dashboard.executionsSuccess")}: ${stats.success}`}>{stats.success}</span>
              </p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.executionsFailed")}</p>
              <p className="text-3xl font-bold text-status-error" aria-live="polite">
                <span aria-label={`${t("dashboard.executionsFailed")}: ${stats.failed}`}>{stats.failed}</span>
              </p>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
