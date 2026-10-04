import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

// Kumo LayerCard: an elevated gray frame whose header strip carries the title
// (and optional action), over a white body that sits flush inside the frame.
// The body's own 1px ring draws the line under the header; its side and bottom
// edges are clipped by the frame, so the section reads as one squared surface.
// `description` adds one muted line under the title inside the strip.
// Pass `flush` for edge-to-edge content (tables, divided lists, metric grids)
// where rows carry their own horizontal padding.
export function Panel({
  title,
  description,
  action,
  children,
  flush = false,
  className,
  bodyClassName,
  ariaLabel,
}: {
  title: ReactNode
  description?: ReactNode
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
      className={cn(
        "flex flex-col overflow-hidden rounded-sm bg-elevated ring-1 ring-border",
        className
      )}
    >
      <header className="flex min-h-11 items-center justify-between gap-3 px-4 py-2">
        <div className="min-w-0">
          <h2 className="truncate text-sm font-semibold text-strong">{title}</h2>
          {description ? (
            <p className="mt-0.5 text-[13px] leading-5 text-muted-foreground">{description}</p>
          ) : null}
        </div>
        {action ? <div className="flex shrink-0 items-center gap-2">{action}</div> : null}
      </header>
      <div
        className={cn(
          "flex-1 rounded-sm bg-card ring-1 ring-border",
          flush ? "py-0" : "p-4",
          bodyClassName
        )}
      >
        {children}
      </div>
    </section>
  )
}
