import { useParams, Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useQueryClient } from "@tanstack/react-query"
import { useSessionExecutions } from "@/hooks/use-executions"
import { LogViewer } from "@/components/log-viewer"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"

const statusColor: Record<string, string> = {
  pending: "bg-gray-100 text-gray-800",
  running: "bg-blue-100 text-blue-800",
  completed: "bg-green-100 text-green-800",
  failed: "bg-red-100 text-red-800",
}

export function SessionDetail() {
  const { t } = useTranslation()
  const { sessionId } = useParams<{ sessionId: string }>()
  const queryClient = useQueryClient()
  const { data: executions = [], error } = useSessionExecutions(sessionId!)

  const workerId = executions[0]?.worker_id

  if (executions.length === 0 && !error) return <p>Loading...</p>

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">{t("sessionDetail.session")}</h1>
          <p className="text-sm text-muted-foreground font-mono">{sessionId}</p>
        </div>
        <div className="flex items-center gap-4 text-sm text-muted-foreground">
          {workerId && (
            <Link to={`/workers/${workerId}`} className="hover:underline font-mono">
              {t("sessionDetail.worker")}: {workerId.slice(0, 8)}...
            </Link>
          )}
          <span>{t("executions.turnCount", { count: executions.length })}</span>
        </div>
      </div>

      {error && <p className="text-red-500 mb-4">{error.message}</p>}

      <div className="space-y-4">
        {executions.map((exec, index) => (
          <div key={exec.id} className="relative">
            {index < executions.length - 1 && (
              <div className="absolute left-6 top-full h-4 w-0.5 bg-border z-10" />
            )}
            <Card>
              <CardHeader className="pb-2">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-muted-foreground font-mono">
                      {t("sessionDetail.turn", { index: index + 1 })} ·{" "}
                      <Link to={`/executions/${exec.id}`} className="hover:underline">
                        {exec.id.slice(0, 8)}...
                      </Link>
                    </span>
                  </div>
                  <Badge className={statusColor[exec.status] || ""}>{exec.status}</Badge>
                </div>
                {exec.trigger_input && (
                  <p className="text-sm text-muted-foreground mt-1 truncate max-w-xl">
                    {exec.trigger_input}
                  </p>
                )}
                <div className="text-xs text-muted-foreground">
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
                  logs={exec.logs}
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
  )
}
