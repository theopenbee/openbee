import { useTranslation } from "react-i18next"
import { ChevronLeft, ChevronRight } from "lucide-react"
import { Button } from "@/components/ui/button"

interface PaginationControlsProps {
  page: number
  totalPages: number
  onPageChange: (page: number) => void
  /** Optional record-count label shown on the left; keeps the footer visible even on a single page. */
  leadingLabel?: string
}

export function PaginationControls({ page, totalPages, onPageChange, leadingLabel }: PaginationControlsProps) {
  const { t } = useTranslation()

  if (totalPages <= 1 && !leadingLabel) return null

  const hasPages = totalPages > 1

  return (
    <div className="flex items-center justify-between mt-4">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        {leadingLabel && <span>{leadingLabel}</span>}
        {leadingLabel && hasPages && <span aria-hidden="true" className="text-border">·</span>}
        {hasPages && <span>{t("sessions.pagination.page", { page, totalPages })}</span>}
      </div>
      {hasPages && (
        <div className="flex gap-2">
          <Button
            variant="outline"
            disabled={page <= 1}
            onClick={() => onPageChange(Math.max(1, page - 1))}
          >
            <ChevronLeft className="size-4" />
            {t("sessions.pagination.previous")}
          </Button>
          <Button
            variant="outline"
            disabled={page >= totalPages}
            onClick={() => onPageChange(Math.min(totalPages, page + 1))}
          >
            {t("sessions.pagination.next")}
            <ChevronRight className="size-4" />
          </Button>
        </div>
      )}
    </div>
  )
}
