import { useState } from "react"
import { useTranslation } from "react-i18next"
import { KeyRoundIcon } from "lucide-react"
import { UserMenu } from "@/components/arc/user-menu/user-menu"
import { ChangePasswordDialog } from "@/components/change-password-dialog"
import { useLanguage } from "@/hooks/use-language"
import { useLogout } from "@/hooks/use-logout"
import { useThemePreference } from "@/hooks/use-theme-toggle"
import { getStoredUsername } from "@/lib/auth"
import { LANGUAGES } from "@/lib/language"

export function NavUser({
  showName = false,
  align = "end",
  className,
}: {
  showName?: boolean
  align?: "start" | "center" | "end"
  className?: string
}) {
  const { t } = useTranslation()
  const logout = useLogout()
  const { preference, setPreference } = useThemePreference()
  const { language, setLanguage } = useLanguage()
  const [changePasswordOpen, setChangePasswordOpen] = useState(false)
  const username = getStoredUsername() ?? "admin"

  return (
    <>
      <UserMenu
        className={className}
        user={{ name: username, email: t("account.role", "Administrator") }}
        showName={showName}
        align={align}
        theme={preference}
        onThemeChange={setPreference}
        language={language}
        languages={LANGUAGES}
        onLanguageChange={setLanguage}
        items={[
          {
            label: t("account.changePassword", "Change password"),
            icon: <KeyRoundIcon size={16} strokeWidth={1.75} />,
            onSelect: () => setChangePasswordOpen(true),
          },
        ]}
        onSignOut={logout}
      />
      <ChangePasswordDialog open={changePasswordOpen} onOpenChange={setChangePasswordOpen} />
    </>
  )
}
