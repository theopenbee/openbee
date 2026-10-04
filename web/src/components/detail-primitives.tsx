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
    <section className={cn("relative overflow-hidden rounded-sm bg-card ring-1 ring-border", className)}>
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
    <section className={cn("overflow-hidden rounded-sm bg-card ring-1 ring-border", className)}>
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
    <div className={cn("rounded-sm bg-card p-4 ring-1 ring-border", className)}>
      <div className={cn(FIELD_LABEL, "flex items-center gap-1.5")}>
        {Icon ? <Icon className="size-3.5" /> : null}
        <span>{label}</span>
      </div>
      <div className={cn("mt-2 text-xl font-semibold tabular-nums text-strong", valueClassName)}>{value}</div>
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
      <p className={FIELD_LABEL}>{label}</p>
      <div className={cn("text-sm text-foreground", mono && "font-mono break-all")}>{value}</div>
    </div>
  )
}
