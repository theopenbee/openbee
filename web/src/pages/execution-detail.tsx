import { useParams, Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useExecution } from "@/hooks/use-executions"
import { LogViewer } from "@/components/log-viewer"
import { Card, CardContent } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { StatusBadge } from "@/components/status-badge"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"

export function ExecutionDetail() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { data: execution, error: fetchError, refetch } = useExecution(id!)

  if (!execution) return <SkeletonPage />

  return (
    <FadeIn>
      <PageHeader
        title={t("executionDetail.title")}
        subtitle={execution.id}
        actions={<StatusBadge status={execution.status} />}
      />

      {fetchError && (
        <p className="text-destructive mb-4">{fetchError.message}</p>
      )}

      <Tabs defaultValue="logs">
        <TabsList variant="line">
          <TabsTrigger value="logs">{t("executionDetail.logs")}</TabsTrigger>
          <TabsTrigger value="result">{t("executionDetail.result")}</TabsTrigger>
          <TabsTrigger value="info">{t("executionDetail.info")}</TabsTrigger>
        </TabsList>

        <TabsContent value="logs" className="mt-6">
          <LogViewer
            executionId={execution.id}
            status={execution.status}
            onComplete={refetch}
          />
        </TabsContent>

        <TabsContent value="result" className="mt-6">
          <Card>
            <CardContent className="pt-6">
              <pre className="whitespace-pre-wrap text-sm font-mono">
                {execution.result || t("executionDetail.noResult")}
              </pre>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="info" className="mt-6">
          <Card>
            <CardContent className="pt-6">
              <div className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-3 text-sm">
                <span className="text-muted-foreground">{t("executionDetail.worker")}</span>
                <Link to={`/workers/${execution.worker_id}`} className="font-mono text-primary hover:underline">
                  {(execution as any).worker_name || (execution.worker_id?.slice(0, 8) ?? "") + "..."}
                </Link>

                <span className="text-muted-foreground">{t("executionDetail.session")}</span>
                <Link to={`/sessions/${execution.session_id}`} className="font-mono text-primary hover:underline">
                  {execution.session_id}
                </Link>

                {execution.trigger_input && (
                  <>
                    <span className="text-muted-foreground">{t("executionDetail.triggerInput")}</span>
                    <pre className="whitespace-pre-wrap text-sm bg-secondary rounded-lg p-3 font-mono">
                      {execution.trigger_input}
                    </pre>
                  </>
                )}

                <span className="text-muted-foreground">{t("executionDetail.pid")}</span>
                <span className="font-mono">{execution.ai_process_pid || "N/A"}</span>

                <span className="text-muted-foreground">{t("executionDetail.started")}</span>
                <span className="font-mono">{execution.started_at ? new Date(execution.started_at).toLocaleString() : "-"}</span>

                <span className="text-muted-foreground">{t("executionDetail.completed")}</span>
                <span className="font-mono">{execution.completed_at ? new Date(execution.completed_at).toLocaleString() : "-"}</span>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </FadeIn>
  )
}
