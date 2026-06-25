import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

// A self-contained board: the title lives *inside* the card at the top, with the
// content directly beneath it — replacing the older "floating label + hairline
// rule above a separate panel" pattern. One squared, hairline-ringed surface per
// section. Pass `flush` for edge-to-edge content (divided lists, metric grids)
// where rows carry their own horizontal padding.
export function Panel({
  title,
  action,
  children,
  flush = false,
  className,
  bodyClassName,
  ariaLabel,
}: {
  title: ReactNode
  action?: ReactNode
  children: ReactNode
  flush?: boolean
  className?: string
  bodyClassName?: string
  ariaLabel?: string
}) {
  return (
    <section
      aria-label={ariaLabel}
      className={cn("rounded-sm border border-border/70 bg-card", className)}
    >
      <header className="flex items-center justify-between gap-3 px-5 pt-4 pb-3">
        <h2 className="min-w-0 truncate text-sm font-medium tracking-tight text-foreground">
          {title}
        </h2>
        {action ? <div className="shrink-0">{action}</div> : null}
      </header>
      <div className={cn(flush ? "pb-1.5" : "px-5 pb-5", bodyClassName)}>{children}</div>
    </section>
  )
}
