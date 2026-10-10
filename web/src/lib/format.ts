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
  return formatTotalDuration(diff)
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

export function formatEngineLabel(engine: string | null | undefined, t: (key: string) => string): string {
  return engine ? t(`workers.engines.${engine}`) : "—"
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

type RelativeTranslate = (key: string, options?: { count: number }) => string

export function formatRelative(ms: number | null, t: RelativeTranslate): string {
  if (!ms) return "—"
  const diff = Date.now() - ms
  const sec = Math.floor(diff / 1000)
  if (sec < 60) return t("time.justNow")
  const min = Math.floor(sec / 60)
  if (min < 60) return t("time.minutesAgo", { count: min })
  const hr = Math.floor(min / 60)
  if (hr < 24) return t("time.hoursAgo", { count: hr })
  return t("time.daysAgo", { count: Math.floor(hr / 24) })
}

export function groupExecutionsBySession<T extends { session_id: string; started_at: number | null }>(
  executions: T[]
): T[][] {
  const map = new Map<string, T[]>()
  for (const e of executions) {
    const group = map.get(e.session_id) ?? []
    group.push(e)
    map.set(e.session_id, group)
  }
  return Array.from(map.values()).sort((a, b) => (b[0].started_at ?? 0) - (a[0].started_at ?? 0))
}

export function formatChange(ratio: number | null): string | null {
  if (ratio === null) return null
  const pct = (ratio * 100).toFixed(1)
  return ratio >= 0 ? `+${pct}%` : `${pct}%`
}

export const STATUS_ROW_BORDER: Record<string, string> = {
  pending: "border-l-transparent",
  running: "border-l-status-working",
  completed: "border-l-status-idle",
  failed: "border-l-status-error",
  cancelled: "border-l-transparent",
}

const CONTENT_TAG_RE = /<(message_content|task_content)>([\s\S]*?)<\/\1>/
const FRONTMATTER_RE = /^---\n[\s\S]*?\n---\n\n?/

export function extractMessageContent(input: string): string {
  if (!input) return input

  const newFormatMatch = input.match(CONTENT_TAG_RE)
  if (newFormatMatch) return newFormatMatch[2].trim()

  const oldFormatMatch = input.match(FRONTMATTER_RE)
  if (oldFormatMatch) return input.slice(oldFormatMatch[0].length).trim()

  return input
}

// Examples: 45000 → "45s", 90000 → "1m 30s", 8100000 → "2h 15m", 97200000 → "1d 3h"
export function formatTotalDuration(ms: number): string {
  if (ms <= 0) return "0s"
  const totalSec = Math.floor(ms / 1000)
  const days = Math.floor(totalSec / 86400)
  const hours = Math.floor((totalSec % 86400) / 3600)
  const minutes = Math.floor((totalSec % 3600) / 60)
  const seconds = totalSec % 60
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}

export function formatTokenCount(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

// Compact variant for y-axis tick labels: drops the decimal for 3-digit values
// to prevent label clipping caused by the chart's negative left margin.
export function formatTokenCountAxis(n: number): string {
  if (n >= 100_000_000_000) return `${Math.round(n / 1_000_000_000)}B`
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`
  if (n >= 100_000_000) return `${Math.round(n / 1_000_000)}M`
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 100_000) return `${Math.round(n / 1_000)}K`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

export function formatNumber(n: number): string {
  return n.toLocaleString()
}
