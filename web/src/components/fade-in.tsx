import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

export function FadeIn({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("animate-fade-in", className)}>{children}</div>
}
