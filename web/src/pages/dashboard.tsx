import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useWorkers } from "@/hooks/use-workers"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { StatusBadge } from "@/components/status-badge"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonCard } from "@/components/skeleton-loader"
import { Button } from "@/components/ui/button"

export function Dashboard() {
  const { data: workers = [], error, isLoading, refetch } = useWorkers()
  const { t } = useTranslation()

  const activeCount = workers.filter((w) => w.status === "working").length

  return (
    <FadeIn>
      <PageHeader
        title={t("dashboard.title")}
        subtitle={
          workers.length > 0
            ? activeCount > 0
              ? t("dashboard.summary", { count: activeCount })
              : t("dashboard.summaryNone")
            : undefined
        }
        actions={
          workers.length > 0 ? (
            <Link to="/workers">
              <Button>{t("workers.createWorker")}</Button>
            </Link>
          ) : undefined
        }
      />

      {error && (
        <div className="flex items-center gap-3 mb-4">
          <p className="text-destructive text-sm">{t("common.loadError")}</p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            {t("common.retry")}
          </Button>
        </div>
      )}

      {isLoading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5">
          {Array.from({ length: 4 }).map((_, i) => (
            <SkeletonCard key={i} />
          ))}
        </div>
      ) : workers.length === 0 && !error ? (
        <EmptyState
          title={t("emptyState.noWorkers")}
          description={t("emptyState.noWorkersDesc")}
          action={
            <Link to="/workers">
              <Button>{t("workers.createWorker")}</Button>
            </Link>
          }
        />
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5">
          {workers.map((w) => (
            <Link key={w.id} to={`/workers/${w.id}`}>
              <Card className="hover:ring-1 hover:ring-primary/30 transition-shadow duration-200 cursor-pointer h-full">
                <CardHeader className="pb-2">
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-base font-semibold">{w.name}</CardTitle>
                    <StatusBadge status={w.status} />
                  </div>
                </CardHeader>
                <CardContent>
                  <p className="text-sm text-muted-foreground line-clamp-2">
                    {w.description || t("common.noDescription")}
                  </p>
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </FadeIn>
  )
}
