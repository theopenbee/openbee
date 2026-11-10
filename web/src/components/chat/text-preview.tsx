import { useState, type ReactNode } from "react"
import { useQuery } from "@tanstack/react-query"
import { Eye, FileWarning, RotateCw } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Button } from "@/components/arc/button/button"
import { Skeleton } from "@/components/arc/skeleton/skeleton"
import { ChatMarkdown } from "@/components/chat-markdown"
import { EmptyState } from "@/components/empty-state"
import { DownloadLink, MediaViewer, VIEWER_KEEP, VIEWER_LINK } from "@/components/media-viewer"
import { FILE_CHIP, FileCategoryIcon } from "@/components/chat/file-chip"
import { api } from "@/lib/api"
import { attachmentName, mediaUrl } from "@/lib/chat-format"
import {
  codeLanguage,
  fenceCode,
  readTextPreview,
  RENDER_LIMIT,
  type PreviewKind,
} from "@/lib/text-preview"
import { basename } from "@/lib/utils"

const PREVIEW_CONTROLS = { table: { fullscreen: false } }

function Notice({ title, description, action }: { title: string; description: string; action?: ReactNode }) {
  return (
    <EmptyState
      icon={<FileWarning size={20} strokeWidth={1.75} aria-hidden="true" />}
      title={title}
      description={description}
      action={action}
    />
  )
}

function PlainText({ text }: { text: string }) {
  return (
    <pre className="m-0 whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-foreground">
      {text}
    </pre>
  )
}

export function TextPreview({ mediaPath, kind }: { mediaPath: string; kind: PreviewKind }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const stored = basename(mediaPath)
  const filename = attachmentName(mediaPath)
  const url = mediaUrl(mediaPath)
  const preview = useQuery({
    queryKey: ["media-preview", stored],
    queryFn: async ({ signal }) => readTextPreview(await api.chat.media(stored, signal)),
    enabled: open,
    staleTime: Infinity,
    retry: false,
  })

  const download = <DownloadLink url={url} filename={filename} className={VIEWER_LINK} />

  let body: ReactNode
  if (preview.isPending) {
    body = <Skeleton lines={6} label={t("chat.previewLoading")} />
  } else if (preview.isError) {
    body = (
      <Notice
        title={t("chat.previewFailedTitle")}
        description={t("chat.previewFailed")}
        action={
          <Button variant="secondary" onClick={() => preview.refetch()}>
            <RotateCw size={16} strokeWidth={1.75} aria-hidden="true" />
            {t("common.retry")}
          </Button>
        }
      />
    )
  } else if (preview.data.status === "too-large") {
    body = <Notice title={t("chat.previewTooLargeTitle")} description={t("chat.previewTooLarge")} action={download} />
  } else if (preview.data.status === "unsupported") {
    body = <Notice title={t("chat.previewUnsupportedTitle")} description={t("chat.previewUnsupported")} action={download} />
  } else if (preview.data.text.trim() === "") {
    body = <p className="text-sm text-secondary">{t("chat.previewEmpty")}</p>
  } else if (kind !== "text" && preview.data.text.length <= RENDER_LIMIT) {
    const source = kind === "markdown" ? preview.data.text : fenceCode(preview.data.text, codeLanguage(mediaPath))
    body = <ChatMarkdown controls={PREVIEW_CONTROLS}>{source}</ChatMarkdown>
  } else {
    body = <PlainText text={preview.data.text} />
  }

  return (
    <MediaViewer
      open={open}
      onOpenChange={setOpen}
      title={filename}
      closeLabel={t("chat.closePreview")}
      trigger={
        <button type="button" aria-label={t("chat.previewFile", { name: filename })} className={FILE_CHIP}>
          <FileCategoryIcon filePath={mediaPath} className="text-secondary" />
          <span className="min-w-0 truncate">{filename}</span>
          <Eye size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-muted" />
        </button>
      }
      action={download}
    >
      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-8 sm:px-12">
        <div
          {...VIEWER_KEEP}
          className="mx-auto w-full max-w-4xl rounded-surface border border-border bg-surface p-6 shadow-floating"
        >
          {body}
        </div>
      </div>
    </MediaViewer>
  )
}
