import { useTranslation } from "react-i18next"
import { Avatar } from "@/components/arc/avatar/avatar"
import { statusLabel } from "@/lib/format"
import { cn } from "@/lib/utils"

export const presenceColor: Record<string, string> = {
  idle: "bg-success",
  working: "bg-accent",
  error: "bg-danger",
}

export function initials(name: string): string {
  const trimmed = name.trim()
  if (!trimmed) return "?"
  if (/[一-鿿]/.test(trimmed[0])) return trimmed.slice(0, 1)
  const parts = trimmed.split(/[\s_-]+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return trimmed.slice(0, 2).toUpperCase()
}

interface WorkerAvatarProps {
  name: string
  status: string
  size?: "default" | "sm" | "lg"
  className?: string
}

const sizes = { sm: "sm", default: "md", lg: "lg" } as const

export function WorkerAvatar({ name, status, size = "default", className }: WorkerAvatarProps) {
  const { t } = useTranslation()
  const color = presenceColor[status] ?? "bg-border-strong"
  const label = statusLabel(status, t)

  return (
    <span className={cn("relative inline-flex shrink-0", className)}>
      <Avatar name={Array.from(initials(name)).join(" ")} size={sizes[size]} aria-hidden="true" />
      <span
        className="absolute -right-0.5 -bottom-0.5 inline-flex size-2.5 items-center justify-center rounded-full ring-2 ring-surface"
        title={label}
        role="img"
        aria-label={label}
      >
        {status === "working" && (
          <span className={cn("absolute inline-flex size-full animate-ping rounded-full opacity-60", color)} />
        )}
        <span className={cn("relative inline-flex size-full rounded-full", color)} />
      </span>
    </span>
  )
}
