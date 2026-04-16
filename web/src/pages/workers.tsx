import { useState, useMemo, type FormEvent } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useTranslation, Trans } from "react-i18next"
import { Copy, EyeIcon, MoreHorizontalIcon, Trash2Icon } from "lucide-react"
import { useWorkers, useDeleteWorker } from "@/hooks/use-workers"
import { useDepartments } from "@/hooks/use-departments"
import { DepartmentTreeSidebar, UNGROUPED_FILTER } from "@/components/department-tree"
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
import { StatusBadge } from "@/components/status-badge"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonTable } from "@/components/skeleton-loader"
import { CreateWorkerSheet } from "@/components/create-worker-sheet"
import type { Worker } from "@/lib/types"

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
  const [open, setOpen] = useState(false)
  const [copySource, setCopySource] = useState<Worker | null>(null)
  const handleSheetOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setOpen(false)
      setCopySource(null)
    }
  }
  const copyInitialValues = useMemo(
    () =>
      copySource
        ? {
            name: copySource.name,
            description: copySource.description,
            memory: copySource.memory,
            work_dir: copySource.work_dir,
            permission_scopes: copySource.permission_scopes ?? "",
            departmentIds: copySource.departments?.map((d) => d.id) ?? [],
          }
        : undefined,
    [copySource],
  )
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
    <FadeIn>
      <div className="flex gap-6 h-full">
        <div className="w-56 shrink-0 border-r pr-4">
          <DepartmentTreeSidebar
            departments={departments}
            selectedId={selectedDeptId}
            onSelect={setSelectedDeptId}
          />
        </div>

        <div className="flex-1 min-w-0">
      <PageHeader
        title={t("workers.title")}
        subtitle={
          displayedWorkers.length > 0
            ? t("workers.summary", { count: displayedWorkers.length, active: activeWorkers })
            : undefined
        }
        actions={
          <>
            <Button onClick={() => setOpen(true)}>
              {t("workers.createWorker")}
            </Button>
            <CreateWorkerSheet
              open={open || copySource !== null}
              onOpenChange={handleSheetOpenChange}
              initialValues={copyInitialValues}
            />
          </>
        }
      />

      {error && (
        <div role="alert" className="mb-4 rounded-2xl border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      <div className="min-h-[320px]">
      {isLoading ? (
        <SkeletonTable rows={6} columns={4} />
      ) : displayedWorkers.length === 0 && !error ? (
        <EmptyState
          title={selectedDeptId !== null ? t("emptyState.noWorkersInGroup") : t("emptyState.noWorkers")}
          description={selectedDeptId !== null ? t("emptyState.noWorkersInGroupDesc") : t("emptyState.noWorkersDesc")}
          action={
            selectedDeptId === null ? (
              <Button onClick={() => setOpen(true)}>{t("workers.createWorker")}</Button>
            ) : undefined
          }
        />
      ) : (
        <div className="rounded-2xl border border-border/70 bg-card overflow-hidden">
          <Table className="min-w-[600px]">
            <TableHeader>
              <TableRow className="bg-secondary/50 hover:bg-secondary/50">
                <TableHead>{t("workers.columns.name")}</TableHead>
                <TableHead>{t("workers.columns.status")}</TableHead>
                <TableHead>{t("workers.columns.activeTime")}</TableHead>
                <TableHead className="text-right">{t("workers.columns.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {displayedWorkers.map((w) => (
                <TableRow key={w.id} className="hover:bg-primary/5 transition-colors">
                  <TableCell className="min-w-[19rem]">
                    <div className="flex flex-col gap-1.5 py-1">
                      <Link
                        to={`/workers/${w.id}`}
                        className="font-medium text-foreground transition-colors hover:text-primary"
                      >
                        {w.name}
                      </Link>
                      <p className="max-w-[26rem] text-xs leading-5 text-muted-foreground line-clamp-2">
                        {w.description || "-"}
                      </p>
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={w.status} />
                  </TableCell>
                  <TableCell className="text-sm font-mono text-muted-foreground">
                    {w.updated_at ? new Date(w.updated_at).toLocaleString() : "-"}
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
                        <DropdownMenuItem onClick={() => setCopySource(w)}>
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
            <p className="text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
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
