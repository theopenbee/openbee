import type { ReactNode } from "react"
import type { LucideIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import { FIELD_LABEL } from "@/lib/styles"

export function DetailHero({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn("min-w-0 overflow-hidden rounded-surface border border-border bg-surface", className)}>
      {children}
    </section>
  )
}

export function DetailSection({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn("min-w-0 overflow-hidden rounded-surface border border-border bg-surface", className)}>
      {children}
    </section>
  )
}

export function DetailOverviewStat({
  icon: Icon,
  label,
  value,
  hint,
  className,
  valueClassName,
}: {
  icon?: LucideIcon
  label: string
  value: ReactNode
  hint?: ReactNode
  className?: string
  valueClassName?: string
}) {
  return (
    <div className={cn("min-w-0 rounded-inset bg-surface-muted p-4", className)}>
      <div className={cn(FIELD_LABEL, "flex items-center gap-2")}>
        {Icon ? <Icon size={14} strokeWidth={1.75} aria-hidden="true" /> : null}
        <span>{label}</span>
      </div>
      <div className={cn("mt-2 text-base font-medium text-foreground wrap-anywhere", valueClassName)}>{value}</div>
      {hint ? <div className="mt-1 text-xs text-secondary wrap-anywhere">{hint}</div> : null}
    </div>
  )
}

export function DetailField({
  label,
  value,
  mono = false,
}: {
  label: string
  value: ReactNode
  mono?: boolean
}) {
  return (
    <div className="grid gap-1">
      <p className={FIELD_LABEL}>{label}</p>
      <div className={cn("text-sm text-foreground", mono && "font-mono break-all")}>{value}</div>
    </div>
  )
}
