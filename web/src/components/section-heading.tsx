import { Badge } from "@/components/ui/badge"

// Neutral selection count beside a section or panel title; hidden at zero.
export function CountBadge({ count }: { count: number }) {
  if (count <= 0) return null
  return (
    <Badge variant="secondary" className="tabular-nums">
      {count}
    </Badge>
  )
}

export function SectionHeading({ text, badge }: { text: string; badge?: number }) {
  return (
    <div className="flex items-center gap-2">
      <p className="text-sm leading-none font-semibold text-strong">{text}</p>
      {badge !== undefined && <CountBadge count={badge} />}
    </div>
  )
}
