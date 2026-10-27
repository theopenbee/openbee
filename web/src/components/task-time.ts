import type { Task } from "@/lib/types"

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

export function taskRunAt(task: Task): number | null {
  return task.type === "countdown" ? task.scheduled_at : task.next_run_at
}

export function formatRunAt(ts: number, locale: string, now: number): string {
  const date = new Date(ts)
  const sameYear = date.getFullYear() === new Date(now).getFullYear()
  return new Intl.DateTimeFormat(locale, {
    year: sameYear ? undefined : "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date)
}

export function formatRunAtFull(ts: number, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: "full", timeStyle: "medium" }).format(new Date(ts))
}

export function formatRunIn(ts: number, locale: string, now: number): string {
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" })
  const diff = ts - now
  const abs = Math.abs(diff)
  if (abs < 45_000) return rtf.format(0, "second")
  if (abs < HOUR) return rtf.format(Math.round(diff / MINUTE), "minute")
  if (abs < DAY) return rtf.format(Math.round(diff / HOUR), "hour")
  return rtf.format(Math.round(diff / DAY), "day")
}
