import { useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useChangePassword } from "@/hooks/use-change-password"
import { useLogout } from "@/hooks/use-logout"
import { getErrorMessage } from "@/lib/utils"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
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

const MIN_PASSWORD_LENGTH = 6

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
  const [error, setError] = useState("")

  const reset = () => {
    setOldPassword("")
    setNewPassword("")
    setConfirmPassword("")
    setError("")
  }

  const handleOpenChange = (next: boolean) => {
    if (!next) reset()
    onOpenChange(next)
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (newPassword !== confirmPassword) {
      setError(t("account.errorMismatch"))
      return
    }
    if (newPassword === oldPassword) {
      setError(t("account.errorSameAsOld"))
      return
    }
    try {
      await changePassword.mutateAsync({ oldPassword, newPassword })
      // Changing the password invalidates every existing session server-side,
      // so tear down this one and send the user back to the login page instead
      // of leaving them on a page whose tokens are already dead.
      handleOpenChange(false)
      toast.success(t("account.passwordChangedRelogin"))
      logout()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  const canSubmit =
    oldPassword.length > 0 &&
    newPassword.length >= MIN_PASSWORD_LENGTH &&
    confirmPassword.length > 0 &&
    !changePassword.isPending

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("account.changePassword")}</DialogTitle>
          <DialogDescription>{t("account.changePasswordDescription")}</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          {error && <div role="alert" className={ALERT_DESTRUCTIVE}>{error}</div>}
          <div className="space-y-1.5">
            <Label htmlFor="current-password">{t("account.currentPassword")}</Label>
            <Input
              id="current-password"
              type="password"
              autoComplete="current-password"
              value={oldPassword}
              onChange={(e) => setOldPassword(e.target.value)}
              placeholder={t("account.currentPasswordPlaceholder")}
              required
              autoFocus
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="new-password">{t("account.newPassword")}</Label>
            <Input
              id="new-password"
              type="password"
              autoComplete="new-password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              placeholder={t("account.newPasswordPlaceholder")}
              required
              minLength={MIN_PASSWORD_LENGTH}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="confirm-password">{t("account.confirmPassword")}</Label>
            <Input
              id="confirm-password"
              type="password"
              autoComplete="new-password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder={t("account.confirmPasswordPlaceholder")}
              required
              minLength={MIN_PASSWORD_LENGTH}
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!canSubmit}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
