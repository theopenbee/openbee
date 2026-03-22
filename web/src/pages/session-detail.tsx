import { useParams, Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useQueryClient } from "@tanstack/react-query"
import { useSessionExecutions } from "@/hooks/use-executions"
import { LogViewer } from "@/components/log-viewer"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { StatusBadge } from "@/components/status-badge"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"
import { EmptyState } from "@/components/empty-state"

export function SessionDetail() {
  const { t } = useTranslation()
  const { sessionId } = useParams<{ sessionId: string }>()
  const queryClient = useQueryClient()
  const { data: executions = [], error, isLoading } = useSessionExecutions(sessionId!)

  const workerId = executions[0]?.worker_id

  if (isLoading) return <SkeletonPage />
  if (!error && executions.length === 0) {
    return (
      <EmptyState title={t("sessionDetail.noExecutions")} />
    )
  }

  return (
    <FadeIn>
      <PageHeader
        title={t("sessionDetail.session")}
        subtitle={sessionId}
        actions={
          <div className="flex items-center gap-4 text-sm text-muted-foreground">
            {workerId && (
              <Link to={`/workers/${workerId}`} className="hover:text-primary transition-colors font-mono">
                {t("sessionDetail.worker")}: {workerId.slice(0, 8)}...
              </Link>
            )}
            <span className="font-mono">{t("executions.turnCount", { count: executions.length })}</span>
          </div>
        }
      />

      {error && <p className="text-destructive mb-4">{error.message}</p>}

      <div className="relative pl-8">
        {/* Amber timeline line */}
        <div className="absolute left-3 top-2 bottom-2 w-0.5 bg-primary/20" />

        <div className="space-y-4">
          {executions.map((exec, index) => (
            <div key={exec.id} className="relative">
              {/* Timeline dot */}
              <div className="absolute -left-8 top-4 w-6 h-6 rounded-full bg-background border-2 border-primary/40 flex items-center justify-center">
                <span className="text-[10px] font-mono text-primary font-medium">{index + 1}</span>
              </div>

              <Card>
                <CardHeader className="pb-2">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-muted-foreground font-mono">
                        {t("sessionDetail.turn", { index: index + 1 })} ·{" "}
                        <Link to={`/executions/${exec.id}`} className="text-primary hover:underline">
                          {exec.id.slice(0, 8)}...
                        </Link>
                      </span>
                    </div>
                    <StatusBadge status={exec.status} />
                  </div>
                  {exec.trigger_input && (
                    <p className="text-sm text-muted-foreground mt-1 truncate max-w-xl">
                      {exec.trigger_input}
                    </p>
                  )}
                  <div className="text-xs text-muted-foreground font-mono">
                    {exec.started_at && (
                      <>{t("executionDetail.started")}: {new Date(exec.started_at).toLocaleString()}</>
                    )}
                    {exec.completed_at && (
                      <> · {t("executionDetail.completed")}: {new Date(exec.completed_at).toLocaleString()}</>
                    )}
                  </div>
                </CardHeader>
                <CardContent>
                  <LogViewer
                    executionId={exec.id}
                    status={exec.status}
                    onComplete={
                      index === executions.length - 1
                        ? () => queryClient.invalidateQueries({ queryKey: ["sessions", sessionId, "executions"] })
                        : undefined
                    }
                  />
                </CardContent>
              </Card>
            </div>
          ))}
        </div>
      </div>
    </FadeIn>
  )
}
