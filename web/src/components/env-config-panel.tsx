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
import { getErrorMessage } from "@/lib/utils"
import { ALERT_DESTRUCTIVE, SURFACE } from "@/lib/styles"
import { SkeletonLine } from "@/components/skeleton-loader"
import { useCan } from "@/hooks/use-can"
import { Perm } from "@/lib/permissions"
import type { EnvConfig, EnvScope } from "@/lib/types"

function ValueHints({ value }: { value: string }) {
  const { t } = useTranslation()
  const hasLeadingTrailingSpace = value.length > 0 && value !== value.trim()
  return (
    <div className="flex min-h-4 items-center justify-between gap-3">
      {hasLeadingTrailingSpace && (
        <p className="text-xs text-warning">{t("envConfig.valueSpaceWarning")}</p>
      )}
      {value.length > 0 && (
        <span className="ml-auto text-xs tabular-nums text-muted-foreground">{value.length}</span>
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
        className="pr-10 font-mono"
      />
      <button
        type="button"
        onClick={() => setShow(!show)}
        className="absolute top-1/2 right-1.5 grid size-6 -translate-y-1/2 place-items-center rounded-sm text-muted-foreground transition-colors hover:bg-accent hover:text-strong"
        tabIndex={-1}
        title={show ? t("envConfig.hideValue") : t("envConfig.showValue")}
        aria-label={show ? t("envConfig.hideValue") : t("envConfig.showValue")}
      >
        {show ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
      </button>
    </div>
  )
}

interface AddEnvDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  scope: EnvScope
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
      <DialogContent className="sm:max-w-md" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{t("envConfig.addTitle")}</DialogTitle>
          <DialogDescription>{t("envConfig.addDescription")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          {apiError && (
            <div role="alert" className={ALERT_DESTRUCTIVE}>{apiError}</div>
          )}

          <div className="space-y-1.5">
            <div className="flex items-center justify-between gap-3">
              <Label htmlFor="add-env-key">{t("envConfig.key")}</Label>
              <span className="font-mono text-xs text-muted-foreground select-none">
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
              aria-invalid={keyDuplicate || undefined}
              aria-describedby={keyDuplicate ? "add-env-key-error" : undefined}
              className="font-mono"
            />
            {keyDuplicate && (
              <p id="add-env-key-error" className="text-xs text-destructive-foreground">
                {t("envConfig.keyExists")}
              </p>
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
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={handleClose}>
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
  scope: EnvScope
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
      <DialogContent className="sm:max-w-md" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{t("envConfig.editTitle")}</DialogTitle>
          <DialogDescription className="font-mono text-body-sm break-all text-foreground">
            {target?.key}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          {apiError && (
            <div role="alert" className={ALERT_DESTRUCTIVE}>{apiError}</div>
          )}

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
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={handleClose}>
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
  scope: EnvScope
  scopeId?: string
  title?: string
}

export function EnvConfigPanel({ scope, scopeId, title }: EnvConfigPanelProps) {
  const { t } = useTranslation()
  const canWrite = useCan(Perm.EnvWrite)
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
    <div className="space-y-3">
      {(title || canWrite) && (
        <div className="flex min-h-8 flex-wrap items-center justify-between gap-x-4 gap-y-2">
          {title ? (
            <h2 className="min-w-0 text-sm font-semibold text-strong">{title}</h2>
          ) : (
            <span aria-hidden="true" />
          )}
          {canWrite && (
            <Button variant="outline" size="sm" className="shrink-0" onClick={() => setAddDialogOpen(true)}>
              <PlusIcon />
              {t("envConfig.add")}
            </Button>
          )}
        </div>
      )}

      <div className={SURFACE}>
        {isLoading ? (
          <div className="space-y-3 px-3 py-4" aria-hidden="true">
            <SkeletonLine className="w-2/3" />
            <SkeletonLine className="w-1/2" />
            <SkeletonLine className="w-3/5" />
          </div>
        ) : envs.length === 0 ? (
          <p className="px-4 py-10 text-center text-sm text-muted-foreground">
            {t("envConfig.empty")}
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("envConfig.key")}</TableHead>
                <TableHead className="hidden sm:table-cell">{t("envConfig.masked")}</TableHead>
                {canWrite && (
                  <TableHead className="w-0 text-right">
                    <span className="sr-only">{t("workers.columns.actions")}</span>
                  </TableHead>
                )}
              </TableRow>
            </TableHeader>
            <TableBody>
              {envs.map((env) => (
                <TableRow key={env.id}>
                  <TableCell className="max-w-0 font-mono text-body-sm font-medium text-strong sm:max-w-none">
                    <div className="truncate">{env.key}</div>
                    {/* Below sm the value folds under the key so the row
                        actions stay on screen. */}
                    <div className="mt-0.5 truncate font-normal text-muted-foreground sm:hidden">
                      {env.masked}
                    </div>
                  </TableCell>
                  <TableCell className="hidden font-mono text-body-sm text-muted-foreground sm:table-cell">
                    {env.masked}
                  </TableCell>
                  {canWrite && (
                    <TableCell className="py-2 text-right">
                      <div className="flex items-center justify-end gap-0.5">
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="text-muted-foreground"
                          onClick={() => setEditTarget(env)}
                          title={t("common.edit")}
                          aria-label={`${t("common.edit")} ${env.key}`}
                        >
                          <PencilIcon className="size-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="text-muted-foreground hover:bg-danger-tint hover:text-destructive-foreground"
                          onClick={() => setDeleteTarget(env)}
                          title={t("common.delete")}
                          aria-label={`${t("common.delete")} ${env.key}`}
                        >
                          <Trash2Icon className="size-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                  )}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      <AddEnvDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        scope={scope}
        scopeId={scopeId}
        existingKeys={existingKeys}
      />

      <EditEnvDialog
        target={editTarget}
        onClose={() => setEditTarget(null)}
        scope={scope}
        scopeId={scopeId}
      />

      <Dialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}>
        <DialogContent>
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
