import { useState, useMemo } from "react"
import { useParams, Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useWorker, useWorkerExecutions, useUpdateWorker } from "@/hooks/use-workers"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { CalendarIcon, Check, FolderOpenIcon, HashIcon, Pencil, X } from "lucide-react"
import { StatusBadge } from "@/components/status-badge"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"
import { EmptyState } from "@/components/empty-state"
import { PaginationControls } from "@/components/pagination-controls"
import { TaskList } from "@/components/task-list"
import { cn } from "@/lib/utils"

const PAGE_SIZE = 20

function StatusDot({ status }: { status: string }) {
  const colorMap: Record<string, string> = {
    idle: "bg-status-idle",
    working: "bg-status-working",
    error: "bg-status-error",
  }
  const color = colorMap[status] ?? "bg-muted-foreground"
  return (
    <span className="relative inline-flex size-2 shrink-0">
      {status === "working" && (
        <span className={cn("absolute inline-flex h-full w-full animate-ping rounded-full opacity-60", color)} />
      )}
      <span className={cn("relative inline-flex size-2 rounded-full", color)} />
    </span>
  )
}

export function WorkerDetail() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { data: worker, error: workerError } = useWorker(id!)
  const [page, setPage] = useState(1)
  const { data } = useWorkerExecutions(id!, page, PAGE_SIZE)

  const executions = data?.items ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))

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

  if (!worker) return <SkeletonPage />

  return (
    <FadeIn>
      {/* ── Worker header ───────────────────────────────────────────── */}
      <div className="mb-8">
        <div className="flex items-start justify-between gap-6">
          {/* Left: name + description */}
          <div className="min-w-0 flex-1">
            <h1 className="text-2xl font-bold tracking-tight">{worker.name}</h1>

            {isEditingDesc ? (
              <div className="mt-2 space-y-2">
                <Textarea
                  value={editDesc}
                  onChange={(e) => setEditDesc(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Escape") {
                      setIsEditingDesc(false)
                      setEditDesc(worker.description)
                    }
                  }}
                  autoFocus
                  rows={2}
                  className="text-sm"
                />
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    onClick={async () => {
                      await updateWorker.mutateAsync({ id: id!, data: { description: editDesc } })
                      setIsEditingDesc(false)
                    }}
                  >
                    <Check className="size-3" />
                    {t("common.save")}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setIsEditingDesc(false)
                      setEditDesc(worker.description)
                    }}
                  >
                    <X className="size-3" />
                    {t("common.cancel")}
                  </Button>
                </div>
              </div>
            ) : (
              <button
                className="group mt-1.5 flex items-start gap-1.5 text-left"
                onClick={() => {
                  setEditDesc(worker.description)
                  setIsEditingDesc(true)
                }}
                aria-label={t("workerDetail.editDescription")}
              >
                <span className="text-sm leading-relaxed text-muted-foreground">
                  {worker.description || t("common.noDescription")}
                </span>
                <Pencil className="mt-0.5 size-3 shrink-0 text-muted-foreground/40 opacity-0 transition-opacity group-hover:opacity-100" />
              </button>
            )}
          </div>

          {/* Right: status */}
          <div className="flex shrink-0 items-center gap-2 pt-1">
            <StatusDot status={worker.status} />
            <StatusBadge status={worker.status} />
          </div>
        </div>

        {/* Meta bar */}
        <div className="mt-4 flex flex-wrap items-center gap-x-5 gap-y-1.5 border-t border-border pt-3">
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <HashIcon className="size-3 shrink-0" />
            <span className="font-mono">{worker.id.slice(0, 8)}…{worker.id.slice(-4)}</span>
          </span>
          {worker.work_dir ? (
            <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <FolderOpenIcon className="size-3 shrink-0" />
              <span className="max-w-[20rem] truncate font-mono">{worker.work_dir}</span>
            </span>
          ) : null}
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <CalendarIcon className="size-3 shrink-0" />
            <span>{new Date(worker.created_at).toLocaleDateString()}</span>
          </span>
        </div>
      </div>

      {workerError && (
        <p className="mb-4 text-destructive">{workerError?.message}</p>
      )}

      {/* ── Tabs ────────────────────────────────────────────────────── */}
      <Tabs defaultValue="sessions">
        <TabsList variant="line">
          <TabsTrigger value="sessions">{t("workerDetail.sessions")}</TabsTrigger>
          <TabsTrigger value="tasks">{t("tasks.title")}</TabsTrigger>
          <TabsTrigger value="memory">{t("workerDetail.memory")}</TabsTrigger>
        </TabsList>

        {/* Sessions */}
        <TabsContent value="sessions" className="mt-6">
          {sessionGroups.length === 0 ? (
            <EmptyState title={t("executions.noExecutions")} />
          ) : (
            <div className="overflow-hidden rounded-xl bg-card ring-1 ring-foreground/5 divide-y divide-border">
              {sessionGroups.map((group) => {
                const latest = group[0]
                const oldest = group[group.length - 1]
                const isRunning = latest.status === "running"
                return (
                  <div
                    key={latest.session_id}
                    className={cn(
                      "flex items-center gap-4 px-4 py-3.5 transition-colors hover:bg-primary/5",
                      isRunning && "border-l-2 border-l-status-working"
                    )}
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2.5">
                        <Link
                          to={`/sessions/${latest.session_id}`}
                          className="font-mono text-sm text-primary hover:underline"
                        >
                          {latest.session_id.slice(0, 8)}…
                        </Link>
                        <span className="text-xs text-muted-foreground">
                          {t("executions.turnCount", { count: group.length })}
                        </span>
                      </div>
                      {(oldest.started_at || oldest.trigger_input) && (
                        <p className="mt-0.5 max-w-xl truncate text-xs text-muted-foreground">
                          {oldest.started_at && (
                            <span className="mr-2">{new Date(oldest.started_at).toLocaleString()}</span>
                          )}
                          {oldest.trigger_input && (
                            <span className="text-foreground/55">
                              {oldest.trigger_input.slice(0, 80)}
                              {oldest.trigger_input.length > 80 ? "…" : ""}
                            </span>
                          )}
                        </p>
                      )}
                    </div>
                    <div className="shrink-0">
                      <StatusBadge status={latest.status} />
                    </div>
                  </div>
                )
              })}
            </div>
          )}
          <PaginationControls page={page} totalPages={totalPages} onPageChange={setPage} />
        </TabsContent>

        {/* Tasks */}
        <TabsContent value="tasks" className="mt-6">
          <TaskList workerId={id!} />
        </TabsContent>

        {/* Memory */}
        <TabsContent value="memory" className="mt-6">
          {isEditingMemory ? (
            <div className="space-y-3">
              <Textarea
                value={editMemory}
                onChange={(e) => setEditMemory(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Escape") {
                    setIsEditingMemory(false)
                    setEditMemory(worker.memory)
                  }
                }}
                autoFocus
                rows={12}
                className="font-mono text-sm"
              />
              <div className="flex gap-2">
                <Button
                  size="sm"
                  onClick={async () => {
                    await updateWorker.mutateAsync({ id: id!, data: { memory: editMemory } })
                    setIsEditingMemory(false)
                  }}
                >
                  <Check className="size-3" />
                  {t("common.save")}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setIsEditingMemory(false)
                    setEditMemory(worker.memory)
                  }}
                >
                  <X className="size-3" />
                  {t("common.cancel")}
                </Button>
              </div>
            </div>
          ) : worker.memory ? (
            <button
              className="group relative w-full text-left"
              onClick={() => {
                setEditMemory(worker.memory)
                setIsEditingMemory(true)
              }}
              aria-label={t("workerDetail.editMemory")}
            >
              <pre className="whitespace-pre-wrap rounded-xl bg-secondary p-4 font-mono text-sm leading-relaxed ring-1 ring-foreground/5 transition-all group-hover:ring-primary/25">
                {worker.memory}
              </pre>
              <span className="absolute right-3 top-3 flex items-center gap-1 rounded-md bg-background px-2 py-1 text-xs text-muted-foreground opacity-0 shadow-sm ring-1 ring-foreground/10 transition-opacity group-hover:opacity-100">
                <Pencil className="size-2.5" />
                {t("workerDetail.editMemory")}
              </span>
            </button>
          ) : (
            <button
              className="group flex w-full flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border py-14 text-center transition-colors hover:border-primary/30 hover:bg-primary/[0.02]"
              onClick={() => {
                setEditMemory("")
                setIsEditingMemory(true)
              }}
            >
              <Pencil className="size-4 text-muted-foreground/40 transition-colors group-hover:text-muted-foreground/70" />
              <span className="text-sm text-muted-foreground">{t("workerDetail.noMemory")}</span>
            </button>
          )}
        </TabsContent>
      </Tabs>
    </FadeIn>
  )
}
