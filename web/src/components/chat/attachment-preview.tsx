import { memo } from "react"
import { ArrowUpRight } from "lucide-react"
import { ImageLightbox } from "@/components/image-lightbox"
import { FILE_CHIP, FileCategoryIcon } from "@/components/chat/file-chip"
import { TextPreview } from "@/components/chat/text-preview"
import { attachmentName, mediaUrl } from "@/lib/chat-format"
import { previewKind } from "@/lib/text-preview"
import { isImage } from "@/lib/utils"

export const AttachmentPreview = memo(function AttachmentPreview({
  mediaPath,
}: {
  mediaPath: string
}) {
  const filename = attachmentName(mediaPath)
  const url = mediaUrl(mediaPath)

  if (isImage(mediaPath)) {
    return <ImageLightbox src={url} alt={filename} />
  }

  const kind = previewKind(mediaPath)
  if (kind) {
    return <TextPreview mediaPath={mediaPath} kind={kind} />
  }

  return (
    <a href={url} target="_blank" rel="noreferrer" className={FILE_CHIP}>
      <FileCategoryIcon filePath={mediaPath} className="text-secondary" />
      <span className="min-w-0 truncate">{filename}</span>
      <ArrowUpRight size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-muted" />
    </a>
  )
})
