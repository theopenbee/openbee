import { useParams, Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useExecution } from "@/hooks/use-executions"
import { LogViewer } from "@/components/log-viewer"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

const statusColor: Record<string, string> = {
  pending: "bg-gray-100 text-gray-800",
  running: "bg-blue-100 text-blue-800",
  completed: "bg-green-100 text-green-800",
  failed: "bg-red-100 text-red-800",
}

export function ExecutionDetail() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { data: execution, error: fetchError, refetch } = useExecution(id!)

  if (!execution) return <p>Loading...</p>

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">{t("executionDetail.title")}</h1>
          <p className="text-sm text-muted-foreground font-mono">{execution.id}</p>
        </div>
        <Badge className={statusColor[execution.status] || ""}>
          {execution.status}
        </Badge>
      </div>

      {fetchError && (
        <p className="text-red-500 mb-4">{fetchError.message}</p>
      )}

      <Tabs defaultValue="logs">
        <TabsList>
          <TabsTrigger value="logs">{t("executionDetail.logs")}</TabsTrigger>
          <TabsTrigger value="result">{t("executionDetail.result")}</TabsTrigger>
<TabsTrigger value="info">{t("executionDetail.info")}</TabsTrigger>
        </TabsList>

        <TabsContent value="logs" className="mt-4">
          <LogViewer
            executionId={execution.id}
            status={execution.status}
            logs={execution.logs}
            onComplete={refetch}
          />
        </TabsContent>

        <TabsContent value="result" className="mt-4">
          <Card>
            <CardContent className="pt-6">
              <pre className="whitespace-pre-wrap text-sm">
                {execution.result || t("executionDetail.noResult")}
              </pre>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="info" className="mt-4">
          <Card>
            <CardContent className="pt-6 space-y-2">
              <p><strong>{t("executionDetail.worker")}:</strong>{" "}
                <Link to={`/workers/${execution.worker_id}`} className="text-sm hover:underline">
                  {(execution as any).worker_name || (execution.worker_id?.slice(0, 8) ?? "") + "..."}
                </Link>
              </p>
              <p><strong>{t("executionDetail.session")}:</strong>{" "}
                <Link to={`/sessions/${execution.session_id}`} className="font-mono text-sm hover:underline">
                  {execution.session_id}
                </Link>
              </p>
              {execution.trigger_input && (
                <div>
                  <strong>{t("executionDetail.triggerInput")}:</strong>
                  <pre className="mt-1 whitespace-pre-wrap text-sm bg-muted p-2 rounded-md">
                    {execution.trigger_input}
                  </pre>
                </div>
              )}
              <p><strong>{t("executionDetail.pid")}:</strong> {execution.ai_process_pid || "N/A"}</p>
              <p><strong>{t("executionDetail.started")}:</strong> {execution.started_at ? new Date(execution.started_at).toLocaleString() : "-"}</p>
              <p><strong>{t("executionDetail.completed")}:</strong> {execution.completed_at ? new Date(execution.completed_at).toLocaleString() : "-"}</p>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
