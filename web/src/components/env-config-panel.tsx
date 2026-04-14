import { useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { PlusIcon, Trash2Icon, PencilIcon, EyeIcon, EyeOffIcon } from "lucide-react"
import { useEnvList, useCreateEnv, useUpdateEnv, useDeleteEnv } from "@/hooks/use-envs"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from "@/components/ui/sheet"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { EmptyState } from "@/components/empty-state"
import type { EnvConfig } from "@/lib/types"

interface EnvConfigPanelProps {
  scope: "global" | "department" | "worker"
  scopeId?: string
}

export function EnvConfigPanel({ scope, scopeId }: EnvConfigPanelProps) {
  const { t } = useTranslation()
  const { data: envs = [], isLoading } = useEnvList(scope, scopeId)
  const createEnv = useCreateEnv()
  const updateEnv = useUpdateEnv(scope, scopeId)
  const deleteEnv = useDeleteEnv(scope, scopeId)

  const [sheetOpen, setSheetOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<EnvConfig | null>(null)
  const [formKey, setFormKey] = useState("")
  const [formValue, setFormValue] = useState("")
  const [showValue, setShowValue] = useState(false)
  const [formError, setFormError] = useState("")

  const [deleteTarget, setDeleteTarget] = useState<EnvConfig | null>(null)

  const openSheet = (target: EnvConfig | null) => {
    setEditTarget(target)
    setFormKey(target?.key ?? "")
    setFormValue("")
    setShowValue(false)
    setFormError("")
    setSheetOpen(true)
  }

  const closeSheet = () => {
    setSheetOpen(false)
    setEditTarget(null)
    setFormKey("")
    setFormValue("")
    setShowValue(false)
    setFormError("")
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setFormError("")
    try {
      if (editTarget) {
        await updateEnv.mutateAsync({ id: editTarget.id, value: formValue })
      } else {
        await createEnv.mutateAsync({
          scope,
          scope_id: scopeId,
          key: formKey.trim(),
          value: formValue,
        })
      }
      closeSheet()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : String(err))
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      await deleteEnv.mutateAsync(deleteTarget.id)
    } finally {
      setDeleteTarget(null)
    }
  }

  const isPending = createEnv.isPending || updateEnv.isPending

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-end">
        <Button size="sm" onClick={() => openSheet(null)}>
          <PlusIcon className="size-3.5" />
          {t("envConfig.add")}
        </Button>
      </div>

      {isLoading ? null : envs.length === 0 ? (
        <EmptyState title={t("envConfig.empty")} />
      ) : (
        <div className="rounded-2xl border border-border/70 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("envConfig.key")}</TableHead>
                <TableHead>{t("envConfig.masked")}</TableHead>
                <TableHead className="text-right">{t("workers.columns.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {envs.map((env) => (
                <TableRow key={env.id}>
                  <TableCell className="font-mono text-sm">{env.key}</TableCell>
                  <TableCell className="font-mono text-sm text-muted-foreground">{env.masked}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => openSheet(env)}
                        title={t("common.edit")}
                      >
                        <PencilIcon className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => setDeleteTarget(env)}
                        title={t("common.delete")}
                      >
                        <Trash2Icon className="size-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Sheet open={sheetOpen} onOpenChange={(open) => { if (!open) closeSheet() }}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>
              {editTarget ? t("envConfig.editTitle") : t("envConfig.addTitle")}
            </SheetTitle>
            <SheetDescription>
              {editTarget ? editTarget.key : t("envConfig.keyPlaceholder")}
            </SheetDescription>
          </SheetHeader>
          <form onSubmit={handleSubmit} className="flex flex-col gap-5 px-4 py-6">
            {formError && (
              <div role="alert" className="rounded-2xl border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                {formError}
              </div>
            )}
            {!editTarget && (
              <div className="space-y-1.5">
                <Label htmlFor="env-key">{t("envConfig.key")}</Label>
                <Input
                  id="env-key"
                  value={formKey}
                  onChange={(e) => setFormKey(e.target.value)}
                  placeholder={t("envConfig.keyPlaceholder")}
                  required
                  autoFocus
                  className="font-mono"
                />
              </div>
            )}
            <div className="space-y-1.5">
              <Label htmlFor="env-value">{t("envConfig.value")}</Label>
              <div className="relative">
                <Input
                  id="env-value"
                  type={showValue ? "text" : "password"}
                  value={formValue}
                  onChange={(e) => setFormValue(e.target.value)}
                  placeholder={t("envConfig.valuePlaceholder")}
                  required
                  autoFocus={!!editTarget}
                  className="font-mono pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowValue(!showValue)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                  tabIndex={-1}
                >
                  {showValue ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
                </button>
              </div>
            </div>
            <SheetFooter>
              <Button type="button" variant="outline" onClick={closeSheet}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={isPending || !formValue || (!editTarget && !formKey.trim())}>
                {editTarget ? t("common.save") : t("common.create")}
              </Button>
            </SheetFooter>
          </form>
        </SheetContent>
      </Sheet>

      <Dialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("common.delete")}</DialogTitle>
            <DialogDescription>
              {t("envConfig.deleteConfirm", { key: deleteTarget?.key })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleteEnv.isPending}>
              {t("common.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
