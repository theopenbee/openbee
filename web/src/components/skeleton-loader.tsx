import { cn } from "@/lib/utils"
import { SURFACE } from "@/lib/styles"

export function SkeletonLine({ className }: { className?: string }) {
  return <div className={cn("skeleton h-4 w-full", className)} />
}

export function SkeletonCard() {
  return (
    <div className="space-y-3 rounded-sm bg-card p-5 ring-1 ring-border">
      <div className="flex items-center justify-between">
        <div className="skeleton h-5 w-32" />
        <div className="skeleton h-5 w-16 rounded-full" />
      </div>
      <div className="skeleton h-4 w-full" />
      <div className="skeleton h-4 w-2/3" />
    </div>
  )
}

export function SkeletonTable({ rows = 5, columns = 5 }: { rows?: number; columns?: number }) {
  return (
    <div className={SURFACE}>
      <div className="flex gap-8 border-b border-border px-3 py-3">
        {Array.from({ length: columns }).map((_, i) => (
          <div key={i} className="skeleton h-4 w-20" />
        ))}
      </div>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-8 border-t border-hairline px-3 py-3">
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

export function SkeletonPage() {
  return (
    <div className="animate-fade-in space-y-6">
      <div className="flex items-center justify-between">
        <div className="space-y-2">
          <div className="skeleton h-7 w-48" />
          <div className="skeleton h-4 w-32" />
        </div>
        <div className="skeleton h-5 w-16 rounded-full" />
      </div>
      <div className="space-y-3">
        <div className="skeleton h-4 w-full" />
        <div className="skeleton h-4 w-3/4" />
        <div className="skeleton h-4 w-1/2" />
      </div>
      <div className="space-y-3 rounded-sm bg-card p-5 ring-1 ring-border">
        <div className="skeleton h-4 w-full" />
        <div className="skeleton h-4 w-2/3" />
        <div className="skeleton h-4 w-1/2" />
      </div>
    </div>
  )
}
