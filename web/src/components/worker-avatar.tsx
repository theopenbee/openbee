import { useTranslation } from "react-i18next"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { cn } from "@/lib/utils"

export const presenceColor: Record<string, string> = {
  idle: "bg-status-idle",
  working: "bg-status-working",
  error: "bg-status-error",
}

// First glyph for CJK names, first letters of the first two words otherwise.
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
  // `xl` (56px) anchors a profile header; the others follow the Avatar scale.
  size?: "default" | "sm" | "lg" | "xl"
  className?: string
}

// The presence dot is labeled (role="img" + aria-label + title) so state never
// rides on color alone, and its working pulse stops under reduced motion.
export function WorkerAvatar({ name, status, size = "default", className }: WorkerAvatarProps) {
  const { t } = useTranslation()
  const color = presenceColor[status] ?? "bg-muted-foreground"
  const statusLabel = t(`statuses.${status}`, status)
  const isXl = size === "xl"

  return (
    <span className={cn("relative inline-flex shrink-0", className)}>
      <Avatar size={isXl ? "lg" : size} className={cn(isXl && "size-14 data-[size=lg]:size-14")}>
        <AvatarFallback className={cn("font-medium uppercase", isXl && "text-xl")}>
          {initials(name)}
        </AvatarFallback>
      </Avatar>
      <span
        className={cn(
          "absolute inline-flex items-center justify-center rounded-full ring-2 ring-background",
          isXl ? "right-0.5 bottom-0.5 size-3.5" : "-right-0.5 -bottom-0.5 size-2.5"
        )}
        title={statusLabel}
        role="img"
        aria-label={statusLabel}
      >
        {status === "working" && (
          <span className={cn("absolute inline-flex size-full animate-ping rounded-full opacity-60 motion-reduce:hidden", color)} />
        )}
        <span className={cn("relative inline-flex size-full rounded-full", color)} />
      </span>
    </span>
  )
}
