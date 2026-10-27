import { useTranslation } from "react-i18next"
import { Avatar } from "@/components/arc/avatar/avatar"
import { avatarColorClass } from "@/lib/avatar-color"
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

export type AvatarSize = "sm" | "default" | "lg" | "xl"

interface WorkerAvatarProps {
  workerId: string
  name: string
  status: string
  size?: AvatarSize
  className?: string
}

const sizes = { sm: "sm", default: "md", lg: "lg", xl: "xl" } as const

const dotPlacement: Record<AvatarSize, string> = {
  sm: "-right-0.5 -bottom-0.5 size-2.5",
  default: "-right-0.5 -bottom-0.5 size-2.5",
  lg: "-right-0.5 -bottom-0.5 size-2.5",
  xl: "right-1 bottom-1 size-3.5",
}

export function WorkerAvatar({ workerId, name, status, size = "default", className }: WorkerAvatarProps) {
  const { t } = useTranslation()
  const color = presenceColor[status] ?? "bg-border-strong"
  const label = statusLabel(status, t)

  return (
    <span className={cn("relative inline-flex shrink-0 rounded-full", avatarColorClass(workerId), className)}>
      <Avatar
        name={Array.from(initials(name)).join(" ")}
        size={sizes[size]}
        className="bg-transparent! text-background!"
        aria-hidden="true"
      />
      <span
        className={cn("absolute inline-flex items-center justify-center rounded-full ring-2 ring-surface", dotPlacement[size])}
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
