import type { ChatMessage } from "@/lib/types"

export function messageKeys(messages: ChatMessage[]) {
  const seen = new Map<string, number>()
  return messages.map((message) => {
    const base = `${message.role}-${message.ts}`
    const count = seen.get(base) ?? 0
    seen.set(base, count + 1)
    return count === 0 ? base : `${base}-${count}`
  })
}

export function mergeLatestPage(prev: ChatMessage[], latest: ChatMessage[]) {
  if (latest.length === 0) return latest
  const firstTs = latest[0].ts
  const cut = prev.findIndex((message) => message.ts >= firstTs)
  const older = cut === -1 ? prev : prev.slice(0, cut)
  return older.length > 0 ? [...older, ...latest] : latest
}
