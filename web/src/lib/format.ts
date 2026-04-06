export function formatTimestamp(value: number | null | undefined) {
  if (!value) return "—"
  return new Date(value).toLocaleString()
}

export function formatCompactTimestamp(value: number | null | undefined) {
  if (!value) return "—"
  return new Date(value).toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })
}

export function formatDuration(
  startMs: number | null | undefined,
  endMs: number | null | undefined
): string {
  if (!startMs || !endMs) return "—"
  const diff = endMs - startMs
  if (diff < 0) return "—"
  const totalSec = Math.floor(diff / 1000)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

export function statusTone(status: string) {
  switch (status) {
    case "idle":
    case "completed":
      return "text-status-idle"
    case "working":
    case "running":
      return "text-status-working"
    case "error":
    case "failed":
      return "text-status-error"
    default:
      return "text-muted-foreground"
  }
}

export function isActiveStatus(status: string) {
  return status === "running" || status === "pending"
}

export function isSameDay(left: number, right: number) {
  const leftDate = new Date(left)
  const rightDate = new Date(right)
  return (
    leftDate.getFullYear() === rightDate.getFullYear()
    && leftDate.getMonth() === rightDate.getMonth()
    && leftDate.getDate() === rightDate.getDate()
  )
}

export function formatRelative(ms: number | null): string {
  if (!ms) return "—"
  const diff = Date.now() - ms
  const sec = Math.floor(diff / 1000)
  if (sec < 60) return "just now"
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m ago`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr}h ago`
  return `${Math.floor(hr / 24)}d ago`
}

export const STATUS_ROW_BORDER: Record<string, string> = {
  pending: "border-l-transparent",
  running: "border-l-status-working",
  completed: "border-l-status-idle",
  failed: "border-l-status-error",
  cancelled: "border-l-transparent",
}
