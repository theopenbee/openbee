import { Skeleton } from "@/components/arc/skeleton/skeleton"
import { cn } from "@/lib/utils"

export function SkeletonLine({ className }: { className?: string }) {
  return <div className={cn("skeleton h-4 w-full", className)} />
}

export function SkeletonCard({ className }: { className?: string }) {
  return (
    <div className={cn("rounded-surface border border-border bg-surface p-6", className)}>
      <Skeleton lines={3} />
    </div>
  )
}

export function SkeletonTable({ rows = 5, columns = 5, className }: { rows?: number; columns?: number; className?: string }) {
  return (
    <div className={cn("overflow-hidden rounded-surface border border-border bg-surface", className)} aria-busy="true">
      <div className="flex gap-8 border-b border-border-subtle px-6 py-4">
        {Array.from({ length: columns }).map((_, i) => (
          <div key={i} className="skeleton h-3 w-20" />
        ))}
      </div>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-8 border-t border-border-subtle px-6 py-4 first:border-t-0">
          <div className="flex items-center gap-3">
            <div className="skeleton size-8 rounded-full" />
            <div className="skeleton h-4 w-32" />
          </div>
          {Array.from({ length: Math.max(0, columns - 1) }).map((_, j) => (
            <div key={j} className="skeleton h-4 w-20" />
          ))}
        </div>
      ))}
    </div>
  )
}
