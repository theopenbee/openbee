import { useTranslation } from "react-i18next"
import { SettingsIcon } from "lucide-react"
import { Button } from "@/components/arc/button/button"
import { Tooltip } from "@/components/arc/tooltip/tooltip"
import { useShortcutText } from "@/hooks/use-command"
import { useOpenSettings } from "@/hooks/use-settings"

export function SettingsButton() {
  const { t } = useTranslation()
  const open = useOpenSettings()
  const label = t("nav.settings")
  const shortcut = useShortcutText("openSettings")

  return (
    <Tooltip content={`${label} ${shortcut}`}>
      <Button variant="ghost" size="sm" aria-label={label} onClick={() => open({ section: "env" })}>
        <SettingsIcon size={16} strokeWidth={1.75} aria-hidden="true" />
      </Button>
    </Tooltip>
  )
}
