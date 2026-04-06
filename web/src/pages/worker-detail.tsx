import { useMemo, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Activity, Building2, CalendarIcon, Check, FolderOpenIcon, Logs, Pencil, X } from "lucide-react"
import { useWorker, useWorkerExecutions, useUpdateWorker } from "@/hooks/use-workers"
import { useDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { DetailHero, DetailOverviewStat, DetailSection } from "@/components/detail-primitives"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { StatusBadge } from "@/components/status-badge"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"
import { EmptyState } from "@/components/empty-state"
import { PaginationControls } from "@/components/pagination-controls"
import { TaskList } from "@/components/task-list"
import { cn } from "@/lib/utils"
import type { DepartmentTree } from "@/lib/types"

const PAGE_SIZE = 20

function formatTimestamp(value: number | null | undefined) {
  if (!value) return "—"
  return new Date(value).toLocaleString()
}

function statusTone(status: string) {
  switch (status) {
    case "idle":
      return "text-status-idle"
    case "working":
      return "text-status-working"
    case "error":
      return "text-status-error"
    default:
      return "text-muted-foreground"
  }
}

function StatusDot({ status }: { status: string }) {
  const colorMap: Record<string, string> = {
    idle: "bg-status-idle",
    working: "bg-status-working",
    error: "bg-status-error",
  }
  const color = colorMap[status] ?? "bg-muted-foreground"

  return (
    <span className="relative inline-flex size-2 shrink-0">
      {status === "working" ? (
        <span className={cn("absolute inline-flex h-full w-full animate-ping rounded-full opacity-60", color)} />
      ) : null}
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
  const latestExecution = executions[0]

  const sessionGroups = useMemo(() => {
    const map = new Map<string, typeof executions>()
    for (const execution of executions) {
      const group = map.get(execution.session_id) ?? []
      group.push(execution)
      map.set(execution.session_id, group)
    }

    return Array.from(map.values()).sort((left, right) => {
      return (right[0].started_at ?? 0) - (left[0].started_at ?? 0)
    })
  }, [executions])

  const [isEditingDesc, setIsEditingDesc] = useState(false)
  const [editDesc, setEditDesc] = useState("")
  const [isEditingMemory, setIsEditingMemory] = useState(false)
  const [editMemory, setEditMemory] = useState("")
  const updateWorker = useUpdateWorker()
  const { data: departments = [] } = useDepartments()
  const setWorkerDepts = useSetWorkerDepartments()
  const [deptDialogOpen, setDeptDialogOpen] = useState(false)
  const [selectedDeptIds, setSelectedDeptIds] = useState<string[]>([])

  if (!worker) return <SkeletonPage />

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader
          title={worker.name}
          actions={<StatusBadge status={worker.status} />}
        />

        {workerError ? (
          <p className="text-destructive">{workerError.message}</p>
        ) : null}

        <DetailHero>
          <div className="flex flex-col gap-6 p-5 sm:p-6">
            <div className="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
              <div className="space-y-3">
                <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">
                  {t("workerDetail.workerInfo")}
                </p>

                <div className="space-y-3">
                  <h2 className="max-w-4xl break-all font-mono text-sm leading-7 text-foreground sm:text-base">
                    {worker.id}
                  </h2>

                  {isEditingDesc ? (
                    <div className="max-w-3xl space-y-3">
                      <Textarea
                        value={editDesc}
                        onChange={(event) => setEditDesc(event.target.value)}
                        onKeyDown={(event) => {
                          if (event.key === "Escape") {
                            setIsEditingDesc(false)
                            setEditDesc(worker.description)
                          }
                        }}
                        autoFocus
                        rows={3}
                        className="bg-background/80 text-sm"
                      />

                      <div className="flex flex-wrap gap-2">
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
                    <div className="flex flex-col items-start gap-3">
                      <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
                        {worker.description || t("common.noDescription")}
                      </p>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          setEditDesc(worker.description)
                          setIsEditingDesc(true)
                        }}
                        aria-label={t("workerDetail.editDescription")}
                      >
                        <Pencil className="size-3.5" />
                        {t("workerDetail.editDescription")}
                      </Button>
                    </div>
                  )}
                </div>
              </div>

              <div className="flex flex-col gap-2">
                <div className="flex flex-wrap items-center gap-2">
                  {worker.work_dir ? (
                    <span className="inline-flex max-w-full items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                      <FolderOpenIcon className="size-3.5 shrink-0" />
                      <span className="max-w-80 truncate font-mono text-foreground">{worker.work_dir}</span>
                    </span>
                  ) : null}

                  <span className="inline-flex items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                    <CalendarIcon className="size-3.5 shrink-0" />
                    <span>{new Date(worker.created_at).toLocaleDateString()}</span>
                  </span>
                </div>

                {/* Department badges */}
                <div className="flex flex-wrap items-center gap-2 pt-2">
                  <span className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                    {t("departments.title")}
                  </span>
                  {worker.departments && worker.departments.length > 0 ? (
                    worker.departments.map((d) => (
                      <span
                        key={d.id}
                        className="inline-flex items-center gap-1.5 rounded-full border border-border bg-background px-2.5 py-1 text-xs text-muted-foreground"
                      >
                        <Building2 className="size-3 shrink-0" />
                        {d.name}
                      </span>
                    ))
                  ) : (
                    <span className="text-xs text-muted-foreground">{t("departments.ungrouped")}</span>
                  )}
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      setSelectedDeptIds(worker.departments?.map((d) => d.id) ?? [])
                      setDeptDialogOpen(true)
                    }}
                  >
                    <Pencil className="size-3" />
                    {t("departments.manage")}
                  </Button>
                </div>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <DetailOverviewStat
                icon={Activity}
                label={t("executions.columns.latestStatus")}
                value={
                  <span className={cn("inline-flex items-center gap-2", statusTone(worker.status))}>
                    <StatusDot status={worker.status} />
                    <span className="font-medium">{worker.status}</span>
                  </span>
                }
              />
              <DetailOverviewStat
                icon={Logs}
                label={t("executions.columns.turns")}
                value={<span className="font-mono text-sm sm:text-base">{data?.total ?? 0}</span>}
                hint={latestExecution ? formatTimestamp(latestExecution.started_at) : t("executions.noExecutions")}
              />
              <DetailOverviewStat
                icon={FolderOpenIcon}
                label={t("workerDetail.workDir")}
                valueClassName="font-mono text-sm leading-6 break-all"
                value={worker.work_dir || "—"}
              />
              <DetailOverviewStat
                icon={CalendarIcon}
                label={t("workerDetail.created")}
                value={<span className="font-mono text-sm sm:text-base">{formatTimestamp(worker.created_at)}</span>}
              />
            </div>
          </div>
        </DetailHero>

        <Tabs defaultValue="sessions">
          <TabsList variant="line">
            <TabsTrigger value="sessions">{t("workerDetail.sessions")}</TabsTrigger>
            <TabsTrigger value="tasks">{t("tasks.title")}</TabsTrigger>
            <TabsTrigger value="memory">{t("workerDetail.memory")}</TabsTrigger>
          </TabsList>

          <TabsContent value="sessions" className="mt-6 space-y-4">
            <DetailSection>
              <div className="border-b border-border/70 px-5 py-4 sm:px-6">
                <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
                  <div>
                    <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                      {t("workerDetail.sessions")}
                    </p>
                    <p className="mt-1 text-sm leading-6 text-muted-foreground">
                      {t("executions.turnCount", { count: data?.total ?? 0 })}
                    </p>
                  </div>

                  <span className="font-mono text-xs text-muted-foreground">
                    {t("executions.pagination.page", { page, totalPages })}
                  </span>
                </div>
              </div>

              {sessionGroups.length === 0 ? (
                <div className="px-5 py-10 sm:px-6">
                  <EmptyState title={t("executions.noExecutions")} />
                </div>
              ) : (
                <div className="divide-y divide-border/70">
                  {sessionGroups.map((group) => {
                    const latest = group[0]
                    const oldest = group[group.length - 1]
                    const isRunning = latest.status === "running"

                    return (
                      <div
                        key={latest.session_id}
                        className={cn(
                          "flex items-center gap-4 px-5 py-4 transition-colors hover:bg-primary/5 sm:px-6",
                          isRunning && "border-l-2 border-l-status-working"
                        )}
                      >
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-2.5">
                            <Link
                              to={`/sessions/${latest.session_id}`}
                              className="font-mono text-sm text-primary transition-colors hover:text-primary/80 hover:underline"
                            >
                              {latest.session_id}
                            </Link>
                            <span className="text-xs text-muted-foreground">
                              {t("executions.turnCount", { count: group.length })}
                            </span>
                          </div>

                          {(oldest.started_at || oldest.trigger_input) ? (
                            <p className="mt-1 max-w-3xl truncate text-sm text-muted-foreground">
                              {oldest.started_at ? (
                                <span className="mr-2 font-mono text-xs">{formatTimestamp(oldest.started_at)}</span>
                              ) : null}
                              {oldest.trigger_input ? (
                                <span>{oldest.trigger_input}</span>
                              ) : null}
                            </p>
                          ) : null}
                        </div>

                        <StatusBadge status={latest.status} />
                      </div>
                    )
                  })}
                </div>
              )}
            </DetailSection>

            <PaginationControls page={page} totalPages={totalPages} onPageChange={setPage} />
          </TabsContent>

          <TabsContent value="tasks" className="mt-6">
            <TaskList workerId={id!} />
          </TabsContent>

          <TabsContent value="memory" className="mt-6">
            <DetailSection className="p-5 sm:p-6">
              <div className="flex flex-col gap-6">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                      {t("workerDetail.memory")}
                    </p>
                    <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                      {t("workers.form.memoryHelper")}
                    </p>
                  </div>

                  {!isEditingMemory ? (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setEditMemory(worker.memory || "")
                        setIsEditingMemory(true)
                      }}
                      aria-label={t("workerDetail.editMemory")}
                    >
                      <Pencil className="size-3.5" />
                      {t("workerDetail.editMemory")}
                    </Button>
                  ) : null}
                </div>

                {isEditingMemory ? (
                  <div className="space-y-3">
                    <Textarea
                      value={editMemory}
                      onChange={(event) => setEditMemory(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Escape") {
                          setIsEditingMemory(false)
                          setEditMemory(worker.memory || "")
                        }
                      }}
                      autoFocus
                      rows={12}
                      className="font-mono text-sm"
                    />

                    <div className="flex flex-wrap gap-2">
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
                          setEditMemory(worker.memory || "")
                        }}
                      >
                        <X className="size-3" />
                        {t("common.cancel")}
                      </Button>
                    </div>
                  </div>
                ) : worker.memory ? (
                  <div className="rounded-2xl border border-border/70 bg-background/80 p-4">
                    <pre className="whitespace-pre-wrap break-words font-mono text-sm leading-6 text-foreground">
                      {worker.memory}
                    </pre>
                  </div>
                ) : (
                  <div className="rounded-2xl border border-dashed border-border/80 bg-background/75 px-4 py-8 text-sm leading-6 text-muted-foreground">
                    {t("workerDetail.noMemory")}
                  </div>
                )}
              </div>
            </DetailSection>
          </TabsContent>
        </Tabs>
      </div>

      <Dialog open={deptDialogOpen} onOpenChange={setDeptDialogOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("departments.title")}</DialogTitle>
            <DialogDescription>{t("departments.manageDescription")}</DialogDescription>
          </DialogHeader>
          <div className="max-h-64 overflow-y-auto space-y-1">
            {flattenDeptTree(departments).map(({ dept, depth }) => (
              <label
                key={dept.id}
                className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-muted cursor-pointer"
                style={{ paddingLeft: `${depth * 16 + 8}px` }}
              >
                <input
                  type="checkbox"
                  checked={selectedDeptIds.includes(dept.id)}
                  onChange={(e) => {
                    if (e.target.checked) {
                      setSelectedDeptIds([...selectedDeptIds, dept.id])
                    } else {
                      setSelectedDeptIds(selectedDeptIds.filter((id) => id !== dept.id))
                    }
                  }}
                  className="size-4 rounded accent-primary"
                />
                <span className="text-sm">{dept.name}</span>
              </label>
            ))}
            {departments.length === 0 && (
              <p className="text-sm text-muted-foreground text-center py-4">
                {t("departments.empty")}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeptDialogOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={async () => {
                await setWorkerDepts.mutateAsync({ workerId: id!, departmentIds: selectedDeptIds })
                setDeptDialogOpen(false)
              }}
              disabled={setWorkerDepts.isPending}
            >
              {t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </FadeIn>
  )
}

function flattenDeptTree(
  tree: DepartmentTree[],
  depth = 0
): { dept: { id: string; name: string }; depth: number }[] {
  const result: { dept: { id: string; name: string }; depth: number }[] = []
  for (const node of tree) {
    result.push({ dept: { id: node.id, name: node.name }, depth })
    if (node.children.length > 0) {
      result.push(...flattenDeptTree(node.children, depth + 1))
    }
  }
  return result
}
