import type { ReactNode } from "react"
import { cn } from "@/lib/utils"
import { SURFACE } from "@/lib/styles"

export function DetailSection({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return <section className={cn(SURFACE, className)}>{children}</section>
}
