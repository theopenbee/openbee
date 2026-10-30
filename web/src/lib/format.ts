import { findEngine } from "./engines"
import type { EngineChoice } from "./types"

export function formatTimestamp(value: number | null | undefined) {
  if (!value) return "—"
  return new Date(value).toLocaleString()
}

export function statusLabel(status: string, t: (key: string, defaultValue: string) => string) {
  return t(`statuses.${status}`, status)
}

export function formatEngineLabel(engine: string | null | undefined, engines: readonly EngineChoice[]): string {
  return engine ? findEngine(engines, engine).name : "—"
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

export function formatListTime(ts: number, now: number, locale: string, yesterdayLabel: string): string {
  if (isSameDay(ts, now)) {
    return new Intl.DateTimeFormat(locale, { hour: "2-digit", minute: "2-digit", hour12: false }).format(ts)
  }
  const yesterday = new Date(now)
  yesterday.setDate(yesterday.getDate() - 1)
  if (isSameDay(ts, yesterday.getTime())) return yesterdayLabel
  if (new Date(ts).getFullYear() === new Date(now).getFullYear()) {
    return new Intl.DateTimeFormat(locale, { month: "numeric", day: "numeric" }).format(ts)
  }
  return new Intl.DateTimeFormat(locale, { year: "numeric", month: "numeric", day: "numeric" }).format(ts)
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

export function formatNumber(n: number): string {
  return n.toLocaleString()
}
