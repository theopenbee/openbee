import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { QRCodeSVG } from "qrcode.react"
import { toast } from "sonner"
import { Loader2Icon, RefreshCwIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { usePluginSetup } from "@/hooks/use-plugins"
import { isLink, latestLink } from "@/lib/plugins"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
import { cn, getErrorMessage } from "@/lib/utils"

const VISIBLE_LINES = 6

export function PluginSetupDialog({
  pluginId,
  title,
  open,
  onOpenChange,
}: {
  pluginId: string
  title: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [startError, setStartError] = useState<string | null>(null)
  const [text, setText] = useState("")
  const [sending, setSending] = useState(false)
  const setup = usePluginSetup(pluginId, (ev) => {
    if (!ev.ok) return
    toast.success(t("plugins.setup.success"))
    onOpenChange(false)
  })
  const { start, cancel } = setup
  const cancelRef = useRef(cancel)
  cancelRef.current = cancel

  const begin = () => {
    setStartError(null)
    start().catch((err) => setStartError(getErrorMessage(err)))
  }

  useEffect(() => {
    if (!open) return
    setStartError(null)
    start().catch((err) => setStartError(getErrorMessage(err)))
    return () => {
      cancelRef.current()
    }
  }, [open, start])

  const link = latestLink(setup.lines)
  const progress = setup.lines.filter((line) => !isLink(line)).slice(-VISIBLE_LINES)
  const failed = setup.done && !setup.done.ok ? setup.done.error || "" : null
  const active = Boolean(setup.sessionId) && !setup.done
  const stopped = Boolean(startError) || failed !== null

  const send = async () => {
    if (!text.trim()) return
    setSending(true)
    try {
      await setup.input(text.trim())
      setText("")
    } catch (err) {
      toast.error(getErrorMessage(err))
    } finally {
      setSending(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="rounded-sm sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{t("plugins.setup.scanHint")}</DialogDescription>
        </DialogHeader>

        <div className="flex h-64 items-center justify-center rounded-sm border border-border/70 bg-white p-3">
          {stopped ? (
            <span className="text-sm text-neutral-500">—</span>
          ) : link ? (
            <QRCodeSVG value={link} size={224} marginSize={1} title={link} />
          ) : (
            <span className="inline-flex items-center gap-2 text-sm text-neutral-500">
              <Loader2Icon className="size-4 animate-spin" />
              {t("plugins.setup.waiting")}
            </span>
          )}
        </div>
        <p className="-mt-2 text-xs text-muted-foreground">{t("plugins.setup.refreshHint")}</p>

        {progress.length > 0 && (
          <div className="space-y-1 rounded-sm bg-muted/50 px-3 py-2 font-mono text-xs text-muted-foreground" aria-live="polite">
            {progress.map((line, i) => (
              <div key={`${setup.lines.length}-${i}`} className={cn("break-words", i === progress.length - 1 && "text-foreground")}>
                {line}
              </div>
            ))}
          </div>
        )}
        {active && !setup.connected && setup.lines.length > 0 && (
          <p className="text-xs text-muted-foreground">{t("plugins.setup.reconnecting")}</p>
        )}
        {(startError || failed !== null) && (
          <div className={ALERT_DESTRUCTIVE}>{startError ?? t("plugins.setup.failed", { error: failed })}</div>
        )}

        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            void send()
          }}
        >
          <Input
            className="rounded-sm"
            value={text}
            disabled={!active}
            onChange={(e) => setText(e.target.value)}
            placeholder={t("plugins.setup.inputPlaceholder")}
            aria-label={t("plugins.setup.inputPlaceholder")}
          />
          <Button type="submit" variant="outline" disabled={!active || sending || !text.trim()}>
            {t("plugins.setup.send")}
          </Button>
        </form>

        <DialogFooter className="rounded-none">
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="outline" onClick={begin}>
            <RefreshCwIcon />
            {t("plugins.setup.again")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
