import { useTranslation } from "react-i18next"
import { SettingsIcon } from "lucide-react"
import { Button } from "@/components/arc/button/button"
import { Tooltip } from "@/components/arc/tooltip/tooltip"
import { useSettingsDialog } from "@/hooks/use-settings-dialog"

export function SettingsButton() {
  const { t } = useTranslation()
  const show = useSettingsDialog()
  const label = t("nav.settings")

  return (
    <Tooltip content={label}>
      <Button variant="ghost" size="sm" aria-label={label} aria-haspopup="dialog" onClick={() => show({ section: "env" })}>
        <SettingsIcon size={16} strokeWidth={1.75} aria-hidden="true" />
      </Button>
    </Tooltip>
  )
}
