import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import { basename } from "@/lib/utils"
import { isSameDay } from "@/lib/format"

export function formatMessageTimestamp(timestamp: number | null | undefined, language: string) {
  if (!timestamp) return ""
  return new Intl.DateTimeFormat(language, isSameDay(timestamp, Date.now())
    ? {
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      }
    : {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      }).format(new Date(timestamp))
}

export function mediaUrl(mediaPath: string) {
  const filename = basename(mediaPath)
  return `${config.apiUrl}/local/media/${encodeURIComponent(filename)}${tokenParam()}`
}
