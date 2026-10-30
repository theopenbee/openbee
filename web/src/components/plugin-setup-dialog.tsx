import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { QRCodeSVG } from "qrcode.react"
import { toast } from "sonner"
import { CircleSlashIcon, LoaderCircleIcon, RefreshCwIcon } from "lucide-react"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { usePluginSetup } from "@/hooks/use-plugins"
import { isLink, latestLink } from "@/lib/plugins"
import { FIELD_CONTROL } from "@/lib/styles"
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
  const stopped = Boolean(startError) || failed !== null || setup.lost
  const reconnecting = active && !setup.connected && !setup.lost && setup.lines.length > 0

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
      <DialogContent title={title} description={t("plugins.setup.scanHint")}>
        <div className="grid gap-4">
          <div className="grid justify-items-center gap-3 rounded-inset border border-border-subtle bg-surface-muted px-4 py-5">
            <div className="grid size-60 place-items-center">
              {stopped ? (
                <span className="inline-flex flex-col items-center gap-2 text-sm text-secondary">
                  <CircleSlashIcon size={24} strokeWidth={1.5} aria-hidden="true" />
                  {t("plugins.setup.stopped")}
                </span>
              ) : link ? (
                <QRCodeSVG value={link} size={240} marginSize={2} title={link} />
              ) : (
                <span className="inline-flex items-center gap-2 text-sm text-secondary" role="status">
                  <LoaderCircleIcon size={16} strokeWidth={1.75} className="animate-spin" aria-hidden="true" />
                  <TextShimmer>{t("plugins.setup.waiting")}</TextShimmer>
                </span>
              )}
            </div>
            <p className="text-center text-xs text-secondary">{t("plugins.setup.refreshHint")}</p>
          </div>

          {progress.length > 0 && (
            <div
              className="grid gap-1 rounded-inset border border-border-subtle bg-surface-muted px-4 py-3 font-mono text-xs text-secondary"
              aria-live="polite"
              aria-label={t("plugins.setup.output")}
            >
              {progress.map((line, i) => (
                <div
                  key={`${setup.lines.length}-${i}`}
                  className={cn("wrap-anywhere", i === progress.length - 1 && "text-foreground")}
                >
                  {line}
                </div>
              ))}
            </div>
          )}

          {reconnecting && (
            <span className="inline-flex items-center gap-2 text-xs text-secondary" role="status">
              <LoaderCircleIcon size={14} strokeWidth={1.75} className="animate-spin" aria-hidden="true" />
              <TextShimmer>{t("plugins.setup.reconnecting")}</TextShimmer>
            </span>
          )}

          {stopped && (
            <Alert tone="danger" title={t("plugins.setup.stoppedTitle")}>
              <span className="wrap-anywhere">
                {startError ?? (failed !== null ? t("plugins.setup.failed", { error: failed }) : t("plugins.setup.lost"))}
              </span>
            </Alert>
          )}

          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              void send()
            }}
          >
            <input
              className={cn(FIELD_CONTROL, "min-w-0 flex-1")}
              value={text}
              disabled={!active}
              onChange={(e) => setText(e.target.value)}
              placeholder={t("plugins.setup.inputPlaceholder")}
              aria-label={t("plugins.setup.inputLabel")}
              autoComplete="off"
            />
            <Button type="submit" variant="secondary" loading={sending} disabled={!active || !text.trim()}>
              {t("plugins.setup.send")}
            </Button>
          </form>
        </div>

        <div className="mt-6 flex flex-wrap justify-end gap-2">
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="secondary" loading={setup.starting} onClick={begin}>
            <RefreshCwIcon size={16} strokeWidth={1.75} aria-hidden="true" />
            {t("plugins.setup.again")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
