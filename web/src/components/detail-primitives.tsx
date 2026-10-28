import type { ReactNode } from "react"
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
