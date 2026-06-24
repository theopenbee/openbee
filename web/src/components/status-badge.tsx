import { useTranslation } from "react-i18next"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

const statusStyles: Record<string, string> = {
  idle: "bg-status-idle/15 text-status-idle border-status-idle/20",
  working: "bg-status-working/15 text-status-working border-status-working/20",
  error: "bg-status-error/15 text-status-error border-status-error/20",
  pending: "bg-muted text-muted-foreground border-border",
}

const dotStyles: Record<string, string> = {
  idle: "bg-status-idle",
  working: "bg-status-working",
  error: "bg-status-error",
  pending: "bg-muted-foreground",
}

const statusAliases: Record<string, string> = {
  completed: "idle",
  running: "working",
  failed: "error",
}

export function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  const key = statusAliases[status] ?? status
  return (
    <Badge variant="outline" className={statusStyles[key] || "bg-muted text-muted-foreground"}>
      <span className={cn("size-1.5 rounded-full", dotStyles[key] ?? "bg-muted-foreground")} aria-hidden="true" />
      {t(`statuses.${status}`, status)}
    </Badge>
  )
}
