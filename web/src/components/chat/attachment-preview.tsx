import { memo } from "react"
import {
  ArrowUpRight,
  File,
  FileArchive,
  FileAudio,
  FileCode,
  FileImage,
  FileText,
  FileVideo,
} from "lucide-react"
import { ImageLightbox } from "@/components/image-lightbox"
import { mediaUrl } from "@/lib/chat-format"
import { basename, cn, getFileCategory, isImage } from "@/lib/utils"

export function FileCategoryIcon({ filePath, className }: { filePath: string; className?: string }) {
  const category = getFileCategory(filePath)
  const props = { size: 16, strokeWidth: 1.75, "aria-hidden": true, className: cn("shrink-0", className) }
  switch (category) {
    case "image": return <FileImage {...props} />
    case "audio": return <FileAudio {...props} />
    case "video": return <FileVideo {...props} />
    case "document": return <FileText {...props} />
    case "code": return <FileCode {...props} />
    case "archive": return <FileArchive {...props} />
    default: return <File {...props} />
  }
}

export const AttachmentPreview = memo(function AttachmentPreview({
  mediaPath,
}: {
  mediaPath: string
}) {
  const filename = basename(mediaPath)
  const url = mediaUrl(mediaPath)

  if (isImage(mediaPath)) {
    return <ImageLightbox src={url} alt={filename} />
  }

  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className="inline-flex min-h-11 max-w-full items-center gap-2 rounded-control border border-border bg-surface px-3 text-sm text-foreground transition-colors hover:bg-surface-muted"
    >
      <FileCategoryIcon filePath={mediaPath} className="text-secondary" />
      <span className="min-w-0 truncate">{filename}</span>
      <ArrowUpRight size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-muted" />
    </a>
  )
})
