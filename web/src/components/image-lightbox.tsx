import { useState } from "react"
import { ArrowUpRight } from "lucide-react"
import { useTranslation } from "react-i18next"
import { MediaViewer, VIEWER_KEEP, VIEWER_LINK } from "@/components/media-viewer"
import { cn } from "@/lib/utils"

export function ImageLightbox({
  src,
  alt,
  className,
}: {
  src: string
  alt: string
  className?: string
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <MediaViewer
      open={open}
      onOpenChange={setOpen}
      title={alt}
      closeLabel={t("chat.closeImage")}
      trigger={
        <button
          type="button"
          aria-label={t("chat.viewImage", { name: alt })}
          className={cn(
            "block max-w-full cursor-zoom-in overflow-hidden rounded-inset border border-border bg-surface transition-opacity hover:opacity-90",
            className
          )}
        >
          <img src={src} alt={alt} className="block max-h-60 w-auto max-w-full object-contain" />
        </button>
      }
      action={
        <a href={src} target="_blank" rel="noreferrer" className={VIEWER_LINK}>
          <ArrowUpRight size={16} strokeWidth={1.75} aria-hidden="true" />
          <span className="hidden sm:inline">{t("chat.openOriginal")}</span>
          <span className="sr-only sm:hidden">{t("chat.openOriginal")}</span>
        </a>
      }
    >
      <div className="flex min-h-0 flex-1 items-center justify-center px-4 pb-8 sm:px-12">
        <img
          {...VIEWER_KEEP}
          src={src}
          alt={alt}
          className="max-h-full max-w-full rounded-panel border border-border bg-surface object-contain shadow-floating"
        />
      </div>
    </MediaViewer>
  )
}
