import { useRef, useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { useCreateEnv, useDeleteEnv, useUpdateEnv } from "@/hooks/use-envs"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { Input } from "@/components/arc/input/input"
import { PasswordField } from "@/components/arc/password-field/password-field"
import { Switch } from "@/components/arc/switch/switch"
import { Textarea } from "@/components/arc/textarea/textarea"
import { ApiError } from "@/lib/api"
import { getErrorMessage } from "@/lib/utils"
import type { EnvConfig, EnvScope } from "@/lib/types"

const FOOTER = "flex flex-col-reverse gap-2 sm:flex-row sm:justify-end"

function normalizeKey(raw: string) {
  return raw.toUpperCase().replace(/[^A-Z0-9_]/g, "")
}

function EnvValueField({
  label,
  value,
  onChange,
  multiline,
  onMultilineChange,
  autoFocus,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  multiline: boolean
  onMultilineChange: (multiline: boolean) => void
  autoFocus?: boolean
}) {
  const { t } = useTranslation()
  const description =
    value.length === 0
      ? undefined
      : value !== value.trim()
        ? t("envConfig.valueLengthSpace", { count: value.length, defaultValue: "{{count}} characters. Starts or ends with a space." })
        : t("envConfig.valueLength", { count: value.length, defaultValue: "{{count}} characters." })

  return (
    <div className="grid gap-2">
      {multiline ? (
        <Textarea
          label={label}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          description={description}
          placeholder={t("envConfig.valuePlaceholder", "Enter value")}
          spellCheck={false}
          autoComplete="off"
          rows={4}
          autoFocus={autoFocus}
        />
      ) : (
        <PasswordField
          label={label}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          description={description}
          messages={{
            showPassword: t("envConfig.showValue", "Show value"),
            hidePassword: t("envConfig.hideValue", "Hide value"),
          }}
          spellCheck={false}
          autoComplete="off"
          autoFocus={autoFocus}
        />
      )}
      <Switch
        label={t("envConfig.multiline", "Multi-line value")}
        checked={multiline}
        onCheckedChange={onMultilineChange}
      />
    </div>
  )
}

export function EnvAddDialog({
  open,
  onOpenChange,
  scope,
  scopeId,
  existingKeys,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  scope: EnvScope
  scopeId?: string
  existingKeys: string[]
}) {
  const { t } = useTranslation()
  const createEnv = useCreateEnv()
  const keyRef = useRef<HTMLInputElement>(null)
  const [formKey, setFormKey] = useState("")
  const [formValue, setFormValue] = useState("")
  const [multiline, setMultiline] = useState(false)
  const [keyError, setKeyError] = useState("")
  const [formError, setFormError] = useState("")
  const [lastAdded, setLastAdded] = useState("")
  const [pendingAction, setPendingAction] = useState<"close" | "another" | null>(null)

  const keyDuplicate = formKey.length > 0 && existingKeys.includes(formKey)
  const canSubmit = formKey.length > 0 && formValue.length > 0 && !keyDuplicate && !createEnv.isPending

  const resetFields = () => {
    setFormKey("")
    setFormValue("")
    setKeyError("")
    setFormError("")
  }

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      resetFields()
      setMultiline(false)
      setLastAdded("")
    }
    onOpenChange(next)
  }

  const submit = async (addAnother: boolean) => {
    if (!canSubmit) return
    setFormError("")
    setKeyError("")
    setPendingAction(addAnother ? "another" : "close")
    try {
      await createEnv.mutateAsync({ scope, scope_id: scopeId, key: formKey, value: formValue })
      if (addAnother) {
        setLastAdded(formKey)
        resetFields()
        keyRef.current?.focus()
      } else {
        handleOpenChange(false)
      }
    } catch (err) {
      if (err instanceof ApiError && err.code === "env_reserved_key") setKeyError(getErrorMessage(err))
      else setFormError(getErrorMessage(err))
    } finally {
      setPendingAction(null)
    }
  }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    void submit(false)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        title={t("envConfig.addTitle", "Add variable")}
        description={t("envConfig.addDescription", "Keys are converted to uppercase. Values are stored encrypted.")}
      >
        <form onSubmit={handleSubmit} noValidate className="grid gap-4">
          {lastAdded ? (
            <Alert tone="success" title={t("envConfig.added", { key: lastAdded, defaultValue: "{{key}} added" })}>
              {t("envConfig.addedDescription", "Add the next variable or close this dialog.")}
            </Alert>
          ) : null}
          {formError ? (
            <Alert tone="danger" title={t("envConfig.saveError", "Could not save the variable")}>
              {formError}
            </Alert>
          ) : null}
          <Input
            ref={keyRef}
            label={t("envConfig.key", "Key")}
            value={formKey}
            onChange={(e) => {
              setFormKey(normalizeKey(e.target.value))
              setKeyError("")
              setFormError("")
            }}
            placeholder={t("envConfig.keyPlaceholder", "DATABASE_URL")}
            description={keyDuplicate || keyError ? undefined : t("envConfig.keyHint", "Letters, digits and underscores.")}
            error={
              keyDuplicate
                ? t("envConfig.keyExists", { key: formKey, defaultValue: "{{key}} already exists here. Edit it instead." })
                : keyError || undefined
            }
            autoComplete="off"
            autoCapitalize="characters"
            spellCheck={false}
            autoFocus
          />
          <EnvValueField
            label={t("envConfig.value", "Value")}
            value={formValue}
            onChange={(next) => {
              setFormValue(next)
              setFormError("")
            }}
            multiline={multiline}
            onMultilineChange={setMultiline}
          />
          <div className={`mt-2 ${FOOTER}`}>
            <Button type="button" variant="ghost" onClick={() => handleOpenChange(false)} className="sm:mr-auto">
              {t("common.cancel", "Cancel")}
            </Button>
            <Button
              type="button"
              variant="secondary"
              disabled={!canSubmit && pendingAction !== "another"}
              loading={pendingAction === "another"}
              onClick={() => void submit(true)}
            >
              {t("envConfig.saveAndAddAnother", "Save and add another")}
            </Button>
            <Button
              type="submit"
              disabled={!canSubmit && pendingAction !== "close"}
              loading={pendingAction === "close"}
            >
              {t("envConfig.add", "Add variable")}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function EnvEditDialog({
  target,
  onClose,
  scope,
  scopeId,
}: {
  target: EnvConfig | null
  onClose: () => void
  scope: EnvScope
  scopeId?: string
}) {
  const { t } = useTranslation()
  const updateEnv = useUpdateEnv(scope, scopeId)
  const [formValue, setFormValue] = useState("")
  const [multiline, setMultiline] = useState(false)
  const [formError, setFormError] = useState("")
  const [shownKey, setShownKey] = useState("")

  if (target && target.key !== shownKey) setShownKey(target.key)

  const handleClose = () => {
    setFormValue("")
    setMultiline(false)
    setFormError("")
    onClose()
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!target || !formValue || updateEnv.isPending) return
    setFormError("")
    try {
      await updateEnv.mutateAsync({ id: target.id, value: formValue })
      handleClose()
    } catch (err) {
      setFormError(getErrorMessage(err))
    }
  }

  return (
    <Dialog open={target !== null} onOpenChange={(isOpen) => { if (!isOpen) handleClose() }}>
      <DialogContent
        title={t("envConfig.editTitle", { key: shownKey, defaultValue: "Edit {{key}}" })}
        description={t("envConfig.editDescription", "The current value stays hidden. Enter a new value to replace it.")}
      >
        <form onSubmit={handleSubmit} noValidate className="grid gap-4">
          {formError ? (
            <Alert tone="danger" title={t("envConfig.saveError", "Could not save the variable")}>
              {formError}
            </Alert>
          ) : null}
          <EnvValueField
            label={t("envConfig.newValue", "New value")}
            value={formValue}
            onChange={(next) => {
              setFormValue(next)
              setFormError("")
            }}
            multiline={multiline}
            onMultilineChange={setMultiline}
            autoFocus
          />
          <div className={`mt-2 ${FOOTER}`}>
            <Button type="button" variant="secondary" onClick={handleClose}>
              {t("common.cancel", "Cancel")}
            </Button>
            <Button
              type="submit"
              disabled={formValue.length === 0 && !updateEnv.isPending}
              loading={updateEnv.isPending}
            >
              {t("envConfig.saveValue", "Save value")}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function EnvDeleteDialog({
  target,
  onClose,
  scope,
  scopeId,
}: {
  target: EnvConfig | null
  onClose: () => void
  scope: EnvScope
  scopeId?: string
}) {
  const { t } = useTranslation()
  const deleteEnv = useDeleteEnv(scope, scopeId)
  const [formError, setFormError] = useState("")
  const [shownKey, setShownKey] = useState("")

  if (target && target.key !== shownKey) setShownKey(target.key)

  const handleClose = () => {
    setFormError("")
    deleteEnv.reset()
    onClose()
  }

  const handleDelete = async () => {
    if (!target || deleteEnv.isPending) return
    setFormError("")
    try {
      await deleteEnv.mutateAsync(target.id)
      handleClose()
    } catch (err) {
      setFormError(getErrorMessage(err))
    }
  }

  return (
    <Dialog open={target !== null} onOpenChange={(isOpen) => { if (!isOpen) handleClose() }}>
      <DialogContent
        title={t("envConfig.deleteTitle", { key: shownKey, defaultValue: "Delete {{key}}?" })}
        description={t(
          "envConfig.deleteConfirm",
          "Anything that uses this variable loses it on its next run. This cannot be undone.",
        )}
      >
        <div className="grid gap-4">
          {formError ? (
            <Alert tone="danger" title={t("envConfig.deleteError", "Could not delete the variable")}>
              {formError}
            </Alert>
          ) : null}
          <div className={FOOTER}>
            <Button type="button" variant="secondary" onClick={handleClose}>
              {t("common.cancel", "Cancel")}
            </Button>
            <Button type="button" variant="danger" loading={deleteEnv.isPending} onClick={() => void handleDelete()}>
              {t("envConfig.deleteAction", "Delete variable")}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
