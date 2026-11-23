import type { CSSProperties } from "react"
import { CpuIcon } from "lucide-react"

import { cn } from "@/lib/utils"

interface EngineIconProps {
  icon?: string
  title?: string
  className?: string
}

export function engineIconMask(svg: string): string {
  return `url("data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}")`
}

export function EngineIcon({ icon, title, className }: EngineIconProps) {
  const a11y = title ? { role: "img" as const, "aria-label": title } : { "aria-hidden": true }
  if (!icon) return <CpuIcon strokeWidth={1.75} className={cn("size-4 shrink-0", className)} {...a11y} />

  const style: CSSProperties = {
    maskImage: engineIconMask(icon),
    maskSize: "contain",
    maskRepeat: "no-repeat",
    maskPosition: "center",
  }
  return <span className={cn("inline-block size-4 shrink-0 bg-current", className)} style={style} {...a11y} />
}
