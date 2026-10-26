import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { useNavigate } from "react-router-dom"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { usePluginOperations } from "@/hooks/use-plugins"
import { localize, specPackage } from "@/lib/plugins"
import { cn } from "@/lib/utils"
import type { PluginCatalog, PluginOperation, PluginState, PluginStatus } from "@/lib/types"

const STATE_DOT: Record<PluginState, string> = {
  running: "bg-status-idle",
  starting: "bg-status-working",
  retrying: "bg-status-working",
  failed: "bg-status-error",
  not_configured: "bg-amber-500",
  disabled: "bg-muted-foreground/50",
}

export function PluginStateLabel({ state, className }: { state: PluginState; className?: string }) {
  const { t } = useTranslation()
  return (
    <span className={cn("inline-flex items-center gap-1.5 text-sm", className)}>
      <span aria-hidden="true" className={cn("size-1.5 shrink-0 rounded-full", STATE_DOT[state] ?? "bg-muted-foreground")} />
      {t(`plugins.state.${state}`, { defaultValue: state })}
    </span>
  )
}

export function PluginTile({ name, className }: { name: string; className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={cn(
        "flex size-9 shrink-0 items-center justify-center rounded-sm border border-border/70 bg-muted text-sm font-medium text-foreground",
        className,
      )}
    >
      {name.slice(0, 1).toUpperCase()}
    </div>
  )
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
  onOpenChange,
  onConfirm,
}: {
  name: string
  open: boolean
  pending: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="rounded-sm">
        <DialogHeader>
          <DialogTitle>{t("plugins.uninstallDialog.title", { name })}</DialogTitle>
          <DialogDescription>{t("plugins.uninstallDialog.description")}</DialogDescription>
        </DialogHeader>
        <DialogFooter className="rounded-none">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="destructive" disabled={pending} onClick={onConfirm}>
            {t("plugins.uninstallDialog.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
