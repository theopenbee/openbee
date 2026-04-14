import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { PlusIcon, Trash2Icon, PencilIcon, EyeIcon, EyeOffIcon } from "lucide-react"
import { useEnvList, useCreateEnv, useUpdateEnv, useDeleteEnv } from "@/hooks/use-envs"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
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
import { cn, getErrorMessage } from "@/lib/utils"
import type { EnvConfig } from "@/lib/types"

function ValueHints({ value }: { value: string }) {
  const { t } = useTranslation()
  const hasLeadingTrailingSpace = value.length > 0 && value !== value.trim()
  return (
    <div className="flex items-center justify-between min-h-[1rem]">
      {hasLeadingTrailingSpace ? (
        <p className="text-xs text-amber-500 dark:text-amber-400">{t("envConfig.valueSpaceWarning")}</p>
      ) : (
        <span />
      )}
      {value.length > 0 && (
        <span className="text-[11px] tabular-nums text-muted-foreground/60">{value.length}</span>
      )}
    </div>
  )
}

function SecretInput({ id, value, onChange, autoFocus, placeholder }: {
  id: string
  value: string
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  autoFocus?: boolean
  placeholder?: string
}) {
  const [show, setShow] = useState(false)
  const { t } = useTranslation()
  return (
    <div className="relative">
      <Input
        id={id}
        type={show ? "text" : "password"}
        value={value}
        onChange={onChange}
        placeholder={placeholder}
        required
        autoFocus={autoFocus}
        className="font-mono pr-10"
      />
      <button
        type="button"
        onClick={() => setShow(!show)}
        className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
        tabIndex={-1}
        title={show ? t("envConfig.hideValue") : t("envConfig.showValue")}
      >
        {show ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
      </button>
    </div>
  )
}

interface AddEnvDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  scope: "global" | "bee" | "department" | "worker"
  scopeId?: string
  existingKeys: string[]
}

function AddEnvDialog({ open, onOpenChange, scope, scopeId, existingKeys }: AddEnvDialogProps) {
  const { t } = useTranslation()
  const createEnv = useCreateEnv()

  const [formKey, setFormKey] = useState("")
  const [formValue, setFormValue] = useState("")
  const [apiError, setApiError] = useState("")

  const handleKeyChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const normalized = e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, "")
    setFormKey(normalized)
    setApiError("")
  }

  const keyDuplicate = formKey.length > 0 && existingKeys.includes(formKey)
  const canSubmit = formKey.trim().length > 0 && formValue.length > 0 && !keyDuplicate && !createEnv.isPending

  const resetForm = () => {
    setFormKey("")
    setFormValue("")
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
      await createEnv.mutateAsync({ scope, scope_id: scopeId, key: formKey.trim(), value: formValue })
      if (addAnother) {
        resetForm()
      } else {
        handleClose()
      }
    } catch (err) {
      setApiError(getErrorMessage(err))
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
            <SecretInput
              id="add-env-value"
              value={formValue}
              onChange={(e) => { setFormValue(e.target.value); setApiError("") }}
              placeholder={t("envConfig.valuePlaceholder")}
            />
            <ValueHints value={formValue} />
          </div>

          {apiError && (
            <p className="text-xs text-destructive">{apiError}</p>
          )}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={handleClose} className="mr-auto sm:mr-auto">
            {t("common.cancel")}
          </Button>
          <Button type="button" variant="outline" disabled={!canSubmit} onClick={() => submit(true)}>
            {t("envConfig.saveAndAddAnother")}
          </Button>
          <Button type="button" disabled={!canSubmit} onClick={() => submit(false)}>
            {t("common.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface EditEnvDialogProps {
  target: EnvConfig | null
  onClose: () => void
  scope: "global" | "bee" | "department" | "worker"
  scopeId?: string
}

function EditEnvDialog({ target, onClose, scope, scopeId }: EditEnvDialogProps) {
  const { t } = useTranslation()
  const updateEnv = useUpdateEnv(scope, scopeId)

  const [formValue, setFormValue] = useState("")
  const [apiError, setApiError] = useState("")

  const handleClose = () => {
    setFormValue("")
    setApiError("")
    onClose()
  }

  const handleSubmit = async () => {
    if (!target || !formValue) return
    setApiError("")
    try {
      await updateEnv.mutateAsync({ id: target.id, value: formValue })
      handleClose()
    } catch (err) {
      setApiError(getErrorMessage(err))
    }
  }

  const canSubmit = formValue.length > 0 && !updateEnv.isPending

  return (
    <Dialog
      open={target !== null}
      onOpenChange={(isOpen) => { if (!isOpen) handleClose() }}
    >
      <DialogContent className="max-w-md" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{t("envConfig.editTitle")}</DialogTitle>
          <DialogDescription className="font-mono">{target?.key}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4 py-1">
          <div className="space-y-1.5">
            <Label htmlFor="edit-env-value">{t("envConfig.value")}</Label>
            <SecretInput
              id="edit-env-value"
              value={formValue}
              onChange={(e) => { setFormValue(e.target.value); setApiError("") }}
              placeholder={t("envConfig.valuePlaceholder")}
              autoFocus
            />
            <ValueHints value={formValue} />
          </div>

          {apiError && (
            <p className="text-xs text-destructive">{apiError}</p>
          )}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={handleClose} className="mr-auto sm:mr-auto">
            {t("common.cancel")}
          </Button>
          <Button type="button" disabled={!canSubmit} onClick={handleSubmit}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface EnvConfigPanelProps {
  scope: "global" | "bee" | "department" | "worker"
  scopeId?: string
  onSubDialogChange?: (open: boolean) => void
}

export function EnvConfigPanel({ scope, scopeId, onSubDialogChange }: EnvConfigPanelProps) {
  const { t } = useTranslation()
  const { data: envs = [], isLoading } = useEnvList(scope, scopeId)
  const deleteEnv = useDeleteEnv(scope, scopeId)

  const [addDialogOpen, setAddDialogOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<EnvConfig | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<EnvConfig | null>(null)

  const existingKeys = useMemo(() => envs.map((e) => e.key), [envs])

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
        <Button size="sm" onClick={() => { setAddDialogOpen(true); onSubDialogChange?.(true) }}>
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
                        onClick={() => { setEditTarget(env); onSubDialogChange?.(true) }}
                        title={t("common.edit")}
                      >
                        <PencilIcon className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => { setDeleteTarget(env); onSubDialogChange?.(true) }}
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
        onOpenChange={(open) => { setAddDialogOpen(open); if (!open) onSubDialogChange?.(false) }}
        scope={scope}
        scopeId={scopeId}
        existingKeys={existingKeys}
      />

      <EditEnvDialog
        target={editTarget}
        onClose={() => { setEditTarget(null); onSubDialogChange?.(false) }}
        scope={scope}
        scopeId={scopeId}
      />

      <Dialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) { setDeleteTarget(null); onSubDialogChange?.(false) } }}>
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
