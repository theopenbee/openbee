import { Fragment, useState, type FormEvent, type ComponentType } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useTranslation, Trans } from "react-i18next"
import { Copy, EyeIcon, MoreHorizontalIcon, Trash2Icon } from "lucide-react"
import { useWorkers, useDeleteWorker } from "@/hooks/use-workers"
import { useCan } from "@/hooks/use-can"
import { Perm } from "@/lib/permissions"
import { useDepartments } from "@/hooks/use-departments"
import { useIsMobile } from "@/hooks/use-mobile"
import { formatEngineLabel, formatRelative } from "@/lib/format"
import { cn } from "@/lib/utils"
import { ALERT_DESTRUCTIVE, FIELD_LABEL, SURFACE } from "@/lib/styles"
import { DepartmentTreeSidebar, UNGROUPED_FILTER } from "@/components/department-tree"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { WorkerAvatar } from "@/components/worker-avatar"
import { EmptyState } from "@/components/empty-state"
import { FadeIn } from "@/components/fade-in"
import { SkeletonTable } from "@/components/skeleton-loader"
import { StatusBadge } from "@/components/status-badge"

type DeleteStep = 1 | 2

// One row action in the worker dropdown. Read actions (View) are always present;
// write actions (Copy, Delete) are appended only when the user holds
// contacts:write, so the menu is built by filtering rather than per-item guards.
type WorkerRowAction = {
  key: string
  icon: ComponentType<{ className?: string }>
  label: string
  onClick: () => void
  destructive?: boolean
}

