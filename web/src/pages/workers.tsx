import { useState, useMemo, type FormEvent } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useTranslation, Trans } from "react-i18next"
import { EyeIcon, MoreHorizontalIcon, Trash2Icon } from "lucide-react"
import { useWorkers, useCreateWorker, useDeleteWorker } from "@/hooks/use-workers"
import { useDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { flattenDeptTree } from "@/lib/department-utils"
import { DepartmentTreeSidebar, UNGROUPED_FILTER } from "@/components/department-tree"
import { DepartmentManageDialog } from "@/components/department-dialog"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from "@/components/ui/dialog"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from "@/components/ui/sheet"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Separator } from "@/components/ui/separator"
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

type DeleteStep = 1 | 2

export function Workers() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [selectedDeptId, setSelectedDeptId] = useState<string | null>(null)
  const [manageDeptOpen, setManageDeptOpen] = useState(false)
  const { data: departments = [] } = useDepartments()
  const deptFilter = selectedDeptId === UNGROUPED_FILTER ? undefined : (selectedDeptId ?? undefined)
  const { data: workers = [], error: fetchError, isLoading } = useWorkers(deptFilter)
  const displayedWorkers = selectedDeptId === UNGROUPED_FILTER
    ? workers.filter((w) => !w.departments || w.departments.length === 0)
    : workers
  const createWorker = useCreateWorker()
  const deleteWorker = useDeleteWorker()
  const setWorkerDepts = useSetWorkerDepartments()
  const [open, setOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; name: string } | null>(null)
  const [deleteStep, setDeleteStep] = useState<DeleteStep>(1)
  const [deleteWorkDir, setDeleteWorkDir] = useState(false)
  const [deleteConfirmationText, setDeleteConfirmationText] = useState("")
  const [selectedCreateDeptIds, setSelectedCreateDeptIds] = useState<Set<string>>(new Set())
  const flatDepts = useMemo(() => flattenDeptTree(departments), [departments])

  const resetDelete = () => {
    setDeleteTarget(null)
    setDeleteStep(1)
    setDeleteWorkDir(false)
    setDeleteConfirmationText("")
  }
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [memory, setMemory] = useState("")
  const [workDir, setWorkDir] = useState("")

  const error = fetchError?.message || createWorker.error?.message || deleteWorker.error?.message || setWorkerDepts.error?.message || ""
  const activeWorkers = displayedWorkers.filter((worker) => worker.status === "working").length
  const isDeleteNameConfirmed = deleteConfirmationText === (deleteTarget?.name ?? "")

  const handleCreate = async (e?: FormEvent) => {
    e?.preventDefault()
    const worker = await createWorker.mutateAsync({
      name,
      description,
      memory: memory || undefined,
      work_dir: workDir || undefined,
    })
    if (selectedCreateDeptIds.size > 0) {
      await setWorkerDepts.mutateAsync({ workerId: worker.id, departmentIds: [...selectedCreateDeptIds] })
    }
    setOpen(false)
    setName("")
    setDescription("")
    setMemory("")
    setWorkDir("")
    setSelectedCreateDeptIds(new Set())
  }

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
            onManage={() => setManageDeptOpen(true)}
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
            <Sheet open={open} onOpenChange={setOpen}>
              <SheetContent className="w-full sm:max-w-[26rem] p-0 gap-0">
                <SheetHeader className="px-6 pt-6 pb-4">
                  <SheetTitle>{t("workers.createWorker")}</SheetTitle>
                  <SheetDescription>{t("workers.form.panelDescription")}</SheetDescription>
                </SheetHeader>
                <Separator />
                <form
                  id="create-worker-form"
                  onSubmit={handleCreate}
                  className="flex-1 overflow-y-auto px-6 py-5 space-y-6"
                >
                  <div className="space-y-4">
                    <p className="text-xs font-medium uppercase tracking-[0.1em] text-muted-foreground">
                      {t("workers.form.sectionBasic")}
                    </p>
                    <div className="space-y-1.5">
                      <Label htmlFor="name">
                        {t("workers.form.name")}
                        <span className="ml-1 text-destructive" aria-hidden>*</span>
                      </Label>
                      <Input
                        id="name"
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        placeholder={t("workers.form.namePlaceholder")}
                        required
                        autoFocus
                      />
                      <p className="text-xs text-muted-foreground">{t("workers.form.nameHelper")}</p>
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="desc">{t("workers.form.description")}</Label>
                      <Textarea
                        id="desc"
                        value={description}
                        onChange={(e) => setDescription(e.target.value)}
                        placeholder={t("workers.form.descriptionPlaceholder")}
                        rows={2}
                      />
                      <p className="text-xs text-muted-foreground">{t("workers.form.descriptionHelper")}</p>
                    </div>
                  </div>

                  <Separator />

                  <div className="space-y-4">
                    <p className="text-xs font-medium uppercase tracking-[0.1em] text-muted-foreground">
                      {t("workers.form.sectionConfig")}
                    </p>
                    <div className="space-y-1.5">
                      <Label htmlFor="workdir">{t("workers.form.workDir")}</Label>
                      <Input
                        id="workdir"
                        value={workDir}
                        onChange={(e) => setWorkDir(e.target.value)}
                        placeholder={t("workers.form.workDirPlaceholder")}
                        className="font-mono text-xs"
                      />
                      <p className="text-xs text-muted-foreground">{t("workers.form.workDirHelper")}</p>
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="memory">{t("workers.form.memory")}</Label>
                      <Textarea
                        id="memory"
                        value={memory}
                        onChange={(e) => setMemory(e.target.value)}
                        placeholder={t("workers.form.memoryPlaceholder")}
                        rows={5}
                      />
                      <p className="text-xs text-muted-foreground">{t("workers.form.memoryHelper")}</p>
                    </div>
                  </div>

                  {flatDepts.length > 0 && (
                    <>
                      <Separator />
                      <div className="space-y-4">
                        <p className="text-xs font-medium uppercase tracking-[0.1em] text-muted-foreground">
                          {t("workers.form.sectionDepartment")}
                        </p>
                        <div className="space-y-2 max-h-40 overflow-y-auto">
                          {flatDepts.map(({ dept, depth }) => (
                            <div
                              key={dept.id}
                              className="flex items-center gap-2"
                              style={{ paddingLeft: `${depth * 12}px` }}
                            >
                              <input
                                type="checkbox"
                                id={`create-dept-${dept.id}`}
                                checked={selectedCreateDeptIds.has(dept.id)}
                                onChange={(e) => {
                                  const next = new Set(selectedCreateDeptIds)
                                  if (e.target.checked) next.add(dept.id)
                                  else next.delete(dept.id)
                                  setSelectedCreateDeptIds(next)
                                }}
                                className="size-4 cursor-pointer rounded accent-primary"
                              />
                              <Label htmlFor={`create-dept-${dept.id}`} className="cursor-pointer text-sm font-normal">
                                {dept.name}
                              </Label>
                            </div>
                          ))}
                        </div>
                        <p className="text-xs text-muted-foreground">{t("workers.form.departmentHelper")}</p>
                      </div>
                    </>
                  )}
                </form>
                <Separator />
                <SheetFooter className="px-6 py-4 flex-row gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    className="flex-1"
                    onClick={() => setOpen(false)}
                  >
                    {t("common.cancel")}
                  </Button>
                  <Button
                    type="submit"
                    form="create-worker-form"
                    disabled={createWorker.isPending || setWorkerDepts.isPending || !name.trim()}
                    className="flex-1"
                  >
                    {t("workers.createWorker")}
                  </Button>
                </SheetFooter>
              </SheetContent>
            </Sheet>
          </>
        }
      />

      {error && <p className="text-destructive mb-4">{error}</p>}

      <div className="min-h-[320px]">
      {isLoading ? (
        <SkeletonTable rows={6} columns={4} />
      ) : displayedWorkers.length === 0 && !error ? (
        <EmptyState
          title={t("emptyState.noWorkers")}
          description={t("emptyState.noWorkersDesc")}
          action={
            <Button onClick={() => setOpen(true)}>{t("workers.createWorker")}</Button>
          }
        />
      ) : (
        <div className="rounded-xl bg-card ring-1 ring-foreground/5 overflow-hidden">
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

      <DepartmentManageDialog open={manageDeptOpen} onOpenChange={setManageDeptOpen} />
    </FadeIn>
  )
}
