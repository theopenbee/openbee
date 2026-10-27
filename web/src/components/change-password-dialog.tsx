import { useRef, useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { AuthPasswordField } from "@/components/auth-password-field"
import { useChangePassword } from "@/hooks/use-change-password"
import { useLogout } from "@/hooks/use-logout"
import { ApiError } from "@/lib/api"
import { getErrorMessage } from "@/lib/utils"

const MIN_PASSWORD_LENGTH = 6

interface FieldErrors {
  oldPassword?: string
  newPassword?: string
  confirmPassword?: string
}

export function ChangePasswordDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const changePassword = useChangePassword()
  const logout = useLogout()

  const [oldPassword, setOldPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [errors, setErrors] = useState<FieldErrors>({})
  const [formError, setFormError] = useState("")
  const [done, setDone] = useState(false)
  const oldRef = useRef<HTMLInputElement>(null)
  const newRef = useRef<HTMLInputElement>(null)
  const confirmRef = useRef<HTMLInputElement>(null)

  const reset = () => {
    setOldPassword("")
    setNewPassword("")
    setConfirmPassword("")
    setErrors({})
    setFormError("")
    setDone(false)
    changePassword.reset()
  }

  const handleOpenChange = (next: boolean) => {
    if (next) {
      onOpenChange(true)
      return
    }
    const finished = done
    reset()
    onOpenChange(false)
    if (finished) logout()
  }

  const validate = (): FieldErrors => {
    const next: FieldErrors = {}
    if (!oldPassword) next.oldPassword = t("account.currentRequired", "Enter your current password")
    if (newPassword.length < MIN_PASSWORD_LENGTH) {
      next.newPassword = t("account.errorTooShort", "Use at least 6 characters")
    } else if (newPassword === oldPassword) {
      next.newPassword = t("account.errorSameAsOld", "Choose a password different from the current one")
    }
    if (!next.newPassword && confirmPassword !== newPassword) {
      next.confirmPassword = t("account.errorMismatch", "The passwords do not match")
    }
    return next
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (changePassword.isPending) return
    const next = validate()
    setErrors(next)
    setFormError("")
    const firstInvalid = next.oldPassword ? oldRef : next.newPassword ? newRef : next.confirmPassword ? confirmRef : null
    if (firstInvalid) {
      firstInvalid.current?.focus()
      return
    }
    try {
      await changePassword.mutateAsync({ oldPassword, newPassword })
      setDone(true)
    } catch (err) {
      if (err instanceof ApiError && err.code === "old_password_incorrect") {
        setErrors({ oldPassword: getErrorMessage(err) })
        oldRef.current?.select()
      } else {
        setFormError(getErrorMessage(err))
      }
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        title={t("account.changePassword", "Change password")}
        description={t("account.changePasswordDescription", "Changing it signs you out of every session.")}
      >
        {done ? (
          <div className="grid gap-6">
            <Alert tone="success" title={t("account.passwordChanged", "Password changed")}>
              {t("account.passwordChangedRelogin", "Sign in again with your new password.")}
            </Alert>
            <div className="flex justify-end">
              <Button type="button" autoFocus onClick={() => handleOpenChange(false)}>
                {t("account.signInAgain", "Sign in again")}
              </Button>
            </div>
          </div>
        ) : (
          <form onSubmit={handleSubmit} noValidate className="grid gap-4">
            {formError ? (
              <Alert tone="danger" title={t("account.errorTitle", "Could not change the password")}>
                {formError}
              </Alert>
            ) : null}
            <AuthPasswordField
              ref={oldRef}
              label={t("account.currentPassword", "Current password")}
              autoComplete="current-password"
              value={oldPassword}
              onChange={(e) => {
                setOldPassword(e.target.value)
                setErrors((prev) => ({ ...prev, oldPassword: undefined }))
              }}
              error={errors.oldPassword}
              autoFocus
            />
            <AuthPasswordField
              ref={newRef}
              label={t("account.newPassword", "New password")}
              autoComplete="new-password"
              value={newPassword}
              onChange={(e) => {
                setNewPassword(e.target.value)
                setErrors((prev) => ({ ...prev, newPassword: undefined }))
              }}
              description={errors.newPassword ? undefined : t("account.newPasswordHint", "Use at least 6 characters.")}
              error={errors.newPassword}
              minLength={MIN_PASSWORD_LENGTH}
            />
            <AuthPasswordField
              ref={confirmRef}
              label={t("account.confirmPassword", "Confirm new password")}
              autoComplete="new-password"
              value={confirmPassword}
              onChange={(e) => {
                setConfirmPassword(e.target.value)
                setErrors((prev) => ({ ...prev, confirmPassword: undefined }))
              }}
              error={errors.confirmPassword}
              minLength={MIN_PASSWORD_LENGTH}
            />
            <div className="mt-2 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <Button type="button" variant="secondary" onClick={() => handleOpenChange(false)}>
                {t("common.cancel", "Cancel")}
              </Button>
              <Button type="submit" loading={changePassword.isPending}>
                {t("account.changePassword", "Change password")}
              </Button>
            </div>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
