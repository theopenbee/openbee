import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

export function SettingsRow({
  title,
  description,
  inline = false,
  children,
}: {
  title: string
  description: string
  inline?: boolean
  children: ReactNode
}) {
  return (
    <div
      className={cn(
        "grid gap-4 px-4 py-5 @lg:px-6",
        inline
          ? "@lg:flex @lg:items-center @lg:justify-between @lg:gap-8"
          : "@2xl:grid-cols-[minmax(0,14rem)_minmax(0,1fr)] @2xl:gap-8",
      )}
    >
      <div className={cn("grid content-start", inline ? "min-w-0 gap-0.5" : "gap-1")}>
        <h3 className="text-sm font-medium text-foreground">{title}</h3>
        <p className="text-sm text-secondary">{description}</p>
      </div>
      <div className={inline ? "flex min-w-0 @lg:shrink-0 @lg:justify-end" : "min-w-0"}>{children}</div>
    </div>
  )
}

export function SettingsGroup({ children }: { children: ReactNode }) {
  return (
    <div className="divide-y divide-border-subtle overflow-hidden rounded-inset border border-border bg-surface">
      {children}
    </div>
  )
}

export function SettingsSkeleton() {
  return (
    <div className="grid gap-10" aria-busy="true">
      {[1, 2].map((group) => (
        <div key={group} className="grid gap-4">
          <div className="skeleton h-5 w-32" />
          <div className="divide-y divide-border-subtle rounded-inset border border-border bg-surface">
            {Array.from({ length: group }).map((_, row) => (
              <div key={row} className="grid gap-4 px-6 py-5 @2xl:grid-cols-[14rem_1fr] @2xl:gap-8">
                <div className="grid content-start gap-2">
                  <div className="skeleton h-4 w-24" />
                  <div className="skeleton h-4 w-40" />
                </div>
                <div className="skeleton h-11 w-full rounded-control" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
