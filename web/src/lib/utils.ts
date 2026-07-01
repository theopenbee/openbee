import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"
import i18n from "i18next"
import { ApiError } from "./api"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

const IMAGE_EXTS = new Set(["jpg", "jpeg", "png", "gif", "webp", "svg", "bmp"])
const AUDIO_EXTS = new Set(["mp3", "wav", "ogg", "aac", "flac", "m4a", "wma"])
const VIDEO_EXTS = new Set(["mp4", "mov", "avi", "mkv", "webm", "flv", "wmv"])
const DOC_EXTS = new Set(["pdf", "doc", "docx", "txt", "md", "csv", "xls", "xlsx", "ppt", "pptx", "rtf"])
const CODE_EXTS = new Set(["js", "ts", "jsx", "tsx", "py", "go", "java", "c", "cpp", "h", "cs", "rb", "php", "html", "css", "json", "xml", "yaml", "yml", "sh", "sql"])
const ARCHIVE_EXTS = new Set(["zip", "tar", "gz", "rar", "7z", "bz2"])

export type FileCategory = "image" | "audio" | "video" | "document" | "code" | "archive" | "other"

export function basename(filePath: string): string {
  return filePath.split("/").pop() ?? filePath
}

export function isImage(filePath: string): boolean {
  const ext = filePath.split(".").pop()?.toLowerCase() ?? ""
  return IMAGE_EXTS.has(ext)
}

// getErrorMessage turns a caught value into a user-facing string. For ApiErrors
// that carry a stable backend `code`, it looks the code up in the `errors` i18n
// namespace (interpolating any `params`) so the message is shown in the user's
// language; the raw backend message is used as the fallback when the code is
// missing or has no translation.
export function getErrorMessage(err: unknown): string {
  if (err instanceof ApiError && err.code) {
    return i18n.t(`errors.${err.code}`, {
      defaultValue: err.message,
      ...err.params,
    })
  }
  return err instanceof Error ? err.message : String(err)
}

export function getFileCategory(filePath: string): FileCategory {
  const ext = filePath.split(".").pop()?.toLowerCase() ?? ""
  if (IMAGE_EXTS.has(ext)) return "image"
  if (AUDIO_EXTS.has(ext)) return "audio"
  if (VIDEO_EXTS.has(ext)) return "video"
  if (DOC_EXTS.has(ext)) return "document"
  if (CODE_EXTS.has(ext)) return "code"
  if (ARCHIVE_EXTS.has(ext)) return "archive"
  return "other"
}
