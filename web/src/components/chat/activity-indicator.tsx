import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { ListTreeIcon } from "lucide-react"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { CHAT_COLUMN } from "@/lib/styles"
import { cn } from "@/lib/utils"

interface ActivityIndicatorProps {
  label: string | null
  avatar: ReactNode
  onShowExecution?: () => void
}

export function ActivityIndicator({ label, avatar, onShowExecution }: ActivityIndicatorProps) {
  const { t } = useTranslation()
  return (
    <div role="status" className="shrink-0 bg-background">
      {label && (
        <div className={cn(CHAT_COLUMN, "flex items-center gap-3 pb-3")}>
          {avatar}
          <TextShimmer className="text-sm text-secondary">{label}</TextShimmer>
          {onShowExecution && (
            <button
              type="button"
              onClick={onShowExecution}
              className="ml-auto inline-flex h-8 shrink-0 items-center gap-1.5 rounded-control px-3 text-xs text-secondary transition-colors hover:bg-surface-muted hover:text-foreground"
            >
              <ListTreeIcon size={14} strokeWidth={1.75} aria-hidden="true" />
              {t("execution.show")}
            </button>
          )}
        </div>
      )}
    </div>
  )
}
