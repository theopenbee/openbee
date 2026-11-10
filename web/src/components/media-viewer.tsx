import { useRef, type MouseEvent, type PointerEvent, type ReactNode } from "react"
import * as DialogPrimitive from "@radix-ui/react-dialog"
import { AnimatePresence, motion, useReducedMotion, type Transition } from "motion/react"
import { X } from "lucide-react"
import { Button } from "@/components/arc/button/button"
import { motionTokens } from "@/components/arc/lib/motion-tokens"

export const VIEWER_LINK =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-control border border-border bg-surface px-4 text-sm font-medium text-foreground transition-colors hover:bg-surface-muted"

export const VIEWER_KEEP = { "data-viewer-keep": "" }

function holdEscape(e: KeyboardEvent) {
  if (document.querySelector('[data-streamdown="link-safety-modal"]')) e.preventDefault()
}

function insideKeep(target: EventTarget) {
  return target instanceof Element && target.closest("[data-viewer-keep]") !== null
}

export function MediaViewer({
  open,
  onOpenChange,
  trigger,
  title,
  action,
  closeLabel,
  children,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  trigger: ReactNode
  title: string
  action: ReactNode
  closeLabel: string
  children: ReactNode
}) {
  const reduced = useReducedMotion()
  const armed = useRef(false)

  const fade: Transition = reduced
    ? { duration: motionTokens.duration.instant }
    : { duration: motionTokens.duration.standard, ease: [...motionTokens.ease.enter] }
  const leave: Transition = { duration: motionTokens.duration.fast, ease: [...motionTokens.ease.standard] }

  const arm = (e: PointerEvent) => {
    armed.current = !insideKeep(e.target)
  }
  const dismiss = (e: MouseEvent) => {
    if (armed.current && !insideKeep(e.target)) onOpenChange(false)
    armed.current = false
  }

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Trigger asChild>{trigger}</DialogPrimitive.Trigger>

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
            <DialogPrimitive.Content asChild forceMount aria-describedby={undefined} onEscapeKeyDown={holdEscape}>
              <motion.div
                className="fixed inset-0 z-50 flex flex-col"
                onPointerDown={arm}
                onClick={dismiss}
                initial={reduced ? { opacity: 0 } : { opacity: 0, scale: 0.98 }}
                animate={{ opacity: 1, scale: 1 }}
                exit={reduced ? { opacity: 0, transition: { duration: 0 } } : { opacity: 0, scale: 0.99, transition: leave }}
                transition={reduced ? fade : { default: motionTokens.spring.smooth, opacity: fade }}
              >
                <div {...VIEWER_KEEP} className="flex shrink-0 items-center gap-3 p-4">
                  <DialogPrimitive.Title className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
                    {title}
                  </DialogPrimitive.Title>
                  {action}
                  <DialogPrimitive.Close asChild>
                    <Button variant="secondary" aria-label={closeLabel}>
                      <X size={16} strokeWidth={1.75} aria-hidden="true" />
                    </Button>
                  </DialogPrimitive.Close>
                </div>
                {children}
              </motion.div>
            </DialogPrimitive.Content>
          </DialogPrimitive.Portal>
        )}
      </AnimatePresence>
    </DialogPrimitive.Root>
  )
}
