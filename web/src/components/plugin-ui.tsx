import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { useNavigate } from "react-router-dom"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { LoaderCircleIcon } from "lucide-react"
import { Alert } from "@/components/arc/alert/alert"
import { Avatar } from "@/components/arc/avatar/avatar"
import { Badge, type BadgeTone } from "@/components/arc/badge/badge"
import { Button } from "@/components/arc/button/button"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { usePluginOperations } from "@/hooks/use-plugins"
import { formatElapsed, localize, specPackage } from "@/lib/plugins"
import { cn } from "@/lib/utils"
import type { PluginCatalog, PluginOperation, PluginState, PluginStatus } from "@/lib/types"

const STATE_TONE: Record<PluginState, BadgeTone> = {
  running: "success",
  starting: "info",
  retrying: "warning",
  failed: "danger",
  not_configured: "warning",
  disabled: "neutral",
}

const STATE_DOT: Record<PluginState, string> = {
  running: "bg-success",
  starting: "bg-accent animate-pulse-soft",
  retrying: "bg-warning animate-pulse-soft",
  failed: "bg-danger",
  not_configured: "bg-warning",
  disabled: "bg-border-strong",
}

export function PluginStateBadge({ state, uninstalling = false }: { state: PluginState; uninstalling?: boolean }) {
  const { t } = useTranslation()
  if (uninstalling) {
    return (
      <Badge tone="neutral" size="sm" icon={<LoaderCircleIcon size={12} strokeWidth={1.75} className="animate-spin" />}>
        {t("plugins.state.uninstalling")}
      </Badge>
    )
  }
  return (
    <Badge
      tone={STATE_TONE[state] ?? "neutral"}
      size="sm"
      icon={<span className={cn("block size-1.5 rounded-full", STATE_DOT[state] ?? "bg-border-strong")} />}
    >
      {t(`plugins.state.${state}`, { defaultValue: state })}
    </Badge>
  )
}

export function PluginAvatar({ name, size = "md" }: { name: string; size?: "sm" | "md" | "lg" }) {
  return <Avatar name={name} size={size} aria-hidden="true" />
}

export function OperationProgress({ label, startedAt, now }: { label: string; startedAt?: string; now: number }) {
  const started = Date.parse(startedAt ?? "") || now
  return (
    <span className="inline-flex min-h-9 items-center gap-2 text-sm" role="status">
      <LoaderCircleIcon size={16} strokeWidth={1.75} className="shrink-0 animate-spin text-accent" aria-hidden="true" />
      <TextShimmer>{label}</TextShimmer>
      <span className="text-secondary tabular">{formatElapsed(now - started)}</span>
    </span>
  )
}

export function pendingUninstalls(ops: PluginOperation[]): Record<string, PluginOperation> {
  const out: Record<string, PluginOperation> = {}
  for (const op of ops) {
    if (op.kind !== "uninstall" || !op.plugin_id) continue
    if (op.status !== "queued" && op.status !== "running") continue
    if (!out[op.plugin_id]) out[op.plugin_id] = op
  }
  return out
}

export function useNow(active: boolean, intervalMs = 1000): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return
    setNow(Date.now())
    const timer = setInterval(() => setNow(Date.now()), intervalMs)
    return () => clearInterval(timer)
  }, [active, intervalMs])
  return now
}

export function usePluginOperationToasts() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const describe = (op: PluginOperation) => {
    const catalog = queryClient.getQueryData<PluginCatalog>(["plugin-catalog"])
    const installed = queryClient.getQueryData<PluginStatus[]>(["plugins"])
    const pkg = op.spec ? specPackage(op.spec) : ""
    const fromCatalog = catalog?.plugins.find((p) => (op.plugin_id ? p.id === op.plugin_id : p.package === pkg))
    const fromInstalled = installed?.find((p) => p.id === op.plugin_id)
    return {
      name: localize(fromCatalog?.name ?? fromInstalled?.name, i18n.language) || op.plugin_id || pkg,
      update: Boolean(fromCatalog?.installed),
      version: op.spec && pkg !== op.spec ? op.spec.slice(pkg.length + 1) : "",
    }
  }

  return usePluginOperations((op) => {
    const { name, update, version } = describe(op)
    if (op.kind === "install") {
      if (op.status === "succeeded" && update) {
        toast.success(t("plugins.updateDone", { name, version }))
      } else if (op.status === "succeeded") {
        toast.success(t("plugins.installDone", { name }), op.plugin_id
          ? { action: { label: t("plugins.installGoConfigure"), onClick: () => navigate(`/plugins/${op.plugin_id}`) } }
          : undefined)
      } else {
        toast.error(t("plugins.installFailed", { name, error: op.error ?? "" }))
      }
      return
    }
    if (op.status === "succeeded") toast.success(t("plugins.uninstallDone", { name }))
    else toast.error(t("plugins.uninstallFailed", { name, error: op.error ?? "" }))
  })
}

export function UninstallPluginDialog({
  name,
  open,
  pending,
  error,
  onOpenChange,
  onConfirm,
}: {
  name: string
  open: boolean
  pending: boolean
  error?: string
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title={t("plugins.uninstallDialog.title", { name })} description={t("plugins.uninstallDialog.description")}>
        {error && (
          <div className="mb-6">
            <Alert tone="danger" title={t("plugins.uninstallDialog.error")}>
              <span className="wrap-anywhere">{error}</span>
            </Alert>
          </div>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="danger" loading={pending} onClick={onConfirm}>
            {t("plugins.uninstallDialog.confirm")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
