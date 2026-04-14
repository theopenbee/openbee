import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

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

export function getErrorMessage(err: unknown): string {
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
