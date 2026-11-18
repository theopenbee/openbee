import { useState } from "react"
import { useTranslation } from "react-i18next"
import { KeyboardIcon, KeyRoundIcon } from "lucide-react"
import { shortcutTokens, usePlatform } from "@/components/arc/shortcut-recorder/shortcut-recorder"
import { UserMenu } from "@/components/arc/user-menu/user-menu"
import { ChangePasswordDialog } from "@/components/change-password-dialog"
import { useLanguage } from "@/hooks/use-language"
import { useLogout } from "@/hooks/use-logout"
import { useNotificationSettings } from "@/hooks/use-notifications"
import { useThemePreference } from "@/hooks/use-theme-toggle"
import { getStoredUsername } from "@/lib/auth"
import { LANGUAGES } from "@/lib/language"
import { runCommand, SHORTCUTS } from "@/lib/shortcuts"

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
  const notifications = useNotificationSettings()
  const [changePasswordOpen, setChangePasswordOpen] = useState(false)
  const username = getStoredUsername() ?? "admin"
  const platform = usePlatform()

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
        notifications={{
          enabled: notifications.active,
          disabled: notifications.unavailable !== null,
          note: notifications.unavailable ? t(`notifications.unavailable.${notifications.unavailable}`) : undefined,
          onChange: (enabled) => (enabled ? void notifications.enable() : notifications.disable()),
        }}
        items={[
          {
            label: t("account.changePassword", "Change password"),
            icon: <KeyRoundIcon size={16} strokeWidth={1.75} />,
            onSelect: () => setChangePasswordOpen(true),
          },
          {
            label: t("shortcuts.title"),
            icon: <KeyboardIcon size={16} strokeWidth={1.75} />,
            keys: shortcutTokens(SHORTCUTS.showShortcuts, platform).map((token) => token.label),
            onSelect: () => runCommand("showShortcuts"),
          },
        ]}
        onSignOut={logout}
      />
      <ChangePasswordDialog open={changePasswordOpen} onOpenChange={setChangePasswordOpen} />
    </>
  )
}
