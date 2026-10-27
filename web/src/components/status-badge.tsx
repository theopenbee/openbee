import { useTranslation } from "react-i18next"
import { Badge, type BadgeTone } from "@/components/arc/badge/badge"
import { statusLabel } from "@/lib/format"
import { cn } from "@/lib/utils"

const statusTones: Record<string, BadgeTone> = {
  idle: "success",
  working: "info",
  error: "danger",
  pending: "neutral",
}

const dotStyles: Record<string, string> = {
  idle: "bg-success",
  working: "bg-accent",
  error: "bg-danger",
  pending: "bg-border-strong",
}

const statusAliases: Record<string, string> = {
  completed: "idle",
  running: "working",
  failed: "error",
}

export function StatusBadge({ status, size = "sm" }: { status: string; size?: "sm" | "md" }) {
  const { t } = useTranslation()
  const key = statusAliases[status] ?? status
  return (
    <Badge
      tone={statusTones[key] ?? "neutral"}
      size={size}
      icon={<span className={cn("block size-1.5 rounded-full", dotStyles[key] ?? "bg-border-strong", key === "working" && "animate-pulse-soft")} />}
    >
      {statusLabel(status, t)}
    </Badge>
  )
}
