import { useState, useMemo } from "react"
import { useParams, Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useWorker, useWorkerExecutions, useUpdateWorker } from "@/hooks/use-workers"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { Pencil } from "lucide-react"

const statusColor: Record<string, string> = {
  idle: "bg-green-100 text-green-800",
  working: "bg-blue-100 text-blue-800",
  error: "bg-red-100 text-red-800",
}

const execStatusColor: Record<string, string> = {
  pending: "bg-gray-100 text-gray-800",
  running: "bg-blue-100 text-blue-800",
  completed: "bg-green-100 text-green-800",
  failed: "bg-red-100 text-red-800",
}

export function WorkerDetail() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { data: worker, error: workerError } = useWorker(id!)
  const { data: executions = [] } = useWorkerExecutions(id!)

  const sessionGroups = useMemo(() => {
    const map = new Map<string, typeof executions>()
    for (const e of executions) {
      const group = map.get(e.session_id) ?? []
      group.push(e)
      map.set(e.session_id, group)
    }
    return Array.from(map.values()).sort((a, b) => {
      return (b[0].started_at ?? 0) - (a[0].started_at ?? 0)
    })
  }, [executions])

  const [isEditingDesc, setIsEditingDesc] = useState(false)
  const [editDesc, setEditDesc] = useState("")
  const [isEditingMemory, setIsEditingMemory] = useState(false)
  const [editMemory, setEditMemory] = useState("")
  const updateWorker = useUpdateWorker()

  if (!worker) return <p>Loading...</p>

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">{worker.name}</h1>
          {isEditingDesc ? (
            <div className="mt-1 space-y-2">
              <Textarea
                value={editDesc}
                onChange={(e) => setEditDesc(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Escape") { setIsEditingDesc(false); setEditDesc(worker.description) }
                }}
                autoFocus
                rows={2}
                className="text-sm"
              />
              <div className="flex gap-2">
                <Button size="sm" onClick={async () => {
                  await updateWorker.mutateAsync({ id: id!, data: { description: editDesc } })
                  setIsEditingDesc(false)
                }}>{t("common.save")}</Button>
                <Button size="sm" variant="outline" onClick={() => {
                  setIsEditingDesc(false)
                  setEditDesc(worker.description)
                }}>{t("common.cancel")}</Button>
              </div>
            </div>
          ) : (
            <div className="group flex items-center gap-1 mt-1">
              <p className="text-muted-foreground text-sm">
                {worker.description || t("common.noDescription")}
              </p>
              <button
                className="opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-foreground"
                onClick={() => { setEditDesc(worker.description); setIsEditingDesc(true) }}
              >
                <Pencil className="h-3 w-3" />
              </button>
            </div>
          )}
        </div>
        <div className="flex gap-2 items-center">
          <Badge className={statusColor[worker.status] || ""}>{worker.status}</Badge>
        </div>
      </div>

      {workerError && (
        <p className="text-red-500 mb-4">{workerError?.message}</p>
      )}

      <Tabs defaultValue="sessions">
        <TabsList>
          <TabsTrigger value="sessions">{t("workerDetail.sessions")}</TabsTrigger>
          <TabsTrigger value="info">{t("executionDetail.info")}</TabsTrigger>
        </TabsList>

        <TabsContent value="sessions" className="mt-4">
          {sessionGroups.length === 0 && (
            <p className="text-muted-foreground">{t("executions.noExecutions")}</p>
          )}
          <div className="space-y-3">
            {sessionGroups.map((group) => {
              const latest = group[0]
              const oldest = group[group.length - 1]
              return (
                <Card key={latest.session_id}>
                  <CardContent className="flex items-center justify-between py-4">
                    <div>
                      <Link
                        to={`/sessions/${latest.session_id}`}
                        className="font-mono text-sm hover:underline"
                      >
                        {latest.session_id.slice(0, 8)}...
                      </Link>
                      <p className="text-xs text-muted-foreground mt-1">
                        {oldest.started_at ? new Date(oldest.started_at).toLocaleString() : "-"}
                        {oldest.trigger_input && ` | ${oldest.trigger_input.slice(0, 50)}${oldest.trigger_input.length > 50 ? "..." : ""}`}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {t("executions.turnCount", { count: group.length })}
                      </p>
                    </div>
                    <Badge className={execStatusColor[latest.status] || ""}>
                      {latest.status}
                    </Badge>
                  </CardContent>
                </Card>
              )
            })}
          </div>
        </TabsContent>

        <TabsContent value="info" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>{t("workerDetail.workerInfo")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <p><strong>{t("workerDetail.id")}:</strong> <span className="font-mono text-sm">{worker.id}</span></p>
              <p><strong>{t("workerDetail.workDir")}:</strong> {worker.work_dir}</p>
              <p><strong>{t("workerDetail.created")}:</strong> {new Date(worker.created_at).toLocaleString()}</p>
              <div>
                <div className="group flex items-center gap-1">
                  <strong>{t("workerDetail.memory")}:</strong>
                  {!isEditingMemory && (
                    <button
                      className="opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-foreground"
                      onClick={() => { setEditMemory(worker.memory); setIsEditingMemory(true) }}
                    >
                      <Pencil className="h-3 w-3" />
                    </button>
                  )}
                </div>
                {isEditingMemory ? (
                  <div className="mt-1 space-y-2">
                    <Textarea
                      value={editMemory}
                      onChange={(e) => setEditMemory(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Escape") { setIsEditingMemory(false); setEditMemory(worker.memory) }
                      }}
                      autoFocus
                      rows={8}
                      className="text-sm font-mono"
                    />
                    <div className="flex gap-2">
                      <Button size="sm" onClick={async () => {
                        await updateWorker.mutateAsync({ id: id!, data: { memory: editMemory } })
                        setIsEditingMemory(false)
                      }}>{t("common.save")}</Button>
                      <Button size="sm" variant="outline" onClick={() => {
                        setIsEditingMemory(false)
                        setEditMemory(worker.memory)
                      }}>{t("common.cancel")}</Button>
                    </div>
                  </div>
                ) : (
                  worker.memory ? (
                    <pre className="mt-1 whitespace-pre-wrap text-sm bg-muted p-3 rounded-md">
                      {worker.memory}
                    </pre>
                  ) : (
                    <p className="mt-1 text-muted-foreground text-sm">{t("workerDetail.noMemory")}</p>
                  )
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
