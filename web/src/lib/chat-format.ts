import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import { basename } from "@/lib/utils"
import { isSameDay } from "@/lib/format"

export function formatMessageTimestamp(timestamp: number | null | undefined, language: string) {
  if (!timestamp) return ""
  return new Intl.DateTimeFormat(language, isSameDay(timestamp, Date.now())
    ? {
        hour: "numeric",
        minute: "2-digit",
      }
    : {
        month: "short",
        day: "numeric",
        hour: "numeric",
        minute: "2-digit",
      }).format(new Date(timestamp))
}

export function mediaUrl(mediaPath: string) {
  const filename = basename(mediaPath)
  return `${config.apiUrl}/local/media/${encodeURIComponent(filename)}${tokenParam()}`
}
