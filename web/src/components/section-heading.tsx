import { Badge } from "@/components/arc/badge/badge"

export function SectionHeading({ text, badge }: { text: string; badge?: number }) {
  return (
    <div className="flex items-center gap-2">
      <h2 className="text-base font-medium text-foreground">{text}</h2>
      {badge !== undefined && badge > 0 && (
        <Badge tone="info" size="sm" className="tabular">{badge}</Badge>
      )}
    </div>
  )
}
