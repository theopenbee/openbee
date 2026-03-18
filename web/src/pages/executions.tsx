import { useMemo } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useExecutions } from "@/hooks/use-executions"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

const statusColor: Record<string, string> = {
  pending: "bg-gray-100 text-gray-800",
  running: "bg-blue-100 text-blue-800",
  completed: "bg-green-100 text-green-800",
  failed: "bg-red-100 text-red-800",
}

export function Executions() {
  const { t } = useTranslation()
  const { data: executions = [], error } = useExecutions()

  const sessionGroups = useMemo(() => {
    const map = new Map<string, typeof executions>()
    for (const e of executions) {
      const group = map.get(e.session_id) ?? []
      group.push(e)
      map.set(e.session_id, group)
    }
    // Each group: executions already ordered DESC from API, so first = latest
    return Array.from(map.values()).sort((a, b) => {
      return (b[0].started_at ?? 0) - (a[0].started_at ?? 0)
    })
  }, [executions])

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">{t("executions.title")}</h1>
      {error && <p className="text-red-500 mb-4">{error.message}</p>}

      {sessionGroups.length === 0 && !error && (
        <p className="text-muted-foreground">{t("executions.noExecutions")}</p>
      )}

      {sessionGroups.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("executions.columns.session")}</TableHead>
              <TableHead>{t("executions.columns.worker")}</TableHead>
              <TableHead>{t("executions.columns.turns")}</TableHead>
              <TableHead>{t("executions.columns.latestStatus")}</TableHead>
              <TableHead>{t("executions.columns.started")}</TableHead>
              <TableHead>{t("executions.columns.lastCompleted")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sessionGroups.map((group) => {
              const latest = group[0]
              const oldest = group[group.length - 1]
              const lastCompleted = group.find((e) => e.completed_at)
              return (
                <TableRow key={latest.session_id}>
                  <TableCell>
                    <Link
                      to={`/sessions/${latest.session_id}`}
                      className="font-mono text-sm hover:underline"
                    >
                      {latest.session_id.slice(0, 8)}...
                    </Link>
                  </TableCell>
                  <TableCell>
                    {latest.worker_id ? (
                      <Link
                        to={`/workers/${latest.worker_id}`}
                        className="text-sm hover:underline"
                      >
                        {(latest as any).worker_name || latest.worker_id.slice(0, 8) + "..."}
                      </Link>
                    ) : (
                      <span className="text-sm text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell className="text-sm">{t("executions.turnCount", { count: group.length })}</TableCell>
                  <TableCell>
                    <Badge className={statusColor[latest.status] || ""}>{latest.status}</Badge>
                  </TableCell>
                  <TableCell className="text-sm">
                    {oldest.started_at ? new Date(oldest.started_at).toLocaleString() : "-"}
                  </TableCell>
                  <TableCell className="text-sm">
                    {lastCompleted?.completed_at
                      ? new Date(lastCompleted.completed_at).toLocaleString()
                      : "-"}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}
    </div>
  )
}
