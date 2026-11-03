import type { ReactNode } from "react"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { CHAT_COLUMN } from "@/lib/styles"
import { cn } from "@/lib/utils"

interface ActivityIndicatorProps {
  label: string | null
  avatar: ReactNode
}

export function ActivityIndicator({ label, avatar }: ActivityIndicatorProps) {
  return (
    <div role="status" className="shrink-0 bg-background">
      {label && (
        <div className={cn(CHAT_COLUMN, "flex items-center gap-3 pb-3")}>
          {avatar}
          <TextShimmer className="text-sm text-secondary">{label}</TextShimmer>
        </div>
      )}
    </div>
  )
}
