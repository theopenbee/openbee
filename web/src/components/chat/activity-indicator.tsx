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
    <div className="shrink-0 bg-background">
      <div className={cn(CHAT_COLUMN, "flex items-center gap-3", label && "pb-3")}>
        {label && avatar}
        <div role="status" className="min-w-0">
          {label && <TextShimmer className="text-sm text-secondary">{label}</TextShimmer>}
        </div>
        {label && onShowExecution && (
          <Button variant="ghost" size="sm" className="ml-auto shrink-0" onClick={onShowExecution}>
            <ListTreeIcon size={14} strokeWidth={1.75} aria-hidden="true" />
            {t("execution.show")}
          </Button>
        )}
      </div>
    </div>
  )
}
