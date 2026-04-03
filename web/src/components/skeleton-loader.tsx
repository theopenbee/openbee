import { cn } from "@/lib/utils"

export function SkeletonLine({ className }: { className?: string }) {
  return <div className={cn("skeleton h-4 w-full", className)} />
}

export function SkeletonCard() {
  return (
    <div className="rounded-xl bg-card border p-5 space-y-3">
      <div className="flex items-center justify-between">
        <div className="skeleton h-5 w-32" />
        <div className="skeleton h-5 w-16 rounded-full" />
      </div>
      <div className="skeleton h-4 w-full" />
      <div className="skeleton h-4 w-2/3" />
    </div>
  )
}

export function SkeletonTable({ rows = 5 }: { rows?: number }) {
  return (
    <div className="rounded-xl bg-card ring-1 ring-foreground/5 overflow-hidden">
      <div className="bg-secondary/50 px-4 py-3 flex gap-8">
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="skeleton h-4 w-20" />
        ))}
      </div>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="px-4 py-3 flex gap-8 border-t border-border">
          {Array.from({ length: 5 }).map((_, j) => (
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
          <div className="skeleton h-8 w-48" />
          <div className="skeleton h-4 w-32" />
        </div>
        <div className="skeleton h-5 w-16 rounded-full" />
      </div>
      <div className="space-y-3">
        <div className="skeleton h-4 w-full" />
        <div className="skeleton h-4 w-3/4" />
        <div className="skeleton h-4 w-1/2" />
      </div>
      <div className="rounded-xl bg-card ring-1 ring-foreground/5 p-5 space-y-3">
        <div className="skeleton h-4 w-full" />
        <div className="skeleton h-4 w-2/3" />
        <div className="skeleton h-4 w-1/2" />
      </div>
    </div>
  )
}
