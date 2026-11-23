import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { ListTreeIcon } from "lucide-react"
import { Button } from "@/components/arc/button/button"
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
            <Button variant="ghost" size="sm" className="ml-auto shrink-0" onClick={onShowExecution}>
              <ListTreeIcon size={14} strokeWidth={1.75} aria-hidden="true" />
              {t("execution.show")}
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
