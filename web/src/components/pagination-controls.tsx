import { Pagination } from "@/components/arc/pagination/pagination"

interface PaginationControlsProps {
  page: number
  totalPages: number
  onPageChange: (page: number) => void
  leadingLabel?: string
}

export function PaginationControls({ page, totalPages, onPageChange, leadingLabel }: PaginationControlsProps) {
  if (totalPages <= 1 && !leadingLabel) return null

  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
      <span className="text-sm text-secondary tabular">{leadingLabel}</span>
      {totalPages > 1 && <Pagination page={page} pageCount={totalPages} onPageChange={onPageChange} />}
    </div>
  )
}
