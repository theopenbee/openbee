import { useState, type FormEvent } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useTranslation, Trans } from "react-i18next"
import { CheckIcon, CopyIcon, EyeIcon, MoreHorizontalIcon, Trash2Icon } from "lucide-react"
import { useWorkers, useCreateWorker, useDeleteWorker } from "@/hooks/use-workers"
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
  const { data: workers = [], error: fetchError, isLoading } = useWorkers()
  const createWorker = useCreateWorker()
  const deleteWorker = useDeleteWorker()
  const [open, setOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; name: string } | null>(null)
  const [deleteStep, setDeleteStep] = useState<DeleteStep>(1)
  const [deleteWorkDir, setDeleteWorkDir] = useState(false)
  const [deleteConfirmationText, setDeleteConfirmationText] = useState("")
  const [copiedWorkerId, setCopiedWorkerId] = useState<string | null>(null)

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

  const error = fetchError?.message || createWorker.error?.message || deleteWorker.error?.message || ""
  const activeWorkers = workers.filter((worker) => worker.status === "working").length
  const isDeleteNameConfirmed = deleteConfirmationText === (deleteTarget?.name ?? "")

  const handleCreate = async (e?: FormEvent) => {
    e?.preventDefault()
    await createWorker.mutateAsync({
      name,
      description,
      memory: memory || undefined,
      work_dir: workDir || undefined,
    })
    setOpen(false)
    setName("")
    setDescription("")
    setMemory("")
    setWorkDir("")
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

  const handleCopyWorkDir = async (workerId: string, dir: string) => {
    try {
      await navigator.clipboard.writeText(dir)
      setCopiedWorkerId(workerId)
      window.setTimeout(() => {
        setCopiedWorkerId((current) => current === workerId ? null : current)
      }, 1500)
    } catch {
      setCopiedWorkerId(null)
    }
  }

  return (
    <FadeIn>
      <PageHeader
        title={t("workers.title")}
        subtitle={
          workers.length > 0
            ? t("workers.summary", { count: workers.length, active: activeWorkers })
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
                  {/* Basic info */}
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

                  {/* Configuration */}
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
                    disabled={createWorker.isPending || !name.trim()}
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

      {isLoading ? (
        <SkeletonTable rows={6} columns={5} />
      ) : workers.length === 0 && !error ? (
        <EmptyState
          title={t("emptyState.noWorkers")}
          description={t("emptyState.noWorkersDesc")}
          action={
            <Button onClick={() => setOpen(true)}>{t("workers.createWorker")}</Button>
          }
        />
      ) : (
        <div className="rounded-xl bg-card ring-1 ring-foreground/5 overflow-hidden">
          <Table className="min-w-[760px]">
            <TableHeader>
              <TableRow className="bg-secondary/50 hover:bg-secondary/50">
                <TableHead>{t("workers.columns.name")}</TableHead>
                <TableHead>{t("workers.columns.workDir")}</TableHead>
                <TableHead>{t("workers.columns.status")}</TableHead>
                <TableHead>{t("workers.columns.activeTime")}</TableHead>
                <TableHead className="text-right">{t("workers.columns.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {workers.map((w) => (
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
                  <TableCell className="max-w-[20rem]">
                    {w.work_dir ? (
                      <div className="flex items-center gap-2">
                        <span className="block max-w-[16rem] truncate font-mono text-xs text-muted-foreground">
                          {w.work_dir}
                        </span>
                        <Button
                          variant={copiedWorkerId === w.id ? "secondary" : "ghost"}
                          size="icon-xs"
                          className="shrink-0"
                          aria-label={copiedWorkerId === w.id ? t("common.copied") : t("common.copy")}
                          title={copiedWorkerId === w.id ? t("common.copied") : t("common.copy")}
                          onClick={() => handleCopyWorkDir(w.id, w.work_dir)}
                        >
                          {copiedWorkerId === w.id ? (
                            <CheckIcon className="size-3.5" />
                          ) : (
                            <CopyIcon className="size-3.5" />
                          )}
                        </Button>
                      </div>
                    ) : (
                      <span className="text-sm text-muted-foreground">
                        {t("workers.defaultWorkDir")}
                      </span>
                    )}
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
    </FadeIn>
  )
}
