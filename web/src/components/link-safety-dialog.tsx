import { ExternalLink } from "lucide-react"
import { useTranslation } from "react-i18next"
import type { LinkSafetyModalProps } from "streamdown"
import { Button } from "@/components/arc/button/button"
import { CopyButton } from "@/components/arc/copy-button/copy-button"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"

export function LinkSafetyDialog({ url, isOpen, onClose, onConfirm }: LinkSafetyModalProps) {
  const { t } = useTranslation()
  return (
    <Dialog
      open={isOpen}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent title={t("chat.linkTitle")} description={t("chat.linkDescription")}>
        <div className="max-h-32 overflow-y-auto break-all rounded-inset bg-surface-muted p-3 font-mono text-xs text-foreground select-text">
          {url}
        </div>
        <div className="mt-4 flex gap-2">
          <CopyButton value={url} label={t("chat.linkCopy")} className="flex-1" />
          <Button
            className="flex-1"
            onClick={() => {
              onConfirm()
              onClose()
            }}
          >
            <ExternalLink size={16} strokeWidth={1.75} aria-hidden="true" />
            {t("chat.linkOpen")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
