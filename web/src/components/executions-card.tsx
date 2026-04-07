// web/src/components/executions-card.tsx
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
              <p className="text-3xl font-bold">{stats.total}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.executionsSuccess")}</p>
              <p className="text-3xl font-bold text-green-500">{stats.success}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.executionsFailed")}</p>
              <p className="text-3xl font-bold text-red-500">{stats.failed}</p>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
