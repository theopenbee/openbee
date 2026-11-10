import {
  File,
  FileArchive,
  FileAudio,
  FileCode,
  FileImage,
  FileText,
  FileVideo,
} from "lucide-react"
import { cn, getFileCategory } from "@/lib/utils"

export const FILE_CHIP =
  "inline-flex min-h-11 max-w-full items-center gap-2 rounded-control border border-border bg-surface px-3 text-sm text-foreground transition-colors hover:bg-surface-muted"

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
