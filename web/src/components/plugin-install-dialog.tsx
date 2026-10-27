import { useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { Input } from "@/components/arc/input/input"
import { OperationProgress, useNow } from "@/components/plugin-ui"
import { useInstallPlugin } from "@/hooks/use-plugins"
import { useSettingsDialog } from "@/hooks/use-settings-dialog"
import { getErrorMessage } from "@/lib/utils"
import type { PluginOperation } from "@/lib/types"

export function PluginInstallDialog({
  open,
  onOpenChange,
  operations,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  operations: PluginOperation[]
}) {
  const { t } = useTranslation()
  const show = useSettingsDialog()
  const install = useInstallPlugin()
  const [spec, setSpec] = useState("")
  const [opId, setOpId] = useState<string | null>(null)
  const [error, setError] = useState("")

  const op = opId ? operations.find((o) => o.id === opId) : undefined
  const running = Boolean(op && (op.status === "queued" || op.status === "running"))
  const succeeded = op?.status === "succeeded"
  const failed = op?.status === "failed"
  const now = useNow(open && running)

  const close = (next: boolean) => {
    onOpenChange(next)
    if (!next) {
      setSpec("")
      setOpId(null)
      setError("")
    }
  }

  const submit = (event: FormEvent) => {
    event.preventDefault()
    const value = spec.trim()
    if (!value || running) return
    setError("")
    setOpId(null)
    install.mutate(value, {
      onSuccess: (created) => setOpId(created.id),
      onError: (err) => setError(getErrorMessage(err)),
    })
  }

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent title={t("plugins.installDialog.title")} description={t("plugins.installDialog.description")}>
        <form onSubmit={submit} className="grid gap-5">
          <Input
            label={t("plugins.installDialog.spec")}
            description={t("plugins.installDialog.specHelp")}
            value={spec}
            onChange={(e) => setSpec(e.target.value)}
            placeholder={t("plugins.installDialog.placeholder")}
            spellCheck={false}
            autoCapitalize="off"
            autoComplete="off"
            disabled={running || succeeded}
            autoFocus
          />
          {running && op && (
            <OperationProgress label={t("plugins.installDialog.installing")} startedAt={op.created_at} now={now} />
          )}
          {succeeded && (
            <Alert tone="success" title={t("plugins.installDialog.done")}>
              {t("plugins.installDialog.doneHint")}
            </Alert>
          )}
          {(failed || error) && (
            <Alert tone="danger" title={t("plugins.installDialog.failed")}>
              <span className="font-mono text-xs wrap-anywhere">{error || op?.error}</span>
            </Alert>
          )}
          <div className="mt-1 flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={() => close(false)}>
              {succeeded || running ? t("plugins.installDialog.close") : t("common.cancel")}
            </Button>
            {succeeded && op?.plugin_id ? (
              <Button
                type="button"
                onClick={() => {
                  close(false)
                  show({ section: "plugins", plugin: op.plugin_id })
                }}
              >
                {t("plugins.installDialog.configure")}
              </Button>
            ) : (
              <Button type="submit" loading={install.isPending} disabled={!spec.trim() || succeeded || running}>
                {t("plugins.installDialog.submit")}
              </Button>
            )}
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}
