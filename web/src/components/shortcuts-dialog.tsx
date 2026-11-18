import { useTranslation } from "react-i18next"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { ShortcutList, type ShortcutListGroup } from "@/components/arc/shortcut-recorder/shortcut-recorder"
import { SHORTCUTS } from "@/lib/shortcuts"

interface ShortcutsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ShortcutsDialog({ open, onOpenChange }: ShortcutsDialogProps) {
  const { t } = useTranslation()
  const title = t("shortcuts.title")
  const groups: ShortcutListGroup[] = [
    {
      label: t("shortcuts.general"),
      items: [
        { label: t("shortcuts.quickSearch"), shortcut: SHORTCUTS.quickSearch },
        { label: t("shortcuts.showShortcuts"), shortcut: SHORTCUTS.showShortcuts },
        { label: t("shortcuts.openSettings"), shortcut: SHORTCUTS.openSettings },
        { label: t("shortcuts.close"), shortcut: "escape" },
      ],
    },
    {
      label: t("shortcuts.chat"),
      items: [
        { label: t("shortcuts.previousConversation"), shortcut: SHORTCUTS.previousConversation },
        { label: t("shortcuts.nextConversation"), shortcut: SHORTCUTS.nextConversation },
        { label: t("shortcuts.newConversation"), shortcut: SHORTCUTS.newConversation },
        { label: t("shortcuts.sendMessage"), shortcut: "enter" },
        { label: t("shortcuts.newLine"), shortcut: "shift+enter" },
      ],
    },
  ]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title={title} aria-describedby={undefined}>
        <ShortcutList groups={groups} label={title} searchable={false} />
      </DialogContent>
    </Dialog>
  )
}
