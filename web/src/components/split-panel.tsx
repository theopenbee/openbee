import type { CSSProperties, ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { AnimatePresence, PresenceContext, motion, useReducedMotion, type Transition } from "motion/react"
import { XIcon } from "lucide-react"
import { Button } from "@/components/arc/button/button"
import { Drawer, DrawerContent } from "@/components/arc/drawer/drawer"
import { motionTokens } from "@/components/arc/lib/motion-tokens"
import { useMediaQuery } from "@/hooks/use-media-query"

const WIDE_QUERY = "(min-width: 74rem)"
const PANEL_WIDTH = "25rem"
const DRAWER_STYLE = { "--drawer-size": "min(28rem, 100vw)", zIndex: 50 } as CSSProperties

const slide: Transition = {
  width: motionTokens.spring.smooth,
  opacity: { duration: motionTokens.duration.standard, ease: [...motionTokens.ease.standard] },
}
const fade: Transition = { duration: motionTokens.duration.instant }

interface SplitPanelProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  children: ReactNode
}

export function SplitPanel({ open, onOpenChange, title, children }: SplitPanelProps) {
  const wide = useMediaQuery(WIDE_QUERY)
  const body = <PresenceContext.Provider value={null}>{children}</PresenceContext.Provider>

  if (!wide) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent side="right" title={title} aria-describedby={undefined} style={DRAWER_STYLE}>
          {body}
        </DrawerContent>
      </Drawer>
    )
  }

  return (
    <InlinePanel open={open} onClose={() => onOpenChange(false)} title={title}>
      {body}
    </InlinePanel>
  )
}

function InlinePanel({ open, onClose, title, children }: { open: boolean; onClose: () => void; title: string; children: ReactNode }) {
  const { t } = useTranslation()
  const reduced = useReducedMotion()
  const closeLabel = t("chat.panelClose")

  return (
    <AnimatePresence initial={false}>
      {open && (
        <motion.aside
          key="panel"
          aria-label={title}
          className="w-100 shrink-0 overflow-hidden border-l border-border bg-surface"
          initial={reduced ? { opacity: 0 } : { width: 0, opacity: 0 }}
          animate={{ width: PANEL_WIDTH, opacity: 1 }}
          exit={reduced ? { opacity: 0 } : { width: 0, opacity: 0 }}
          transition={reduced ? fade : slide}
        >
          <div className="flex h-full w-[calc(25rem-1px)] flex-col">
            <header className="flex h-16 shrink-0 items-center justify-between gap-3 border-b border-border pr-3 pl-6">
              <h2 className="min-w-0 truncate text-base font-medium text-foreground">{title}</h2>
              <Button variant="ghost" size="sm" aria-label={closeLabel} onClick={onClose}>
                <XIcon size={18} strokeWidth={1.75} aria-hidden="true" />
              </Button>
            </header>
            <div className="relative min-h-0 flex-1 overflow-y-auto overscroll-contain p-6">{children}</div>
          </div>
        </motion.aside>
      )}
    </AnimatePresence>
  )
}