export function Workers() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const canWrite = useCan(Perm.ContactsWrite)
  const isMobile = useIsMobile()
  const [selectedDeptId, setSelectedDeptId] = useState<string | null>(null)
  const { data: departments = [] } = useDepartments()
  const deptFilter = selectedDeptId === UNGROUPED_FILTER ? undefined : (selectedDeptId ?? undefined)
  const { data: workers = [], error: fetchError, isLoading } = useWorkers(deptFilter)
  const displayedWorkers = selectedDeptId === UNGROUPED_FILTER
    ? workers.filter((w) => !w.departments || w.departments.length === 0)
    : workers
  const deleteWorker = useDeleteWorker()
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; name: string } | null>(null)
  const [deleteStep, setDeleteStep] = useState<DeleteStep>(1)
  const [deleteWorkDir, setDeleteWorkDir] = useState(false)
  const [deleteConfirmationText, setDeleteConfirmationText] = useState("")

  const resetDelete = () => {
    setDeleteTarget(null)
    setDeleteStep(1)
    setDeleteWorkDir(false)
    setDeleteConfirmationText("")
  }

  const error = fetchError?.message || deleteWorker.error?.message || ""
  const activeWorkers = displayedWorkers.filter((worker) => worker.status === "working").length
  const isDeleteNameConfirmed = deleteConfirmationText === (deleteTarget?.name ?? "")

  const handleDeleteConfirm = async () => {
    if (!deleteTarget || !isDeleteNameConfirmed) return
    await deleteWorker.mutateAsync({ id: deleteTarget.id, deleteWorkDir })
    resetDelete()
  }

  const handleDeleteStepOne = (e?: FormEvent) => {
    e?.preventDefault()
    if (!deleteTarget || !isDeleteNameConfirmed) return
    setDeleteStep(2)
  }

  const openDeleteDialog = (target: { id: string; name: string }) => {
    setDeleteStep(1)
    setDeleteWorkDir(false)
    setDeleteConfirmationText("")
    setDeleteTarget(target)
  }

  // The department filter is a fixed left rail from md up, and stacks under the
  // page header on narrow screens (where a side rail would starve the table).
  // Only one placement is mounted, so the tree (and each node's expanded
  // state) exists once; the CSS breakpoints just cover the first paint.
  const departmentFilter = (
    <DepartmentTreeSidebar
      departments={departments}
      selectedId={selectedDeptId}
      onSelect={setSelectedDeptId}
    />
  )

  return (
    <FadeIn className="h-full">
      <div className="flex h-full">
        {/* Left rail: department filter, flush to the layout edge with its own scroll. */}
        {!isMobile && (
          <aside className="hidden w-60 shrink-0 flex-col border-r border-border bg-background md:flex">
            <div className="flex h-18 shrink-0 items-center border-b border-border px-5">
              <h2 className="text-sm font-semibold text-strong">{t("departments.filter")}</h2>
            </div>
            <div className="min-h-0 flex-1">{departmentFilter}</div>
          </aside>
        )}

        {/* Content pane: header row on the base surface, worker table on the canvas. */}
        <div className="flex min-w-0 flex-1 flex-col">
          <header className="flex min-h-18 shrink-0 flex-wrap items-center justify-between gap-x-6 gap-y-3 border-b border-border bg-background px-6 py-3">
            <div className="min-w-0">
              <h1 className="text-xl leading-7 font-semibold tracking-[-0.015em] text-strong">
                {t("workers.title")}
              </h1>
              {displayedWorkers.length > 0 && (
                <p className="text-sm text-muted-foreground tabular-nums" aria-live="polite">
                  {t("workers.summary", { count: displayedWorkers.length, active: activeWorkers })}
                </p>
              )}
            </div>
            {canWrite && (
              <div className="flex shrink-0 items-center gap-2">
                <Button onClick={() => navigate("/workers/create")}>
                  {t("workers.createWorker")}
                </Button>
              </div>
            )}
          </header>

          <div className="min-w-0 flex-1 overflow-auto">
            {isMobile && (
              <div className="border-b border-border bg-background md:hidden">
                <h2 className="px-5 pt-3 text-sm font-semibold text-strong">{t("departments.filter")}</h2>
                <div className="max-h-56 overflow-y-auto">{departmentFilter}</div>
              </div>
            )}

            <div className="p-6">
              {error && (
                <div role="alert" className={cn(ALERT_DESTRUCTIVE, "mb-4")}>
                  {error}
                </div>
              )}

              {isLoading ? (
                <SkeletonTable rows={6} columns={4} />
              ) : displayedWorkers.length === 0 && !error ? (
                <EmptyState
                  framed
                  title={selectedDeptId !== null ? t("emptyState.noWorkersInGroup") : t("emptyState.noWorkers")}
                  description={selectedDeptId !== null ? t("emptyState.noWorkersInGroupDesc") : t("emptyState.noWorkersDesc")}
                  action={
                    selectedDeptId === null && canWrite ? (
                      <Button onClick={() => navigate("/workers/create")}>{t("workers.createWorker")}</Button>
                    ) : undefined
                  }
                />
              ) : (
                <div className={SURFACE}>
                  <Table className="table-fixed md:min-w-[760px]">
                    <TableHeader>
                      <TableRow>
                        <TableHead className="pl-4">{t("workers.columns.name")}</TableHead>
                        <TableHead className="hidden w-[120px] md:table-cell">{t("workers.columns.status")}</TableHead>
                        <TableHead className="hidden w-[150px] md:table-cell">{t("workers.columns.engine")}</TableHead>
                        <TableHead className="hidden w-[136px] md:table-cell">{t("workers.columns.activeTime")}</TableHead>
                        <TableHead className="w-20 pr-4 text-right">{t("workers.columns.actions")}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {displayedWorkers.map((w) => {
                        const rowActions: WorkerRowAction[] = [
                          { key: "view", icon: EyeIcon, label: t("common.view"), onClick: () => navigate(`/workers/${w.id}`) },
                          ...(canWrite
                            ? [
                                { key: "copy", icon: Copy, label: t("common.copy"), onClick: () => navigate(`/workers/create?copy=${w.id}`) },
                                { key: "delete", icon: Trash2Icon, label: t("common.delete"), destructive: true, onClick: () => openDeleteDialog({ id: w.id, name: w.name }) },
                              ]
                            : []),
                        ]
                        return (
                          <TableRow key={w.id}>
                            <TableCell className="pl-4">
                              <div className="flex min-w-0 items-center gap-3">
                                <WorkerAvatar name={w.name} status={w.status} />
                                <div className="min-w-0 flex-1">
                                  <Link
                                    to={`/workers/${w.id}`}
                                    className="block truncate rounded-sm text-sm font-medium text-strong underline-offset-4 hover:text-link hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                                  >
                                    {w.name}
                                  </Link>
                                  <p
                                    className="truncate text-body-sm leading-5 text-muted-foreground"
                                    title={w.description || undefined}
                                  >
                                    {w.description || "—"}
                                  </p>
                                  {/* Below md the status column folds in here. */}
                                  <div className="mt-1 md:hidden">
                                    <StatusBadge status={w.status} />
                                  </div>
                                </div>
                              </div>
                            </TableCell>
                            <TableCell className="hidden md:table-cell">
                              <StatusBadge status={w.status} />
                            </TableCell>
                            <TableCell className="hidden md:table-cell">
                              {w.engine ? (
                                <span className="flex min-w-0 items-center gap-2 text-body-sm text-foreground">
                                  <EngineIcon engine={w.engine} className="size-4 text-foreground" />
                                  <span className="truncate">{formatEngineLabel(w.engine, t)}</span>
                                </span>
                              ) : (
                                <span className="text-body-sm text-muted-foreground">—</span>
                              )}
                            </TableCell>
                            <TableCell className="hidden text-body-sm text-muted-foreground tabular-nums md:table-cell">
                              <span title={w.updated_at ? new Date(w.updated_at).toLocaleString() : undefined}>
                                {formatRelative(w.updated_at, t)}
                              </span>
                            </TableCell>
                            <TableCell className="pr-4 text-right">
                              <DropdownMenu>
                                <DropdownMenuTrigger
                                  render={
                                    <Button
                                      variant="ghost"
                                      size="icon-sm"
                                      aria-label={t("workers.columns.actions")}
                                    />
                                  }
                                >
                                  <MoreHorizontalIcon className="size-4" />
                                </DropdownMenuTrigger>
                                <DropdownMenuContent align="end" className="min-w-36">
                                  {rowActions.map((action, i) => (
                                    <Fragment key={action.key}>
                                      {i > 0 && <DropdownMenuSeparator />}
                                      <DropdownMenuItem
                                        variant={action.destructive ? "destructive" : undefined}
                                        onClick={action.onClick}
                                      >
                                        <action.icon className="size-4" />
                                        {action.label}
                                      </DropdownMenuItem>
                                    </Fragment>
                                  ))}
                                </DropdownMenuContent>
                              </DropdownMenu>
                            </TableCell>
                          </TableRow>
                        )
                      })}
                    </TableBody>
                  </Table>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      <Dialog open={!!deleteTarget} onOpenChange={(o) => { if (!o) resetDelete() }}>
        <DialogContent>
          <DialogHeader>
            <p className={FIELD_LABEL}>
              {deleteStep === 1 ? t("workers.deleteDialog.stepOne") : t("workers.deleteDialog.stepTwo")}
            </p>
            <DialogTitle>{t("workers.deleteDialog.title")}</DialogTitle>
            {deleteStep === 2 && (
              <DialogDescription>
                <Trans
                  i18nKey="workers.deleteDialog.stepTwoDescription"
                  values={{ name: deleteTarget?.name ?? "" }}
                  components={{ strong: <strong /> }}
                />
              </DialogDescription>
            )}
          </DialogHeader>
          {deleteStep === 1 ? (
            <form onSubmit={handleDeleteStepOne} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="delete-confirm-name">
                  {t("workers.deleteDialog.confirmNameLabel", { name: deleteTarget?.name ?? "" })}
                </Label>
                <Input
                  id="delete-confirm-name"
                  value={deleteConfirmationText}
                  onChange={(e) => setDeleteConfirmationText(e.target.value)}
                  placeholder={t("workers.deleteDialog.confirmNamePlaceholder")}
                />
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={resetDelete}>
                  {t("common.cancel")}
                </Button>
                <Button type="submit" disabled={!isDeleteNameConfirmed}>
                  {t("workers.deleteDialog.continue")}
                </Button>
              </DialogFooter>
            </form>
          ) : (
            <>
              <div className="flex items-center gap-2 py-2">
                <input
                  type="checkbox"
                  id="delete-work-dir"
                  checked={deleteWorkDir}
                  onChange={(e) => setDeleteWorkDir(e.target.checked)}
                  className="size-4 cursor-pointer rounded-sm accent-primary"
                />
                <Label htmlFor="delete-work-dir" className="cursor-pointer">
                  {t("workers.deleteDialog.deleteWorkDir")}
                </Label>
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={resetDelete}>
                  {t("common.cancel")}
                </Button>
                <Button
                  variant="destructive"
                  onClick={handleDeleteConfirm}
                  disabled={deleteWorker.isPending}
                >
                  {t("common.delete")}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </FadeIn>
  )
}
