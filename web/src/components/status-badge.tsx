import { useTranslation } from "react-i18next"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

// Kumo status badges: a soft tint of the state's hue with darker text, plus a
// dot so state never rides on color alone (the label carries it too).
const statusVariants: Record<string, "success" | "info" | "destructive" | "secondary"> = {
  idle: "success",
  working: "info",
  error: "destructive",
  pending: "secondary",
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
    <Badge variant={statusVariants[key] ?? "secondary"}>
      <span className={cn("size-1.5 rounded-full", dotStyles[key] ?? "bg-muted-foreground")} aria-hidden="true" />
      {t(`statuses.${status}`, status)}
    </Badge>
  )
}
