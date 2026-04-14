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
import { cn } from "@/lib/utils"
import type { EnvConfig } from "@/lib/types"

interface AddEnvDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  scope: "global" | "department" | "worker"
  scopeId?: string
  existingKeys: string[]
}

function AddEnvDialog({ open, onOpenChange, scope, scopeId, existingKeys }: AddEnvDialogProps) {
  const { t } = useTranslation()
  const createEnv = useCreateEnv()

  const [formKey, setFormKey] = useState("")
  const [formValue, setFormValue] = useState("")
  const [showValue, setShowValue] = useState(false)
  const [apiError, setApiError] = useState("")

  const handleKeyChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const normalized = e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, "")
    setFormKey(normalized)
    setApiError("")
  }

  const keyDuplicate = formKey.length > 0 && existingKeys.includes(formKey)
  const hasLeadingTrailingSpace = formValue.length > 0 && formValue !== formValue.trim()
  const canSubmit = formKey.trim().length > 0 && formValue.length > 0 && !keyDuplicate && !createEnv.isPending

  const resetForm = () => {
    setFormKey("")
    setFormValue("")
    setShowValue(false)
    setApiError("")
  }

  const handleClose = () => {
    resetForm()
    onOpenChange(false)
  }

  const submit = async (addAnother: boolean) => {
    if (!canSubmit) return
    setApiError("")
    try {
      await createEnv.mutateAsync({
        scope,
        scope_id: scopeId,
        key: formKey.trim(),
        value: formValue,
      })
      if (addAnother) {
        resetForm()
      } else {
        handleClose()
      }
    } catch (err) {
      setApiError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <Dialog open={open} onOpenChange={(isOpen) => { if (!isOpen) handleClose() }}>
      <DialogContent className="max-w-md" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{t("envConfig.addTitle")}</DialogTitle>
          <DialogDescription>{t("envConfig.addDescription")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4 py-1">
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="add-env-key">{t("envConfig.key")}</Label>
              <span className="text-[11px] font-mono text-muted-foreground/60 tracking-wide select-none">
                A–Z · 0–9 · _
              </span>
            </div>
            <Input
              id="add-env-key"
              value={formKey}
              onChange={handleKeyChange}
              placeholder={t("envConfig.keyPlaceholder")}
              required
              autoFocus
              autoComplete="off"
              spellCheck={false}
              className={cn("font-mono", keyDuplicate && "border-destructive focus-visible:border-destructive focus-visible:ring-destructive/20")}
            />
            {keyDuplicate && (
              <p className="text-xs text-destructive">{t("envConfig.keyExists")}</p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="add-env-value">{t("envConfig.value")}</Label>
            <div className="relative">
              <Input
                id="add-env-value"
                type={showValue ? "text" : "password"}
                value={formValue}
                onChange={(e) => { setFormValue(e.target.value); setApiError("") }}
                placeholder={t("envConfig.valuePlaceholder")}
                required
                className="font-mono pr-10"
              />
              <button
                type="button"
                onClick={() => setShowValue(!showValue)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                tabIndex={-1}
                title={showValue ? t("envConfig.hideValue") : t("envConfig.showValue")}
              >
                {showValue ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
              </button>
            </div>
            <div className="flex items-center justify-between min-h-[1rem]">
              {hasLeadingTrailingSpace ? (
                <p className="text-xs text-amber-500 dark:text-amber-400">{t("envConfig.valueSpaceWarning")}</p>
              ) : (
                <span />
              )}
              {formValue.length > 0 && (
                <span className="text-[11px] tabular-nums text-muted-foreground/60">{formValue.length}</span>
              )}
            </div>
          </div>

          {apiError && (
            <p className="text-xs text-destructive">{apiError}</p>
          )}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={handleClose} className="mr-auto sm:mr-auto">
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={!canSubmit}
            onClick={() => submit(true)}
          >
            {t("envConfig.saveAndAddAnother")}
          </Button>
          <Button
            type="button"
            disabled={!canSubmit}
            onClick={() => submit(false)}
          >
            {t("common.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface EnvConfigPanelProps {
  scope: "global" | "department" | "worker"
  scopeId?: string
}

export function EnvConfigPanel({ scope, scopeId }: EnvConfigPanelProps) {
  const { t } = useTranslation()
  const { data: envs = [], isLoading } = useEnvList(scope, scopeId)
  const updateEnv = useUpdateEnv(scope, scopeId)
  const deleteEnv = useDeleteEnv(scope, scopeId)

  const [addDialogOpen, setAddDialogOpen] = useState(false)

  // undefined = closed, EnvConfig = edit mode
  const [editTarget, setEditTarget] = useState<EnvConfig | undefined>(undefined)
  const [formValue, setFormValue] = useState("")
  const [showValue, setShowValue] = useState(false)
  const [formError, setFormError] = useState("")

  const [deleteTarget, setDeleteTarget] = useState<EnvConfig | null>(null)

  const openEditSheet = (target: EnvConfig) => {
    setFormValue("")
    setShowValue(false)
    setFormError("")
    setEditTarget(target)
  }

  const closeEditSheet = () => {
    setEditTarget(undefined)
    setFormValue("")
    setShowValue(false)
    setFormError("")
  }

  const handleEditSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setFormError("")
    try {
      await updateEnv.mutateAsync({ id: editTarget!.id, value: formValue })
      closeEditSheet()
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

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-end">
        <Button size="sm" onClick={() => setAddDialogOpen(true)}>
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
                        onClick={() => openEditSheet(env)}
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

      <AddEnvDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        scope={scope}
        scopeId={scopeId}
        existingKeys={envs.map((e) => e.key)}
      />

      <Sheet open={editTarget !== undefined} onOpenChange={(open) => { if (!open) closeEditSheet() }}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("envConfig.editTitle")}</SheetTitle>
            <SheetDescription className="font-mono">{editTarget?.key}</SheetDescription>
          </SheetHeader>
          <form onSubmit={handleEditSubmit} className="flex flex-col gap-5 px-4 py-6">
            <div className="space-y-1.5">
              <Label htmlFor="edit-env-value">{t("envConfig.value")}</Label>
              <div className="relative">
                <Input
                  id="edit-env-value"
                  type={showValue ? "text" : "password"}
                  value={formValue}
                  onChange={(e) => { setFormValue(e.target.value); setFormError("") }}
                  placeholder={t("envConfig.valuePlaceholder")}
                  required
                  autoFocus
                  className="font-mono pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowValue(!showValue)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                  tabIndex={-1}
                  title={showValue ? t("envConfig.hideValue") : t("envConfig.showValue")}
                >
                  {showValue ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
                </button>
              </div>
              <div className="flex items-center justify-between min-h-[1rem]">
                {formValue.length > 0 && formValue !== formValue.trim() ? (
                  <p className="text-xs text-amber-500 dark:text-amber-400">{t("envConfig.valueSpaceWarning")}</p>
                ) : formError ? (
                  <p className="text-xs text-destructive">{formError}</p>
                ) : (
                  <span />
                )}
                {formValue.length > 0 && (
                  <span className="text-[11px] tabular-nums text-muted-foreground/60">{formValue.length}</span>
                )}
              </div>
            </div>
            <SheetFooter>
              <Button type="button" variant="outline" onClick={closeEditSheet}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={updateEnv.isPending || !formValue}>
                {t("common.save")}
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
