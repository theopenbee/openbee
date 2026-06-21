import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

export function SectionRule({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <span className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground whitespace-nowrap select-none">
        {children}
      </span>
      <div className="flex-1 h-px bg-border" />
    </div>
  )
}
