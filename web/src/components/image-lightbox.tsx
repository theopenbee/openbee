import { useState } from "react"
import * as DialogPrimitive from "@radix-ui/react-dialog"
import { AnimatePresence, motion, useReducedMotion, type Transition } from "motion/react"
import { ArrowUpRight, X } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Button } from "@/components/arc/button/button"
import { motionTokens } from "@/components/arc/lib/motion-tokens"
import { cn } from "@/lib/utils"

const VIEWER_LINK =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-control border border-border bg-surface px-4 text-sm font-medium text-foreground transition-colors hover:bg-surface-muted"

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
  const reduced = useReducedMotion()
  const [open, setOpen] = useState(false)

  const fade: Transition = reduced
    ? { duration: motionTokens.duration.instant }
    : { duration: motionTokens.duration.standard, ease: [...motionTokens.ease.enter] }
  const leave: Transition = { duration: motionTokens.duration.fast, ease: [...motionTokens.ease.standard] }

  return (
    <DialogPrimitive.Root open={open} onOpenChange={setOpen}>
      <DialogPrimitive.Trigger asChild>
        <button
          type="button"
          aria-label={t("localChat.viewImage", { name: alt })}
          className={cn(
            "block max-w-full cursor-zoom-in overflow-hidden rounded-inset border border-border bg-surface transition-opacity hover:opacity-90",
            className
          )}
        >
          <img src={src} alt={alt} className="block max-h-60 w-auto max-w-full object-contain" />
        </button>
      </DialogPrimitive.Trigger>

      <AnimatePresence>
        {open && (
          <DialogPrimitive.Portal forceMount>
            <DialogPrimitive.Overlay asChild forceMount>
              <motion.div
                className="fixed inset-0 z-50 bg-background/90 backdrop-blur-sm"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0, transition: reduced ? { duration: 0 } : leave }}
                transition={fade}
              />
            </DialogPrimitive.Overlay>
            <DialogPrimitive.Content asChild forceMount aria-describedby={undefined}>
              <motion.div
                className="fixed inset-0 z-50 flex flex-col"
                onClick={() => setOpen(false)}
                initial={reduced ? { opacity: 0 } : { opacity: 0, scale: 0.98 }}
                animate={{ opacity: 1, scale: 1 }}
                exit={reduced ? { opacity: 0, transition: { duration: 0 } } : { opacity: 0, scale: 0.99, transition: leave }}
                transition={reduced ? fade : { default: motionTokens.spring.smooth, opacity: fade }}
              >
                <div className="flex shrink-0 items-center gap-3 p-4" onClick={(e) => e.stopPropagation()}>
                  <DialogPrimitive.Title className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
                    {alt}
                  </DialogPrimitive.Title>
                  <a href={src} target="_blank" rel="noreferrer" className={VIEWER_LINK}>
                    <ArrowUpRight size={16} strokeWidth={1.75} aria-hidden="true" />
                    <span className="hidden sm:inline">{t("localChat.openOriginal")}</span>
                    <span className="sr-only sm:hidden">{t("localChat.openOriginal")}</span>
                  </a>
                  <DialogPrimitive.Close asChild>
                    <Button variant="secondary" aria-label={t("localChat.closeImage")}>
                      <X size={16} strokeWidth={1.75} aria-hidden="true" />
                    </Button>
                  </DialogPrimitive.Close>
                </div>
                <div className="flex min-h-0 flex-1 items-center justify-center px-4 pb-8 sm:px-12">
                  <img
                    src={src}
                    alt={alt}
                    onClick={(e) => e.stopPropagation()}
                    className="max-h-full max-w-full rounded-panel border border-border bg-surface object-contain shadow-floating"
                  />
                </div>
              </motion.div>
            </DialogPrimitive.Content>
          </DialogPrimitive.Portal>
        )}
      </AnimatePresence>
    </DialogPrimitive.Root>
  )
}
