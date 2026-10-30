import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

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
      className={cn("min-w-0 overflow-hidden rounded-surface border border-border bg-surface", className)}
    >
      <header className="flex items-start justify-between gap-4 px-6 pt-5 pb-4">
        <div className="min-w-0">
          <h2 className="truncate text-base font-medium text-foreground">{title}</h2>
          {description && <p className="mt-1 text-sm text-secondary">{description}</p>}
        </div>
        {action ? <div className="flex shrink-0 items-center gap-2">{action}</div> : null}
      </header>
      <div className={cn(flush ? "pb-2" : "px-6 pb-6", bodyClassName)}>{children}</div>
    </section>
  )
}
