import type { ReactNode } from "react"
import type { LucideIcon } from "lucide-react"
import { cn } from "@/lib/utils"

export function DetailHero({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn("relative overflow-hidden rounded-sm border border-border/70 bg-card", className)}>
      <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-primary/25 to-transparent" />
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
    <section className={cn("overflow-hidden rounded-sm border border-border/70 bg-card", className)}>
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
    <div className={cn("rounded-sm border border-border/70 bg-background/80 p-4", className)}>
      <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
        {Icon ? <Icon className="size-3.5" /> : null}
        <span>{label}</span>
      </div>
      <div className={cn("mt-3 text-base font-medium text-foreground", valueClassName)}>{value}</div>
      {hint ? <div className="mt-2 text-xs text-muted-foreground">{hint}</div> : null}
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
    <div className="space-y-1">
      <p className="text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">{label}</p>
      <div className={cn("text-sm text-foreground", mono && "font-mono break-all")}>{value}</div>
    </div>
  )
}
