import { useState, type FormEvent } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useTranslation, Trans } from "react-i18next"
import { Copy, EyeIcon, MoreHorizontalIcon, Trash2Icon } from "lucide-react"
import { useWorkers, useDeleteWorker } from "@/hooks/use-workers"
import { useDepartments } from "@/hooks/use-departments"
import { formatEngineLabel, formatRelative } from "@/lib/format"
import { EYEBROW_LABEL } from "@/lib/styles"
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

type DeleteStep = 1 | 2

export function Workers() {
  const { t } = useTranslation()
  const navigate = useNavigate()
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

  return (
    <FadeIn className="h-full">
      <div className="flex h-full">
        {/* Left pane: department filter, flush to the layout edge with its own scroll. */}
        <aside className="flex w-60 shrink-0 flex-col border-r">
          <div className="flex h-16 items-center border-b px-4">
            <h2 className="text-sm font-semibold tracking-tight">{t("departments.filter")}</h2>
          </div>
          <div className="min-h-0 flex-1">
            <DepartmentTreeSidebar
              departments={departments}
              selectedId={selectedDeptId}
              onSelect={setSelectedDeptId}
            />
          </div>
        </aside>

        {/* Right pane: worker list, fills remaining width with its own scroll. */}
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex h-16 items-center justify-between gap-4 border-b px-6">
            <div>
              <h1 className="text-xl font-bold tracking-tight">{t("workers.title")}</h1>
              {displayedWorkers.length > 0 && (
                <p className="mt-0.5 text-sm text-muted-foreground" aria-live="polite">
                  {t("workers.summary", { count: displayedWorkers.length, active: activeWorkers })}
                </p>
              )}
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <Button onClick={() => navigate("/workers/create")}>
                {t("workers.createWorker")}
              </Button>
            </div>
          </div>

          <div className="min-w-0 flex-1 overflow-auto px-6 py-5">
      {error && (
        <div role="alert" className="mb-4 mx-auto w-full max-w-6xl rounded-sm border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {isLoading ? (
        <SkeletonTable rows={6} columns={4} />
      ) : displayedWorkers.length === 0 && !error ? (
        <EmptyState
          title={selectedDeptId !== null ? t("emptyState.noWorkersInGroup") : t("emptyState.noWorkers")}
          description={selectedDeptId !== null ? t("emptyState.noWorkersInGroupDesc") : t("emptyState.noWorkersDesc")}
          action={
            selectedDeptId === null ? (
              <Button onClick={() => navigate("/workers/create")}>{t("workers.createWorker")}</Button>
            ) : undefined
          }
        />
      ) : (
        <div className="mx-auto w-full max-w-6xl overflow-hidden rounded-sm bg-card ring-1 ring-foreground/10">
          <Table className="min-w-[680px]">
            <TableHeader>
              <TableRow className="bg-secondary/50 hover:bg-secondary/50">
                <TableHead>{t("workers.columns.name")}</TableHead>
                <TableHead className="w-[112px]">{t("workers.columns.engine")}</TableHead>
                <TableHead className="w-[124px]">{t("workers.columns.activeTime")}</TableHead>
                <TableHead className="w-16 text-right">{t("workers.columns.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {displayedWorkers.map((w) => (
                <TableRow key={w.id} className="hover:bg-muted/50 transition-colors">
                  <TableCell>
                    <div className="flex items-center gap-3 py-1">
                      <WorkerAvatar name={w.name} status={w.status} />
                      <div className="flex min-w-0 flex-col gap-0.5">
                        <Link
                          to={`/workers/${w.id}`}
                          className="font-medium text-foreground transition-colors hover:text-primary"
                        >
                          {w.name}
                        </Link>
                        <p className="max-w-[34rem] text-xs leading-5 text-muted-foreground line-clamp-2">
                          {w.description || "—"}
                        </p>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {w.engine ? (
                      <EngineIcon
                        engine={w.engine}
                        title={formatEngineLabel(w.engine, t)}
                        className="size-5 text-foreground/80"
                      />
                    ) : (
                      "—"
                    )}
                  </TableCell>
                  <TableCell className="text-sm font-mono text-muted-foreground">
                    <span title={w.updated_at ? new Date(w.updated_at).toLocaleString() : undefined}>
                      {formatRelative(w.updated_at, t)}
                    </span>
                  </TableCell>
                  <TableCell className="text-right">
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
                        <DropdownMenuItem onClick={() => navigate(`/workers/${w.id}`)}>
                          <EyeIcon className="size-4" />
                          {t("common.view")}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem onClick={() => navigate(`/workers/create?copy=${w.id}`)}>
                          <Copy className="size-4" />
                          {t("common.copy")}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          variant="destructive"
                          onClick={() => openDeleteDialog({ id: w.id, name: w.name })}
                        >
                          <Trash2Icon className="size-4" />
                          {t("common.delete")}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      </div>

      <Dialog open={!!deleteTarget} onOpenChange={(o) => { if (!o) resetDelete() }}>
        <DialogContent>
          <DialogHeader>
            <p className={EYEBROW_LABEL}>
              {deleteStep === 1 ? t("workers.deleteDialog.stepOne") : t("workers.deleteDialog.stepTwo")}
            </p>
            <DialogTitle>{t("workers.deleteDialog.title")}</DialogTitle>
            <DialogDescription>
              {deleteStep === 1 ? (
                <Trans
                  i18nKey="workers.deleteDialog.stepOneDescription"
                  values={{ name: deleteTarget?.name ?? "" }}
                  components={{ strong: <strong /> }}
                />
              ) : (
                <Trans
                  i18nKey="workers.deleteDialog.stepTwoDescription"
                  values={{ name: deleteTarget?.name ?? "" }}
                  components={{ strong: <strong /> }}
                />
              )}
            </DialogDescription>
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
                  className="size-4 cursor-pointer rounded accent-primary"
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
        </div>
      </div>
    </FadeIn>
  )
}
