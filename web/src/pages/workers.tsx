import { useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation, Trans } from "react-i18next"
import { useWorkers, useCreateWorker, useDeleteWorker } from "@/hooks/use-workers"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DialogFooter,
  DialogDescription,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { StatusBadge } from "@/components/status-badge"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonCard } from "@/components/skeleton-loader"

export function Workers() {
  const { t } = useTranslation()
  const { data: workers = [], error: fetchError, isLoading } = useWorkers()
  const createWorker = useCreateWorker()
  const deleteWorker = useDeleteWorker()
  const [open, setOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; name: string } | null>(null)
  const [deleteWorkDir, setDeleteWorkDir] = useState(false)

  const resetDelete = () => { setDeleteTarget(null); setDeleteWorkDir(false) }
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [memory, setMemory] = useState("")
  const [workDir, setWorkDir] = useState("")

  const error = fetchError?.message || createWorker.error?.message || deleteWorker.error?.message || ""

  const handleCreate = async () => {
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
    if (!deleteTarget) return
    await deleteWorker.mutateAsync({ id: deleteTarget.id, deleteWorkDir })
    resetDelete()
  }

  return (
    <FadeIn>
      <PageHeader
        title={t("workers.title")}
        actions={
          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger render={<Button />}>
              {t("workers.createWorker")}
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>{t("workers.createWorker")}</DialogTitle>
              </DialogHeader>
              <div className="space-y-4 max-h-[70vh] overflow-y-auto">
                <div>
                  <Label htmlFor="name">{t("workers.form.name")}</Label>
                  <Input
                    id="name"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder={t("workers.form.namePlaceholder")}
                  />
                </div>
                <div>
                  <Label htmlFor="desc">{t("workers.form.description")}</Label>
                  <Textarea
                    id="desc"
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    placeholder={t("workers.form.descriptionPlaceholder")}
                  />
                </div>
                <div>
                  <Label htmlFor="workdir">{t("workers.form.workDir")}</Label>
                  <Input
                    id="workdir"
                    value={workDir}
                    onChange={(e) => setWorkDir(e.target.value)}
                    placeholder={t("workers.form.workDirPlaceholder")}
                    className="font-mono"
                  />
                </div>
                <div>
                  <Label htmlFor="memory">{t("workers.form.memory")}</Label>
                  <Textarea
                    id="memory"
                    value={memory}
                    onChange={(e) => setMemory(e.target.value)}
                    placeholder={t("workers.form.memoryPlaceholder")}
                    rows={4}
                  />
                </div>

                <Button onClick={handleCreate} className="w-full">
                  {t("workers.createWorker")}
                </Button>
              </div>
            </DialogContent>
          </Dialog>
        }
      />

      {error && <p className="text-destructive mb-4">{error}</p>}

      {isLoading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5">
          {Array.from({ length: 4 }).map((_, i) => (
            <SkeletonCard key={i} />
          ))}
        </div>
      ) : workers.length === 0 && !error ? (
        <EmptyState
          title={t("emptyState.noWorkers")}
          description={t("emptyState.noWorkersDesc")}
          action={
            <Button onClick={() => setOpen(true)}>{t("workers.createWorker")}</Button>
          }
        />
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5">
          {workers.map((w) => (
            <Card key={w.id} className="hover:ring-1 hover:ring-primary/30 transition-all duration-200">
              <CardHeader className="pb-2">
                <div className="flex items-center justify-between">
                  <Link to={`/workers/${w.id}`}>
                    <CardTitle className="text-base font-semibold hover:text-primary transition-colors">
                      {w.name}
                    </CardTitle>
                  </Link>
                  <StatusBadge status={w.status} />
                </div>
              </CardHeader>
              <CardContent>
                <p className="text-sm text-muted-foreground mb-2 line-clamp-2">
                  {w.description || t("common.noDescription")}
                </p>
                <div className="flex gap-2 pt-2 border-t border-border">
                  <Link to={`/workers/${w.id}`}>
                    <Button variant="outline" size="sm">
                      {t("common.view")}
                    </Button>
                  </Link>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-destructive hover:text-destructive hover:bg-destructive/10"
                    onClick={() => setDeleteTarget({ id: w.id, name: w.name })}
                  >
                    {t("common.delete")}
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Dialog open={!!deleteTarget} onOpenChange={(o) => { if (!o) resetDelete() }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("workers.deleteDialog.title")}</DialogTitle>
            <DialogDescription>
              <Trans
                i18nKey="workers.deleteDialog.confirm"
                values={{ name: deleteTarget?.name ?? "" }}
                components={{ strong: <strong /> }}
              />
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 py-2">
            <input
              type="checkbox"
              id="delete-work-dir"
              checked={deleteWorkDir}
              onChange={(e) => setDeleteWorkDir(e.target.checked)}
              className="accent-primary"
            />
            <Label htmlFor="delete-work-dir">{t("workers.deleteDialog.deleteWorkDir")}</Label>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={resetDelete}>
              {t("common.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleDeleteConfirm} disabled={deleteWorker.isPending}>
              {t("common.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </FadeIn>
  )
}
